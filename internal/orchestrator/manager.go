package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

type Manager struct {
	registry *providers.Registry
	store    store.Store
	events   *EventBus
	logger   zerolog.Logger
	metrics  *MetricsRecorder

	mu           sync.RWMutex
	sandboxes    map[string]*Sandbox
	admissionMu  sync.Mutex
	queueMu      sync.Mutex
	queueWaiters int
	capacityCh   chan struct{}

	defaultTTL    time.Duration
	defaultImage  string
	defaultMemory int
	defaultVCPUs  int
	limits        OperationalLimits

	vmPoolMgr  *VMPoolManager
	poolConfig config.PoolConfig

	previewDomain string

	ctx    context.Context
	cancel context.CancelFunc
}

type ManagerConfig struct {
	DefaultTTL    time.Duration
	DefaultImage  string
	DefaultMemory int
	DefaultVCPUs  int
	Pool          config.PoolConfig
	PreviewDomain string
	Limits        OperationalLimits
}

func NewManager(registry *providers.Registry, st store.Store, events *EventBus, logger zerolog.Logger, cfg ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		registry:      registry,
		store:         st,
		events:        events,
		logger:        logger.With().Str("component", "manager").Logger(),
		metrics:       NewMetricsRecorder(),
		sandboxes:     make(map[string]*Sandbox),
		capacityCh:    make(chan struct{}),
		defaultTTL:    cfg.DefaultTTL,
		defaultImage:  cfg.DefaultImage,
		defaultMemory: cfg.DefaultMemory,
		defaultVCPUs:  cfg.DefaultVCPUs,
		limits:        cfg.Limits,
		poolConfig:    cfg.Pool,
		previewDomain: cfg.PreviewDomain,
		ctx:           ctx,
		cancel:        cancel,
	}
	if m.defaultTTL == 0 {
		m.defaultTTL = 30 * time.Minute
	}
	if m.defaultImage == "" {
		m.defaultImage = "alpine:latest"
	}
	if m.defaultMemory == 0 {
		m.defaultMemory = 512
	}
	if m.defaultVCPUs == 0 {
		m.defaultVCPUs = 1
	}
	if m.limits.SpawnOverflow == "" {
		m.limits.SpawnOverflow = "reject"
	}
	if m.limits.SpawnQueueTimeout == 0 {
		m.limits.SpawnQueueTimeout = 30 * time.Second
	}
	if m.limits.MaxSpawnQueue == 0 {
		m.limits.MaxSpawnQueue = 100
	}
	return m
}

// Start launches the TTL reaper goroutine.
func (m *Manager) Start() {
	go m.reaper()
}

// Stop cancels the reaper and cleans up.
func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) reaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.pruneExpired()
		}
	}
}

func (m *Manager) pruneExpired() {
	expired, err := m.store.ListExpiredSandboxes(m.ctx, time.Now())
	if err != nil {
		m.logger.Error().Err(err).Msg("listing expired sandboxes")
		return
	}
	for _, sb := range expired {
		m.logger.Info().Str("sandbox", sb.ID).Msg("reaping expired sandbox")
		if err := m.Destroy(m.ctx, sb.ID); err != nil {
			m.logger.Error().Err(err).Str("sandbox", sb.ID).Msg("destroying expired sandbox")
		}
	}
}

// Reconcile refreshes persisted sandbox state against provider runtime state.
// It is intended for startup recovery after the server process restarts.
func (m *Manager) Reconcile(ctx context.Context) error {
	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return fmt.Errorf("listing sandboxes for reconciliation: %w", err)
	}

	known := make(map[string]struct{}, len(records))
	for _, rec := range records {
		known[rec.ID] = struct{}{}
		if SandboxState(rec.State) == StateDestroyed {
			continue
		}

		prov, err := m.registry.Get(rec.Provider)
		if err != nil {
			m.logger.Warn().Err(err).Str("sandbox", rec.ID).Str("provider", rec.Provider).Msg("reconcile: provider unavailable")
			m.publishOperationalEvent(EventProviderFailed, rec.ID, map[string]interface{}{
				"operation": "reconcile.status",
				"provider":  rec.Provider,
				"error":     err.Error(),
			})
			if updateErr := m.store.UpdateSandboxState(ctx, rec.ID, string(StateError)); updateErr != nil {
				return fmt.Errorf("marking sandbox %s error: %w", rec.ID, updateErr)
			}
			continue
		}

		status, err := prov.Status(ctx, rec.ID)
		if err != nil {
			if errors.Is(err, providers.ErrSandboxNotFound) || errors.Is(err, providers.ErrSandboxDestroyed) {
				if updateErr := m.store.UpdateSandboxState(ctx, rec.ID, string(StateDestroyed)); updateErr != nil {
					return fmt.Errorf("marking stale sandbox %s destroyed: %w", rec.ID, updateErr)
				}
				m.mu.Lock()
				delete(m.sandboxes, rec.ID)
				m.mu.Unlock()
				m.logger.Info().Str("sandbox", rec.ID).Msg("reconcile: stale sandbox marked destroyed")
				m.publishOperationalEvent(EventReconcileAction, rec.ID, map[string]interface{}{
					"action":   "marked_destroyed",
					"provider": rec.Provider,
					"reason":   err.Error(),
				})
				continue
			}
			m.logger.Warn().Err(err).Str("sandbox", rec.ID).Msg("reconcile: provider status failed")
			m.publishOperationalEvent(EventProviderFailed, rec.ID, map[string]interface{}{
				"operation": "reconcile.status",
				"provider":  rec.Provider,
				"error":     err.Error(),
			})
			if updateErr := m.store.UpdateSandboxState(ctx, rec.ID, string(StateError)); updateErr != nil {
				return fmt.Errorf("marking sandbox %s error: %w", rec.ID, updateErr)
			}
			continue
		}

		state := SandboxState(status.State)
		if state == "" {
			state = StateRunning
		}
		if state == StateDestroyed {
			if err := m.store.UpdateSandboxState(ctx, rec.ID, string(StateDestroyed)); err != nil {
				return fmt.Errorf("marking sandbox %s destroyed: %w", rec.ID, err)
			}
			continue
		}
		if state != SandboxState(rec.State) {
			if err := m.store.UpdateSandboxState(ctx, rec.ID, string(state)); err != nil {
				return fmt.Errorf("updating reconciled sandbox %s state: %w", rec.ID, err)
			}
		}

		sb := recordToSandbox(rec)
		sb.State = state
		sb.PreviewDomain = m.previewDomain
		m.mu.Lock()
		m.sandboxes[rec.ID] = sb
		m.mu.Unlock()
	}

	if err := m.reconcileProviderRuntimes(ctx, known); err != nil {
		return err
	}

	return nil
}

func (m *Manager) reconcileProviderRuntimes(ctx context.Context, known map[string]struct{}) error {
	for _, name := range m.registry.List() {
		prov, err := m.registry.Get(name)
		if err != nil {
			continue
		}
		lister, ok := prov.(providers.RuntimeSandboxLister)
		if !ok {
			continue
		}

		runtimes, err := lister.ListRuntimeSandboxes(ctx)
		if err != nil {
			m.logger.Warn().Err(err).Str("provider", name).Msg("reconcile: runtime inventory failed")
			m.publishOperationalEvent(EventProviderFailed, "", map[string]interface{}{
				"operation": "reconcile.runtime_inventory",
				"provider":  name,
				"error":     err.Error(),
			})
			continue
		}
		for _, runtime := range runtimes {
			if _, ok := known[runtime.ID]; ok {
				continue
			}
			if runtime.State == "" {
				runtime.State = string(StateRunning)
			}
			if SandboxState(runtime.State) == StateDestroyed {
				continue
			}
			if runtime.Provider == "" {
				runtime.Provider = prov.Name()
			}
			if runtime.Image == "" {
				runtime.Image = m.defaultImage
			}
			if runtime.CreatedAt.IsZero() {
				runtime.CreatedAt = time.Now().UTC()
			}
			expiresAt := runtime.CreatedAt.Add(m.defaultTTL)
			if expiresAt.Before(time.Now()) {
				expiresAt = time.Now().Add(m.defaultTTL)
			}

			metaJSON, _ := json.Marshal(runtime.Metadata)
			rec := &store.SandboxRecord{
				ID:        runtime.ID,
				State:     runtime.State,
				Provider:  runtime.Provider,
				Image:     runtime.Image,
				MemoryMB:  m.defaultMemory,
				VCPUs:     m.defaultVCPUs,
				Metadata:  string(metaJSON),
				CreatedAt: runtime.CreatedAt,
				ExpiresAt: expiresAt,
				UpdatedAt: time.Now().UTC(),
			}
			if err := m.store.CreateSandbox(ctx, rec); err != nil {
				if errors.Is(err, store.ErrConflict) {
					continue
				}
				return fmt.Errorf("adopting runtime sandbox %s: %w", runtime.ID, err)
			}

			sb := recordToSandbox(rec)
			sb.PreviewDomain = m.previewDomain
			m.mu.Lock()
			m.sandboxes[runtime.ID] = sb
			m.mu.Unlock()
			known[runtime.ID] = struct{}{}

			m.logger.Info().
				Str("sandbox", runtime.ID).
				Str("provider", runtime.Provider).
				Msg("reconcile: adopted provider runtime")
			m.publishOperationalEvent(EventReconcileAction, runtime.ID, map[string]interface{}{
				"action":   "adopted_runtime",
				"provider": runtime.Provider,
				"image":    runtime.Image,
			})
		}
	}
	return nil
}

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (*Sandbox, error) {
	start := time.Now()
	metricsProvider := req.Provider
	if metricsProvider == "" {
		metricsProvider = m.registry.Default()
	}
	var metricsErr error
	defer func() {
		m.recordOperation(OperationSpawn, metricsProvider, time.Since(start), metricsErr)
	}()

	providerName := req.Provider
	prov, err := m.registry.Get(providerName)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventProviderFailed, "", OperationSpawn, metricsProvider, err)
		return nil, fmt.Errorf("getting provider: %w", err)
	}
	metricsProvider = prov.Name()

	image := req.Image
	if image == "" {
		image = m.defaultImage
	}
	memMB := req.MemoryMB
	if memMB == 0 {
		memMB = m.defaultMemory
	}
	vcpus := req.VCPUs
	if vcpus == 0 {
		vcpus = m.defaultVCPUs
	}
	ttl := m.defaultTTL
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			return nil, fmt.Errorf("parsing TTL: %w", err)
		}
		ttl = parsed
	}
	if err := m.acquireSpawnAdmission(ctx, req.OwnerID, ttl, metricsProvider); err != nil {
		metricsErr = err
		m.publishFailureForError("", OperationSpawn, metricsProvider, err)
		return nil, err
	}
	defer m.admissionMu.Unlock()

	now := time.Now()

	// Pool mode: acquire a VM slot instead of spawning a new VM.
	if m.vmPoolMgr != nil {
		sb, err := m.spawnPooled(ctx, prov, req, image, memMB, vcpus, ttl, now)
		metricsErr = err
		if err != nil {
			m.publishFailureForError("", OperationSpawn, metricsProvider, err)
		}
		return sb, err
	}

	sb := &Sandbox{
		State:         StateCreating,
		Provider:      prov.Name(),
		Image:         image,
		MemoryMB:      memMB,
		VCPUs:         vcpus,
		OwnerID:       req.OwnerID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
		Metadata:      req.Metadata,
		PreviewDomain: m.previewDomain,
	}

	// Spawn via provider
	id, err := prov.Spawn(ctx, providers.SpawnOptions{
		Image:    image,
		MemoryMB: memMB,
		VCPUs:    vcpus,
		Metadata: req.Metadata,
	})
	if err != nil {
		metricsErr = err
		m.publishFailureForError("", OperationSpawn, metricsProvider, err)
		return nil, fmt.Errorf("spawning sandbox: %w", err)
	}
	sb.ID = id
	sb.State = StateRunning

	// Persist
	metaJSON, _ := json.Marshal(sb.Metadata)
	if err := m.store.CreateSandbox(ctx, &store.SandboxRecord{
		ID:        sb.ID,
		State:     string(sb.State),
		Provider:  sb.Provider,
		Image:     sb.Image,
		MemoryMB:  sb.MemoryMB,
		VCPUs:     sb.VCPUs,
		Metadata:  string(metaJSON),
		OwnerID:   sb.OwnerID,
		VMID:      sb.VMID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		// Best effort: destroy the sandbox if DB write fails
		prov.Destroy(ctx, id)
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, id, OperationSpawn, metricsProvider, err)
		return nil, fmt.Errorf("persisting sandbox: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	m.events.Publish(Event{
		Type:      EventSandboxCreated,
		SandboxID: id,
	})
	m.events.Publish(Event{
		Type:      EventSandboxRunning,
		SandboxID: id,
	})

	m.logger.Info().Str("sandbox", id).Str("provider", prov.Name()).Str("image", image).Msg("sandbox spawned")
	return sb, nil
}

// spawnPooled creates a logical sandbox on a shared pool VM.
func (m *Manager) spawnPooled(ctx context.Context, prov providers.Provider, req SpawnRequest, image string, memMB, vcpus int, ttl time.Duration, now time.Time) (*Sandbox, error) {
	// Generate a logical sandbox ID (not tied to a VM).
	b := make([]byte, 4)
	rand.Read(b)
	sandboxID := fmt.Sprintf("sb-%08x", b)

	// Acquire a VM slot (may spawn a new VM if needed).
	vmID, err := m.vmPoolMgr.Acquire(ctx, sandboxID)
	if err != nil {
		if errors.Is(err, ErrVMPoolFull) {
			return nil, providers.ResourceLimitError(err.Error())
		}
		return nil, fmt.Errorf("pool acquire: %w", err)
	}

	sb := &Sandbox{
		ID:            sandboxID,
		State:         StateRunning,
		Provider:      prov.Name(),
		Image:         image,
		MemoryMB:      memMB,
		VCPUs:         vcpus,
		OwnerID:       req.OwnerID,
		VMID:          vmID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
		Metadata:      req.Metadata,
		PreviewDomain: m.previewDomain,
	}

	// Create the sandbox's isolated workspace on the VM.
	workspaceDir := "/workspace/" + sandboxID
	_, err = prov.Exec(ctx, vmID, providers.ExecOptions{
		Command: "mkdir -p " + workspaceDir,
	})
	if err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationSpawn, prov.Name(), err)
		return nil, fmt.Errorf("creating workspace: %w", err)
	}

	// Persist
	metaJSON, _ := json.Marshal(sb.Metadata)
	if err := m.store.CreateSandbox(ctx, &store.SandboxRecord{
		ID:        sb.ID,
		State:     string(sb.State),
		Provider:  sb.Provider,
		Image:     sb.Image,
		MemoryMB:  sb.MemoryMB,
		VCPUs:     sb.VCPUs,
		Metadata:  string(metaJSON),
		OwnerID:   sb.OwnerID,
		VMID:      sb.VMID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationSpawn, prov.Name(), err)
		return nil, fmt.Errorf("persisting sandbox: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[sandboxID] = sb
	m.mu.Unlock()

	m.events.Publish(Event{Type: EventSandboxCreated, SandboxID: sandboxID})
	m.events.Publish(Event{Type: EventSandboxRunning, SandboxID: sandboxID})

	m.logger.Info().
		Str("sandbox", sandboxID).
		Str("vm_id", vmID).
		Str("owner", req.OwnerID).
		Msg("pooled sandbox spawned")
	return sb, nil
}

func (m *Manager) Exec(ctx context.Context, sandboxID string, req ExecRequest) (*ExecResult, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationExec, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventExecFailed, sandboxID, OperationExec, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	execCtx := ctx
	var cancel context.CancelFunc
	timeout, err := m.resolveExecTimeout(req.Timeout, sb.OwnerID)
	if err != nil {
		metricsErr = err
		m.publishFailureForError(sandboxID, OperationExec, metricsProvider, err)
		return nil, err
	}
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	m.events.Publish(Event{
		Type:      EventExecStarted,
		SandboxID: sandboxID,
	})

	// In pool mode, default workdir to the sandbox's workspace.
	workDir := req.WorkDir
	if workDir == "" && sb.VMID != "" {
		workDir = "/workspace/" + sandboxID
	}

	execStart := time.Now()
	result, err := prov.Exec(execCtx, m.resolveVMID(sb), providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: workDir,
	})
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			metricsErr = providers.ExecTimeoutError(sandboxID)
			m.publishOperationFailure(EventExecTimeout, sandboxID, OperationExec, metricsProvider, metricsErr)
			return nil, metricsErr
		}
		metricsErr = err
		m.publishOperationFailure(EventExecFailed, sandboxID, OperationExec, metricsProvider, err)
		return nil, fmt.Errorf("exec: %w", err)
	}
	duration := time.Since(execStart)

	execResult := &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Duration: duration.String(),
	}

	// Log to store
	m.store.CreateExecLog(ctx, &store.ExecLogRecord{
		SandboxID: sandboxID,
		Command:   req.Command,
		ExitCode:  result.ExitCode,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		Duration:  duration.String(),
		CreatedAt: time.Now(),
	})

	m.events.Publish(Event{
		Type:      EventExecCompleted,
		SandboxID: sandboxID,
	})

	return execResult, nil
}

func (m *Manager) ExecStream(ctx context.Context, sandboxID string, req ExecRequest) (<-chan providers.StreamChunk, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		if metricsErr != nil {
			m.recordOperation(OperationExecStream, metricsProvider, time.Since(start), metricsErr)
		}
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventExecFailed, sandboxID, OperationExecStream, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	execCtx := ctx
	var cancel context.CancelFunc
	timeout, err := m.resolveExecTimeout(req.Timeout, sb.OwnerID)
	if err != nil {
		metricsErr = err
		m.publishFailureForError(sandboxID, OperationExecStream, metricsProvider, err)
		return nil, err
	}
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
	}

	workDir := req.WorkDir
	if workDir == "" && sb.VMID != "" {
		workDir = "/workspace/" + sandboxID
	}

	ch, err := prov.ExecStream(execCtx, m.resolveVMID(sb), providers.ExecOptions{
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		WorkDir: workDir,
	})
	if err != nil {
		if cancel != nil {
			cancel()
		}
		if execCtx.Err() == context.DeadlineExceeded {
			metricsErr = providers.ExecTimeoutError(sandboxID)
			m.publishOperationFailure(EventExecTimeout, sandboxID, OperationExecStream, metricsProvider, metricsErr)
			return nil, metricsErr
		}
		metricsErr = err
		m.publishOperationFailure(EventExecFailed, sandboxID, OperationExecStream, metricsProvider, err)
		return nil, err
	}
	if cancel == nil {
		out := make(chan providers.StreamChunk, 64)
		go func() {
			defer close(out)
			for chunk := range ch {
				out <- chunk
			}
			m.recordOperation(OperationExecStream, metricsProvider, time.Since(start), nil)
		}()
		return out, nil
	}

	out := make(chan providers.StreamChunk, 64)
	go func() {
		defer close(out)
		defer cancel()
		timedOut := false
		for chunk := range ch {
			select {
			case out <- chunk:
			case <-execCtx.Done():
				timedOut = true
			}
		}
		if execCtx.Err() == context.DeadlineExceeded || timedOut {
			metricsErr = providers.ExecTimeoutError(sandboxID)
			m.publishOperationFailure(EventExecTimeout, sandboxID, OperationExecStream, metricsProvider, metricsErr)
			select {
			case out <- providers.StreamChunk{Stream: "stderr", Data: metricsErr.Error()}:
			case <-ctx.Done():
			}
		}
		m.recordOperation(OperationExecStream, metricsProvider, time.Since(start), metricsErr)
	}()
	return out, nil
}

func (m *Manager) WriteFile(ctx context.Context, sandboxID string, req FileWriteRequest) error {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileWrite, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileWrite, metricsProvider, err)
		return err
	}
	metricsProvider = sb.Provider

	mode := req.Mode
	if mode == "" {
		mode = "0644"
	}

	path := m.scopedPath(sb, req.Path)
	if err := prov.WriteFile(ctx, m.resolveVMID(sb), path, strings.NewReader(req.Content), mode); err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileWrite, metricsProvider, err)
		return fmt.Errorf("writing file: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileWritten,
		SandboxID: sandboxID,
	})
	return nil
}

func (m *Manager) ReadFile(ctx context.Context, sandboxID string, path string) ([]byte, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileRead, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	rc, err := prov.ReadFile(ctx, m.resolveVMID(sb), m.scopedPath(sb, path))
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
		return nil, fmt.Errorf("reading file: %w", err)
	}
	defer rc.Close()

	buf, err := io.ReadAll(rc)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
		return nil, fmt.Errorf("reading file content: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileRead,
		SandboxID: sandboxID,
	})
	return buf, nil
}

func (m *Manager) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileList, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileList, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	pFiles, err := prov.ListFiles(ctx, m.resolveVMID(sb), m.scopedPath(sb, path))
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileList, metricsProvider, err)
		return nil, err
	}

	files := make([]FileInfo, len(pFiles))
	for i, f := range pFiles {
		files[i] = FileInfo{
			Path:    f.Path,
			Size:    f.Size,
			Mode:    f.Mode,
			IsDir:   f.IsDir,
			ModTime: f.ModTime,
		}
	}
	return files, nil
}

func (m *Manager) DeleteFile(ctx context.Context, sandboxID string, req FileDeleteRequest) error {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileDelete, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileDelete, metricsProvider, err)
		return err
	}
	metricsProvider = sb.Provider

	path := m.scopedPath(sb, req.Path)
	metricsErr = prov.DeleteFile(ctx, m.resolveVMID(sb), path, req.Recursive)
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileDelete, metricsProvider, metricsErr)
	}
	return metricsErr
}

func (m *Manager) MoveFile(ctx context.Context, sandboxID string, req FileMoveRequest) error {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileMove, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileMove, metricsProvider, err)
		return err
	}
	metricsProvider = sb.Provider

	oldPath := m.scopedPath(sb, req.OldPath)
	newPath := m.scopedPath(sb, req.NewPath)
	metricsErr = prov.MoveFile(ctx, m.resolveVMID(sb), oldPath, newPath)
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileMove, metricsProvider, metricsErr)
	}
	return metricsErr
}

func (m *Manager) ChmodFile(ctx context.Context, sandboxID string, req FileChmodRequest) error {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileChmod, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileChmod, metricsProvider, err)
		return err
	}
	metricsProvider = sb.Provider

	path := m.scopedPath(sb, req.Path)
	metricsErr = prov.ChmodFile(ctx, m.resolveVMID(sb), path, req.Mode)
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileChmod, metricsProvider, metricsErr)
	}
	return metricsErr
}

func (m *Manager) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileStat, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileStat, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	scopedPath := m.scopedPath(sb, path)
	fi, err := prov.StatFile(ctx, m.resolveVMID(sb), scopedPath)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileStat, metricsProvider, err)
		return nil, err
	}
	return &FileInfo{
		Path:    fi.Path,
		Size:    fi.Size,
		Mode:    fi.Mode,
		IsDir:   fi.IsDir,
		ModTime: fi.ModTime,
	}, nil
}

func (m *Manager) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationFileGlob, metricsProvider, time.Since(start), metricsErr)
	}()

	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileGlob, metricsProvider, err)
		return nil, err
	}
	metricsProvider = sb.Provider

	scopedPattern := m.scopedPath(sb, pattern)
	matches, err := prov.GlobFiles(ctx, m.resolveVMID(sb), scopedPattern)
	metricsErr = err
	if err != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileGlob, metricsProvider, err)
	}
	return matches, err
}

// scopedPath prefixes a path with the sandbox workspace when running in pool mode.
func (m *Manager) scopedPath(sb *Sandbox, path string) string {
	if sb.VMID == "" {
		return path // dedicated VM, no scoping
	}
	base := "/workspace/" + sb.ID
	cleaned := filepath.Clean(filepath.Join(base, path))
	if !strings.HasPrefix(cleaned, base+"/") && cleaned != base {
		return base
	}
	return cleaned
}

// resolveVMID returns the VM sandbox ID for provider calls.
// In pool mode, operations go to the VM (sb.VMID); in 1:1 mode, to the sandbox itself.
func (m *Manager) resolveVMID(sb *Sandbox) string {
	if sb.VMID != "" {
		return sb.VMID
	}
	return sb.ID
}

// VMPoolStatus returns the current VM pool status. Returns nil if pool is disabled.
func (m *Manager) VMPoolStatus() *VMPoolStatus {
	if m.vmPoolMgr == nil {
		return nil
	}
	status := m.vmPoolMgr.Status()
	return &status
}

func (m *Manager) OperationMetrics() []OperationMetrics {
	return m.metrics.Snapshot()
}

func (m *Manager) Limits() OperationalLimitsInfo {
	return OperationalLimitsInfo{
		MaxSandboxes:         m.limits.MaxSandboxes,
		MaxSandboxesPerOwner: m.limits.MaxSandboxesPerOwner,
		DefaultExecTimeout:   m.limits.DefaultExecTimeout.String(),
		MaxExecTimeout:       m.limits.MaxExecTimeout.String(),
		MaxTTL:               m.limits.MaxTTL.String(),
		SpawnOverflow:        m.limits.SpawnOverflow,
		SpawnQueueTimeout:    m.limits.SpawnQueueTimeout.String(),
		MaxSpawnQueue:        m.limits.MaxSpawnQueue,
	}
}

func (m *Manager) SchedulerStatus() SchedulerStatus {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	return SchedulerStatus{
		SpawnOverflow:     m.limits.SpawnOverflow,
		SpawnQueueDepth:   m.queueWaiters,
		MaxSpawnQueue:     m.limits.MaxSpawnQueue,
		SpawnQueueTimeout: m.limits.SpawnQueueTimeout.String(),
		AdmissionControl:  "single_node",
	}
}

func (m *Manager) GetOwnerQuota(ctx context.Context, ownerID string) (*OwnerQuota, error) {
	ownerID, err := normalizeOwnerID(ownerID)
	if err != nil {
		return nil, err
	}
	rec, err := m.store.GetOwnerQuota(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return ownerQuotaFromRecord(rec), nil
}

func (m *Manager) ListOwnerQuotas(ctx context.Context) ([]*OwnerQuota, error) {
	records, err := m.store.ListOwnerQuotas(ctx)
	if err != nil {
		return nil, err
	}
	quotas := make([]*OwnerQuota, 0, len(records))
	for _, rec := range records {
		quotas = append(quotas, ownerQuotaFromRecord(rec))
	}
	return quotas, nil
}

func (m *Manager) QuotaSummary(ctx context.Context) (QuotaSummary, error) {
	records, err := m.store.ListOwnerQuotas(ctx)
	if err != nil {
		return QuotaSummary{}, err
	}
	summary := QuotaSummary{Total: len(records)}
	for _, quota := range records {
		if quota.MaxSandboxes > 0 {
			summary.WithMaxSandboxes++
		}
		if quota.MaxTTLSeconds > 0 {
			summary.WithMaxTTL++
		}
		if quota.MaxExecTimeoutSeconds > 0 {
			summary.WithMaxExecTimeout++
		}
	}
	return summary, nil
}

func (m *Manager) SaveOwnerQuota(ctx context.Context, quota OwnerQuota) (*OwnerQuota, error) {
	ownerID, err := normalizeOwnerID(quota.OwnerID)
	if err != nil {
		return nil, err
	}
	quota.OwnerID = ownerID
	maxTTL, err := parseOptionalDurationSeconds(quota.MaxTTL)
	if err != nil {
		return nil, InvalidInputError(fmt.Sprintf("parsing max_ttl: %v", err))
	}
	maxExecTimeout, err := parseOptionalDurationSeconds(quota.MaxExecTimeout)
	if err != nil {
		return nil, InvalidInputError(fmt.Sprintf("parsing max_exec_timeout: %v", err))
	}
	if quota.MaxSandboxes < 0 {
		return nil, InvalidInputError("max_sandboxes cannot be negative")
	}
	rec := &store.OwnerQuotaRecord{
		OwnerID:               quota.OwnerID,
		MaxSandboxes:          quota.MaxSandboxes,
		MaxTTLSeconds:         maxTTL,
		MaxExecTimeoutSeconds: maxExecTimeout,
	}
	if err := m.store.SaveOwnerQuota(ctx, rec); err != nil {
		return nil, err
	}
	saved, err := m.GetOwnerQuota(ctx, quota.OwnerID)
	if err != nil {
		return nil, err
	}
	m.publishOperationalEvent(EventQuotaSaved, "", map[string]interface{}{
		"owner_id":         saved.OwnerID,
		"max_sandboxes":    saved.MaxSandboxes,
		"max_ttl":          saved.MaxTTL,
		"max_exec_timeout": saved.MaxExecTimeout,
	})
	return saved, nil
}

func (m *Manager) DeleteOwnerQuota(ctx context.Context, ownerID string) error {
	ownerID, err := normalizeOwnerID(ownerID)
	if err != nil {
		return err
	}
	if err := m.store.DeleteOwnerQuota(ctx, ownerID); err != nil {
		return err
	}
	m.publishOperationalEvent(EventQuotaDeleted, "", map[string]interface{}{
		"owner_id": ownerID,
	})
	return nil
}

func (m *Manager) OwnerUsage(ctx context.Context, ownerID string) (*OwnerUsage, error) {
	ownerID, err := normalizeOwnerID(ownerID)
	if err != nil {
		return nil, err
	}
	records, err := m.store.ListSandboxesByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	usage := &OwnerUsage{
		OwnerID:         ownerID,
		ActiveSandboxes: len(records),
		MaxSandboxes:    m.limits.MaxSandboxesPerOwner,
		MaxTTL:          m.limits.MaxTTL.String(),
		MaxExecTimeout:  m.limits.MaxExecTimeout.String(),
	}
	if quota, err := m.store.GetOwnerQuota(ctx, ownerID); err == nil {
		usage.QuotaConfigured = true
		if quota.MaxSandboxes > 0 {
			usage.MaxSandboxes = quota.MaxSandboxes
		}
		if quota.MaxTTLSeconds > 0 {
			usage.MaxTTL = (time.Duration(quota.MaxTTLSeconds) * time.Second).String()
		}
		if quota.MaxExecTimeoutSeconds > 0 {
			usage.MaxExecTimeout = (time.Duration(quota.MaxExecTimeoutSeconds) * time.Second).String()
		}
	}
	return usage, nil
}

func (m *Manager) recordOperation(operation, provider string, duration time.Duration, err error) {
	if m.metrics == nil {
		return
	}
	m.metrics.RecordOperation(operation, provider, duration, err)
}

func (m *Manager) publishFailureForError(sandboxID, operation, provider string, err error) {
	if errors.Is(err, providers.ErrResourceLimit) {
		m.publishOperationFailure(EventResourceLimit, sandboxID, operation, provider, err)
		return
	}
	if errors.Is(err, providers.ErrProviderUnavailable) || errors.Is(err, providers.ErrProviderNotFound) {
		m.publishOperationFailure(EventProviderFailed, sandboxID, operation, provider, err)
		return
	}
	m.publishOperationFailure(EventOperationFailed, sandboxID, operation, provider, err)
}

func (m *Manager) publishOperationFailure(eventType EventType, sandboxID, operation, provider string, err error) {
	if err == nil {
		return
	}
	m.publishOperationalEvent(eventType, sandboxID, map[string]interface{}{
		"operation": operation,
		"provider":  provider,
		"error":     err.Error(),
	})
}

func (m *Manager) publishOperationalEvent(eventType EventType, sandboxID string, data map[string]interface{}) {
	if m.events == nil {
		return
	}
	payload, _ := json.Marshal(data)
	m.events.Publish(Event{
		Type:      eventType,
		SandboxID: sandboxID,
		Data:      payload,
	})
}

func (m *Manager) acquireSpawnAdmission(ctx context.Context, ownerID string, ttl time.Duration, provider string) error {
	for {
		if err := m.waitForSpawnCapacity(ctx, ownerID, ttl, provider); err != nil {
			return err
		}

		m.admissionMu.Lock()
		decision, err := m.EvaluateSpawnAdmission(ctx, ownerID, ttl)
		if err != nil {
			m.admissionMu.Unlock()
			return err
		}
		if decision.Allowed {
			return nil
		}
		m.admissionMu.Unlock()

		if !decision.Queueable || !strings.EqualFold(m.limits.SpawnOverflow, "queue") {
			return spawnAdmissionError(decision)
		}
	}
}

func (m *Manager) waitForSpawnCapacity(ctx context.Context, ownerID string, ttl time.Duration, provider string) error {
	decision, err := m.EvaluateSpawnAdmission(ctx, ownerID, ttl)
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	if !decision.Queueable || !strings.EqualFold(m.limits.SpawnOverflow, "queue") {
		return spawnAdmissionError(decision)
	}

	m.queueMu.Lock()
	if m.queueWaiters >= m.limits.MaxSpawnQueue {
		m.queueMu.Unlock()
		return providers.ResourceLimitError(fmt.Sprintf("spawn queue full (%d)", m.limits.MaxSpawnQueue))
	}
	m.queueWaiters++
	depth := m.queueWaiters
	capacityCh := m.capacityCh
	m.queueMu.Unlock()

	m.publishOperationalEvent(EventSpawnQueued, "", map[string]interface{}{
		"operation": OperationSpawn,
		"provider":  provider,
		"owner_id":  ownerID,
		"depth":     depth,
	})
	defer func() {
		m.queueMu.Lock()
		m.queueWaiters--
		m.queueMu.Unlock()
	}()

	waitCtx := ctx
	cancel := func() {}
	if m.limits.SpawnQueueTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, m.limits.SpawnQueueTimeout)
	}
	defer cancel()

	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				err := providers.ResourceLimitError(fmt.Sprintf("spawn queue timeout after %s", m.limits.SpawnQueueTimeout))
				m.publishOperationalEvent(EventSpawnQueueTimeout, "", map[string]interface{}{
					"operation": OperationSpawn,
					"provider":  provider,
					"owner_id":  ownerID,
					"error":     err.Error(),
				})
				return err
			}
			return waitCtx.Err()
		case <-capacityCh:
			decision, err = m.EvaluateSpawnAdmission(ctx, ownerID, ttl)
			if err != nil {
				return err
			}
			if decision.Allowed {
				m.publishOperationalEvent(EventSpawnDequeued, "", map[string]interface{}{
					"operation": OperationSpawn,
					"provider":  provider,
					"owner_id":  ownerID,
				})
				return nil
			}
			if !decision.Queueable {
				return spawnAdmissionError(decision)
			}
			m.queueMu.Lock()
			capacityCh = m.capacityCh
			m.queueMu.Unlock()
		}
	}
}

func (m *Manager) notifySpawnCapacity() {
	m.queueMu.Lock()
	close(m.capacityCh)
	m.capacityCh = make(chan struct{})
	m.queueMu.Unlock()
}

func (m *Manager) EvaluateSpawnAdmission(ctx context.Context, ownerID string, ttl time.Duration) (SpawnAdmissionDecision, error) {
	maxTTL, maxPerOwner := m.ownerLimitOverrides(ctx, ownerID)
	decision := SpawnAdmissionDecision{
		Allowed:           true,
		MaxSandboxes:      m.limits.MaxSandboxes,
		MaxOwnerSandboxes: maxPerOwner,
	}
	if maxTTL > 0 {
		decision.MaxTTL = maxTTL.String()
	}
	if maxTTL > 0 && ttl > maxTTL {
		decision.Allowed = false
		decision.Reason = "max_ttl"
		return decision, nil
	}

	if m.limits.MaxSandboxes <= 0 && (ownerID == "" || maxPerOwner <= 0) {
		return decision, nil
	}

	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return SpawnAdmissionDecision{}, fmt.Errorf("checking sandbox limits: %w", err)
	}
	total := 0
	ownerTotal := 0
	for _, rec := range records {
		if SandboxState(rec.State) == StateDestroyed {
			continue
		}
		total++
		if ownerID != "" && rec.OwnerID == ownerID {
			ownerTotal++
		}
	}
	decision.ActiveSandboxes = total
	decision.ActiveOwnerSandboxes = ownerTotal
	if m.limits.MaxSandboxes > 0 && total >= m.limits.MaxSandboxes {
		decision.Allowed = false
		decision.Queueable = true
		decision.Reason = "max_sandboxes"
		return decision, nil
	}
	if ownerID != "" && maxPerOwner > 0 && ownerTotal >= maxPerOwner {
		decision.Allowed = false
		decision.Queueable = true
		decision.Reason = "max_sandboxes_per_owner"
		return decision, nil
	}
	return decision, nil
}

func spawnAdmissionError(decision SpawnAdmissionDecision) error {
	switch decision.Reason {
	case "max_ttl":
		return providers.ResourceLimitError(fmt.Sprintf("ttl exceeds max ttl %s", decision.MaxTTL))
	case "max_sandboxes":
		return providers.ResourceLimitError(fmt.Sprintf("max sandboxes reached (%d)", decision.MaxSandboxes))
	case "max_sandboxes_per_owner":
		return providers.ResourceLimitError(fmt.Sprintf("max sandboxes per owner reached (%d)", decision.MaxOwnerSandboxes))
	default:
		return providers.ResourceLimitError("spawn admission denied")
	}
}

func (m *Manager) resolveExecTimeout(raw string, ownerID string) (time.Duration, error) {
	timeout := m.limits.DefaultExecTimeout
	if raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("parsing exec timeout: %w", err)
		}
		timeout = parsed
	}
	if timeout < 0 {
		return 0, providers.ResourceLimitError("exec timeout cannot be negative")
	}
	maxExecTimeout := m.ownerMaxExecTimeout(context.Background(), ownerID)
	if maxExecTimeout > 0 && timeout > maxExecTimeout {
		return 0, providers.ResourceLimitError(fmt.Sprintf("exec timeout %s exceeds max exec timeout %s", timeout, maxExecTimeout))
	}
	return timeout, nil
}

func (m *Manager) ownerLimitOverrides(ctx context.Context, ownerID string) (time.Duration, int) {
	maxTTL := m.limits.MaxTTL
	maxPerOwner := m.limits.MaxSandboxesPerOwner
	if ownerID == "" {
		return maxTTL, maxPerOwner
	}
	quota, err := m.store.GetOwnerQuota(ctx, ownerID)
	if err != nil {
		return maxTTL, maxPerOwner
	}
	if quota.MaxTTLSeconds > 0 {
		maxTTL = time.Duration(quota.MaxTTLSeconds) * time.Second
	}
	if quota.MaxSandboxes > 0 {
		maxPerOwner = quota.MaxSandboxes
	}
	return maxTTL, maxPerOwner
}

func (m *Manager) ownerMaxExecTimeout(ctx context.Context, ownerID string) time.Duration {
	maxExecTimeout := m.limits.MaxExecTimeout
	if ownerID == "" {
		return maxExecTimeout
	}
	quota, err := m.store.GetOwnerQuota(ctx, ownerID)
	if err != nil {
		return maxExecTimeout
	}
	if quota.MaxExecTimeoutSeconds > 0 {
		return time.Duration(quota.MaxExecTimeoutSeconds) * time.Second
	}
	return maxExecTimeout
}

func ownerQuotaFromRecord(rec *store.OwnerQuotaRecord) *OwnerQuota {
	return &OwnerQuota{
		OwnerID:        rec.OwnerID,
		MaxSandboxes:   rec.MaxSandboxes,
		MaxTTL:         optionalSecondsString(rec.MaxTTLSeconds),
		MaxExecTimeout: optionalSecondsString(rec.MaxExecTimeoutSeconds),
		CreatedAt:      rec.CreatedAt,
		UpdatedAt:      rec.UpdatedAt,
	}
}

func parseOptionalDurationSeconds(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" || raw == "0s" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration cannot be negative")
	}
	if d > 0 && d < time.Second {
		return 0, fmt.Errorf("duration must be at least 1s")
	}
	if d%time.Second != 0 {
		return 0, fmt.Errorf("duration must use whole seconds")
	}
	return int64(d.Seconds()), nil
}

func normalizeOwnerID(ownerID string) (string, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return "", InvalidInputError("owner_id is required")
	}
	if len(ownerID) > 128 {
		return "", InvalidInputError("owner_id must be 128 characters or fewer")
	}
	if strings.ContainsAny(ownerID, `/\`) {
		return "", InvalidInputError("owner_id cannot contain path separators")
	}
	for _, r := range ownerID {
		if r <= 31 || r == 127 || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return "", InvalidInputError("owner_id cannot contain whitespace or control characters")
		}
	}
	return ownerID, nil
}

func optionalSecondsString(seconds int64) string {
	if seconds <= 0 {
		return "0s"
	}
	return (time.Duration(seconds) * time.Second).String()
}

// InitVMPool initializes the VM pool manager if pool mode is enabled.
func (m *Manager) InitVMPool() {
	if !m.poolConfig.Enabled {
		return
	}
	// Use spawnDirect (bypasses pool check) to avoid infinite recursion.
	m.vmPoolMgr = NewVMPoolManager(m.poolConfig, m.spawnDirect, m.logger)
	m.logger.Info().Int("max_vms", m.poolConfig.MaxVMs).Int("max_users_per_vm", m.poolConfig.MaxUsersPerVM).Msg("VM pool mode enabled")
}

// spawnDirect spawns a VM directly via the provider, bypassing pool logic.
// Used by the pool manager to create pool VMs without recursion.
func (m *Manager) spawnDirect(ctx context.Context, req SpawnRequest) (*Sandbox, error) {
	providerName := req.Provider
	prov, err := m.registry.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("getting provider: %w", err)
	}

	image := req.Image
	if image == "" {
		image = m.defaultImage
	}
	memMB := req.MemoryMB
	if memMB == 0 {
		memMB = m.defaultMemory
	}
	vcpus := req.VCPUs
	if vcpus == 0 {
		vcpus = m.defaultVCPUs
	}

	now := time.Now()
	id, err := prov.Spawn(ctx, providers.SpawnOptions{
		Image:    image,
		MemoryMB: memMB,
		VCPUs:    vcpus,
	})
	if err != nil {
		return nil, fmt.Errorf("spawning VM: %w", err)
	}

	sb := &Sandbox{
		ID:            id,
		State:         StateRunning,
		Provider:      prov.Name(),
		Image:         image,
		MemoryMB:      memMB,
		VCPUs:         vcpus,
		CreatedAt:     now,
		ExpiresAt:     now.Add(m.defaultTTL),
		PreviewDomain: m.previewDomain,
	}

	metaJSON, _ := json.Marshal(sb.Metadata)
	if err := m.store.CreateSandbox(ctx, &store.SandboxRecord{
		ID:        sb.ID,
		State:     string(sb.State),
		Provider:  sb.Provider,
		Image:     sb.Image,
		MemoryMB:  sb.MemoryMB,
		VCPUs:     sb.VCPUs,
		Metadata:  string(metaJSON),
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		prov.Destroy(ctx, id)
		return nil, fmt.Errorf("persisting pool VM: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	m.logger.Info().Str("vm_id", id).Str("image", image).Msg("pool VM spawned")
	return sb, nil
}

func (m *Manager) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	_, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}
	return prov.ConsoleLog(ctx, sandboxID, lines)
}

func (m *Manager) Get(ctx context.Context, id string) (*Sandbox, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if ok {
		return sb, nil
	}

	// Fall back to store
	rec, err := m.store.GetSandbox(ctx, id)
	if err != nil {
		return nil, providers.SandboxNotFoundError(id)
	}
	sb = recordToSandbox(rec)
	if sb.State == StateDestroyed {
		return nil, providers.SandboxDestroyedError(id)
	}
	return sb, nil
}

func (m *Manager) CountByProvider(_ context.Context, provider string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, sb := range m.sandboxes {
		if sb.Provider == provider {
			count++
		}
	}
	return count
}

func (m *Manager) List(ctx context.Context) ([]*Sandbox, error) {
	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	sandboxes := make([]*Sandbox, len(records))
	for i, r := range records {
		sandboxes[i] = recordToSandbox(r)
	}
	return sandboxes, nil
}

func (m *Manager) Destroy(ctx context.Context, id string) error {
	start := time.Now()
	metricsProvider := "unknown"
	var metricsErr error
	defer func() {
		m.recordOperation(OperationDestroy, metricsProvider, time.Since(start), metricsErr)
	}()

	// Check if this is a pooled sandbox.
	m.mu.RLock()
	sb := m.sandboxes[id]
	m.mu.RUnlock()
	if sb != nil {
		metricsProvider = sb.Provider
	}

	if sb != nil && sb.VMID != "" && m.vmPoolMgr != nil {
		metricsErr = m.destroyPooled(ctx, id, sb)
		if metricsErr != nil {
			m.publishOperationFailure(EventOperationFailed, id, OperationDestroy, metricsProvider, metricsErr)
		}
		return metricsErr
	}

	prov, err := m.getProvider(id)
	if err != nil {
		// If we can't find the provider, just update the store
		m.store.DeleteSandbox(ctx, id)
		m.mu.Lock()
		delete(m.sandboxes, id)
		m.mu.Unlock()
		m.notifySpawnCapacity()
		return nil
	}
	if sb != nil {
		metricsProvider = sb.Provider
	}

	if err := prov.Destroy(ctx, id); err != nil {
		// Debug-level: this is expected when VMs were killed externally (e.g. process restart).
		m.logger.Debug().Err(err).Str("sandbox", id).Msg("provider destroy failed (VM may already be gone)")
		m.publishOperationFailure(EventProviderFailed, id, OperationDestroy, metricsProvider, err)
	}

	m.store.UpdateSandboxState(ctx, id, string(StateDestroyed))
	m.mu.Lock()
	if sb, ok := m.sandboxes[id]; ok {
		sb.State = StateDestroyed
	}
	delete(m.sandboxes, id)
	m.mu.Unlock()

	m.events.Publish(Event{
		Type:      EventSandboxDestroyed,
		SandboxID: id,
	})
	m.notifySpawnCapacity()

	m.logger.Info().Str("sandbox", id).Msg("sandbox destroyed")
	return nil
}

// destroyPooled cleans up a pooled sandbox's workspace and releases the VM slot.
func (m *Manager) destroyPooled(ctx context.Context, id string, sb *Sandbox) error {
	vmID := sb.VMID

	// Clean up workspace on the VM.
	prov, err := m.registry.Get(sb.Provider)
	if err == nil {
		workspaceDir := "/workspace/" + id
		_ = prov.DeleteFile(ctx, vmID, workspaceDir, true)
	}

	// Release the VM slot; if VM is now empty, destroy it.
	shouldDestroy := m.vmPoolMgr.Release(vmID, id)
	if shouldDestroy {
		if prov != nil {
			if err := prov.Destroy(ctx, vmID); err != nil {
				m.logger.Debug().Err(err).Str("vm_id", vmID).Msg("pool VM destroy failed")
			}
		}
		// Clean up the VM's sandbox record too.
		m.store.DeleteSandbox(ctx, vmID)
		m.mu.Lock()
		delete(m.sandboxes, vmID)
		m.mu.Unlock()
		m.logger.Info().Str("vm_id", vmID).Msg("empty pool VM destroyed")
	}

	// Clean up the logical sandbox record.
	m.store.UpdateSandboxState(ctx, id, string(StateDestroyed))
	m.mu.Lock()
	if cached, ok := m.sandboxes[id]; ok {
		cached.State = StateDestroyed
	}
	delete(m.sandboxes, id)
	m.mu.Unlock()

	m.events.Publish(Event{Type: EventSandboxDestroyed, SandboxID: id})
	m.notifySpawnCapacity()
	m.logger.Info().Str("sandbox", id).Str("vm_id", vmID).Msg("pooled sandbox destroyed")
	return nil
}

func (m *Manager) ExtendTTL(ctx context.Context, id string, extra time.Duration) (*Sandbox, error) {
	sb, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	newExpiresAt := time.Now().Add(extra)
	if err := m.store.UpdateSandboxExpiresAt(ctx, id, newExpiresAt); err != nil {
		return nil, fmt.Errorf("extending TTL: %w", err)
	}

	sb.ExpiresAt = newExpiresAt

	m.mu.Lock()
	if cached, ok := m.sandboxes[id]; ok {
		cached.ExpiresAt = newExpiresAt
	}
	m.mu.Unlock()

	m.logger.Info().Str("sandbox", id).Time("new_expires_at", newExpiresAt).Msg("TTL extended")
	return sb, nil
}

func (m *Manager) Prune(ctx context.Context) (int, error) {
	sandboxes, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()
	for _, sb := range sandboxes {
		if sb.ExpiresAt.Before(now) {
			if err := m.Destroy(ctx, sb.ID); err != nil {
				m.logger.Error().Err(err).Str("sandbox", sb.ID).Msg("prune destroy failed")
				continue
			}
			count++
		}
	}
	return count, nil
}

func (m *Manager) getSandboxAndProvider(id string) (*Sandbox, providers.Provider, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if !ok {
		rec, err := m.store.GetSandbox(context.Background(), id)
		if err != nil {
			return nil, nil, providers.SandboxNotFoundError(id)
		}
		sb = recordToSandbox(rec)
	}

	if sb.State == StateDestroyed {
		return nil, nil, providers.SandboxDestroyedError(id)
	}

	prov, err := m.registry.Get(sb.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("provider %q: %w", sb.Provider, err)
	}

	return sb, prov, nil
}

func (m *Manager) getProvider(id string) (providers.Provider, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if !ok {
		rec, err := m.store.GetSandbox(context.Background(), id)
		if err != nil {
			return nil, providers.SandboxNotFoundError(id)
		}
		sb = recordToSandbox(rec)
	}

	return m.registry.Get(sb.Provider)
}

func recordToSandbox(r *store.SandboxRecord) *Sandbox {
	var metadata map[string]string
	json.Unmarshal([]byte(r.Metadata), &metadata)
	return &Sandbox{
		ID:        r.ID,
		State:     SandboxState(r.State),
		Provider:  r.Provider,
		Image:     r.Image,
		MemoryMB:  r.MemoryMB,
		VCPUs:     r.VCPUs,
		OwnerID:   r.OwnerID,
		VMID:      r.VMID,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
		Metadata:  metadata,
	}
}
