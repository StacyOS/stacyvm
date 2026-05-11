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

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/StacyOs/stacyvm/internal/worker"
	"github.com/StacyOs/stacyvm/internal/workerproto"
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
	queueStats   spawnQueueStats

	defaultTTL            time.Duration
	defaultImage          string
	defaultMemory         int
	defaultVCPUs          int
	limits                OperationalLimits
	workerID              string
	workerToken           string
	workerSigningKey      string
	workerRevokedTokenIDs []string
	workerRPCTLS          worker.TLSConfig

	vmPoolMgr  *VMPoolManager
	poolConfig config.PoolConfig

	previewDomain string

	ctx    context.Context
	cancel context.CancelFunc
}

type spawnQueueStats struct {
	queuedTotal   uint64
	dequeuedTotal uint64
	timeoutTotal  uint64
	waitCount     uint64
	waitTotal     time.Duration
	waitMax       time.Duration
}

const sandboxLeaseGrace = 5 * time.Minute

type ManagerConfig struct {
	DefaultTTL            time.Duration
	DefaultImage          string
	DefaultMemory         int
	DefaultVCPUs          int
	Pool                  config.PoolConfig
	PreviewDomain         string
	Limits                OperationalLimits
	WorkerID              string
	WorkerToken           string
	WorkerSigningKey      string
	WorkerRevokedTokenIDs []string
	WorkerRPCTLS          worker.TLSConfig
}

func NewManager(registry *providers.Registry, st store.Store, events *EventBus, logger zerolog.Logger, cfg ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		registry:              registry,
		store:                 st,
		events:                events,
		logger:                logger.With().Str("component", "manager").Logger(),
		metrics:               NewMetricsRecorder(),
		sandboxes:             make(map[string]*Sandbox),
		capacityCh:            make(chan struct{}),
		defaultTTL:            cfg.DefaultTTL,
		defaultImage:          cfg.DefaultImage,
		defaultMemory:         cfg.DefaultMemory,
		defaultVCPUs:          cfg.DefaultVCPUs,
		limits:                cfg.Limits,
		workerID:              strings.TrimSpace(cfg.WorkerID),
		workerToken:           strings.TrimSpace(cfg.WorkerToken),
		workerSigningKey:      strings.TrimSpace(cfg.WorkerSigningKey),
		workerRevokedTokenIDs: cfg.WorkerRevokedTokenIDs,
		workerRPCTLS:          cfg.WorkerRPCTLS,
		poolConfig:            cfg.Pool,
		previewDomain:         cfg.PreviewDomain,
		ctx:                   ctx,
		cancel:                cancel,
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
	if m.workerID == "" {
		m.workerID = "local"
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
		if handled, err := m.reconcileRemoteOwnedSandbox(ctx, rec); handled || err != nil {
			if err != nil {
				return err
			}
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
		m.applyPreviewDomain(ctx, sb)
		m.mu.Lock()
		m.sandboxes[rec.ID] = sb
		m.mu.Unlock()
	}

	if err := m.reconcileProviderRuntimes(ctx, known); err != nil {
		return err
	}

	return nil
}

func (m *Manager) reconcileRemoteOwnedSandbox(ctx context.Context, rec *store.SandboxRecord) (bool, error) {
	if rec == nil || strings.TrimSpace(rec.WorkerID) == "" || rec.WorkerID == m.workerID {
		return false, nil
	}
	workerRec, err := m.store.GetWorker(ctx, rec.WorkerID)
	reason := ""
	if err != nil {
		reason = "worker_missing"
	} else if time.Since(workerRec.LastHeartbeat) > workerHeartbeatStaleAfter {
		reason = "worker_stale"
	} else if strings.EqualFold(workerRec.Status, "draining") {
		m.publishRemoteOwnershipEvent(rec, "worker_draining", "worker is draining; keeping existing ownership")
		sb := recordToSandbox(rec)
		if refreshed, refreshErr := m.refreshRemoteSandboxStatus(ctx, sb); refreshErr == nil {
			m.mu.Lock()
			m.sandboxes[rec.ID] = refreshed
			m.mu.Unlock()
		}
		return true, nil
	} else if !strings.EqualFold(workerRec.Status, "online") {
		reason = "worker_" + strings.TrimSpace(workerRec.Status)
	}
	if reason == "" {
		sb := recordToSandbox(rec)
		refreshed, refreshErr := m.refreshRemoteSandboxStatus(ctx, sb)
		if refreshErr != nil {
			reason = "worker_rpc_unavailable"
		} else {
			m.mu.Lock()
			m.sandboxes[rec.ID] = refreshed
			m.mu.Unlock()
			return true, nil
		}
	}

	state := StateUnhealthy
	action := "marked_unhealthy"
	if !rec.ExpiresAt.IsZero() && time.Now().UTC().After(rec.ExpiresAt) {
		state = StateExpired
		action = "marked_expired"
		_ = m.store.ReleaseLease(ctx, rec.ID, rec.WorkerID)
	}
	if SandboxState(rec.State) != state {
		if err := m.store.UpdateSandboxState(ctx, rec.ID, string(state)); err != nil {
			return true, fmt.Errorf("marking remote sandbox %s %s: %w", rec.ID, state, err)
		}
	}
	sb := recordToSandbox(rec)
	sb.State = state
	m.applyPreviewDomain(ctx, sb)
	m.mu.Lock()
	m.sandboxes[rec.ID] = sb
	m.mu.Unlock()
	m.publishRemoteOwnershipEvent(rec, action, reason)
	return true, nil
}

func (m *Manager) publishRemoteOwnershipEvent(rec *store.SandboxRecord, action, reason string) {
	m.logger.Info().
		Str("sandbox", rec.ID).
		Str("worker", rec.WorkerID).
		Str("action", action).
		Str("reason", reason).
		Msg("reconcile: remote sandbox ownership policy applied")
	m.publishOperationalEvent(EventReconcileAction, rec.ID, map[string]interface{}{
		"action": action,
		"reason": reason,
		"worker": rec.WorkerID,
	})
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
			if _, err := m.acquireSandboxLease(ctx, runtime.ID, expiresAt); err != nil {
				m.logger.Warn().Err(err).Str("sandbox", runtime.ID).Msg("reconcile: skipping runtime adoption because lease is unavailable")
				continue
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
				WorkerID:  m.workerID,
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
	ownerID, err := normalizeOptionalOwnerID(req.OwnerID)
	if err != nil {
		metricsErr = err
		m.publishFailureForError("", OperationSpawn, metricsProvider, err)
		return nil, err
	}
	req.OwnerID = ownerID
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

	placement := m.currentSpawnPlacement(ctx, metricsProvider)
	if placement.SelectedID != "" && placement.SelectedID != m.workerID {
		sb, err := m.spawnRemote(ctx, placement.SelectedID, req, metricsProvider, image, memMB, vcpus, ttl, now)
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
		TenantID:      req.TenantID,
		WorkerID:      m.workerID,
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
	if _, err := m.acquireSandboxLease(ctx, sb.ID, sb.ExpiresAt); err != nil {
		_ = prov.Destroy(ctx, id)
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, id, OperationSpawn, metricsProvider, err)
		return nil, fmt.Errorf("acquiring sandbox lease: %w", err)
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
		TenantID:  sb.TenantID,
		VMID:      sb.VMID,
		WorkerID:  sb.WorkerID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		// Best effort: destroy the sandbox if DB write fails
		prov.Destroy(ctx, id)
		_ = m.releaseSandboxLease(ctx, id)
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
	m.auditOperation(ctx, "sandbox.spawn", sb, sb.ID, sb.Image, "success", "state=running")

	m.logger.Info().Str("sandbox", id).Str("provider", prov.Name()).Str("image", image).Msg("sandbox spawned")
	return sb, nil
}

func (m *Manager) spawnRemote(ctx context.Context, workerID string, req SpawnRequest, providerName, image string, memMB, vcpus int, ttl time.Duration, now time.Time) (*Sandbox, error) {
	client, err := m.remoteWorkerRPCClient(ctx, workerID)
	if err != nil {
		return nil, err
	}
	sandboxID := generateSandboxID()
	expiresAt := now.Add(ttl)
	lease, err := m.acquireSandboxLeaseFor(ctx, sandboxID, workerID, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("acquiring remote sandbox lease: %w", err)
	}
	result, err := client.Spawn(ctx, "spawn-"+sandboxID, leaseTokenFromStore(lease), workerproto.SpawnParams{
		SandboxID: sandboxID,
		Image:     image,
		Provider:  providerName,
		MemoryMB:  memMB,
		VCPUs:     vcpus,
		OwnerID:   req.OwnerID,
		TTL:       ttl.String(),
		Metadata:  req.Metadata,
	})
	if err != nil {
		_ = m.store.ReleaseLease(ctx, sandboxID, workerID)
		return nil, fmt.Errorf("remote worker spawn: %w", err)
	}
	sb := &Sandbox{
		ID:            sandboxID,
		State:         StateRunning,
		Provider:      result.Provider,
		Image:         image,
		MemoryMB:      memMB,
		VCPUs:         vcpus,
		OwnerID:       req.OwnerID,
		VMID:          result.RuntimeID,
		WorkerID:      workerID,
		CreatedAt:     now,
		ExpiresAt:     expiresAt,
		Metadata:      req.Metadata,
		PreviewDomain: m.previewDomainForWorker(ctx, workerID),
	}
	if sb.Provider == "" {
		sb.Provider = providerName
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
		OwnerID:   sb.OwnerID,
		VMID:      sb.VMID,
		WorkerID:  sb.WorkerID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		_ = m.store.ReleaseLease(ctx, sandboxID, workerID)
		return nil, fmt.Errorf("persisting remote sandbox: %w", err)
	}
	m.mu.Lock()
	m.sandboxes[sandboxID] = sb
	m.mu.Unlock()
	m.events.Publish(Event{Type: EventSandboxCreated, SandboxID: sandboxID})
	m.events.Publish(Event{Type: EventSandboxRunning, SandboxID: sandboxID})
	m.auditOperation(ctx, "sandbox.spawn", sb, sb.ID, sb.Image, "success", "state=running remote_worker="+workerID)
	m.logger.Info().Str("sandbox", sandboxID).Str("runtime", sb.VMID).Str("worker", workerID).Str("provider", sb.Provider).Str("image", image).Msg("remote sandbox spawned")
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
		WorkerID:      m.workerID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
		Metadata:      req.Metadata,
		PreviewDomain: m.previewDomain,
	}
	if _, err := m.acquireSandboxLease(ctx, sb.ID, sb.ExpiresAt); err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationSpawn, prov.Name(), err)
		return nil, fmt.Errorf("acquiring sandbox lease: %w", err)
	}

	// Create the sandbox's isolated workspace on the VM.
	workspaceDir := "/workspace/" + sandboxID
	_, err = prov.Exec(ctx, vmID, providers.ExecOptions{
		Command: "mkdir -p " + workspaceDir,
	})
	if err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		_ = m.releaseSandboxLease(ctx, sandboxID)
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
		WorkerID:  sb.WorkerID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		m.vmPoolMgr.Release(vmID, sandboxID)
		_ = m.releaseSandboxLease(ctx, sandboxID)
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationSpawn, prov.Name(), err)
		return nil, fmt.Errorf("persisting sandbox: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[sandboxID] = sb
	m.mu.Unlock()

	m.events.Publish(Event{Type: EventSandboxCreated, SandboxID: sandboxID})
	m.events.Publish(Event{Type: EventSandboxRunning, SandboxID: sandboxID})
	m.auditOperation(ctx, "sandbox.spawn", sb, sb.ID, sb.Image, "success", "state=running pooled=true")

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

	if err := validateExecRequest(req); err != nil {
		metricsErr = err
		m.publishFailureForError(sandboxID, OperationExec, metricsProvider, err)
		return nil, err
	}

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
	if workDir == "" && sb.VMID != "" && !m.isRemoteOwnedSandbox(sb) {
		workDir = "/workspace/" + sandboxID
	}

	execStart := time.Now()
	var result *providers.ExecResult
	if m.isRemoteOwnedSandbox(sb) {
		result, err = m.execRemote(execCtx, sb, req, workDir)
	} else {
		result, err = prov.Exec(execCtx, m.resolveVMID(sb), providers.ExecOptions{
			Command: req.Command,
			Args:    req.Args,
			Mode:    req.Mode,
			Env:     req.Env,
			WorkDir: workDir,
		})
	}
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			metricsErr = providers.ExecTimeoutError(sandboxID)
			m.publishOperationFailure(EventExecTimeout, sandboxID, OperationExec, metricsProvider, metricsErr)
			m.auditOperation(ctx, "exec", sb, sandboxID, req.Command, "failure", metricsErr.Error())
			return nil, metricsErr
		}
		metricsErr = err
		m.publishOperationFailure(EventExecFailed, sandboxID, OperationExec, metricsProvider, err)
		m.auditOperation(ctx, "exec", sb, sandboxID, req.Command, "failure", err.Error())
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
	m.auditOperation(ctx, "exec", sb, sandboxID, req.Command, "success", fmt.Sprintf("exit=%d duration=%s mode=%s", result.ExitCode, duration.String(), req.Mode))

	return execResult, nil
}

func (m *Manager) execRemote(ctx context.Context, sb *Sandbox, req ExecRequest, workDir string) (*providers.ExecResult, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = sb.ID
	}
	result, err := client.Exec(ctx, "exec-"+sb.ID, workerproto.ExecParams{
		SandboxID: sb.ID,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
		Command:   req.Command,
		Args:      req.Args,
		Mode:      req.Mode,
		Env:       req.Env,
		WorkDir:   workDir,
		Timeout:   req.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("remote worker exec: %w", err)
	}
	return &providers.ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
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

	if err := validateExecRequest(req); err != nil {
		metricsErr = err
		m.publishFailureForError(sandboxID, OperationExecStream, metricsProvider, err)
		return nil, err
	}

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
	if workDir == "" && sb.VMID != "" && !m.isRemoteOwnedSandbox(sb) {
		workDir = "/workspace/" + sandboxID
	}

	var ch <-chan providers.StreamChunk
	if m.isRemoteOwnedSandbox(sb) {
		ch, err = m.execStreamRemote(execCtx, sb, req, workDir)
	} else {
		ch, err = prov.ExecStream(execCtx, m.resolveVMID(sb), providers.ExecOptions{
			Command: req.Command,
			Args:    req.Args,
			Mode:    req.Mode,
			Env:     req.Env,
			WorkDir: workDir,
		})
	}
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
	// metricsErr is read by the defer above; use a goroutine-local variable to
	// avoid a data race between the defer (which runs at ExecStream return) and
	// this goroutine (which may still be running).
	go func() {
		defer close(out)
		defer cancel()
		var goroutineErr error
		interrupted := false
		for chunk := range ch {
			select {
			case out <- chunk:
			case <-execCtx.Done():
				interrupted = true
			}
		}
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			goroutineErr = providers.ExecTimeoutError(sandboxID)
			m.publishOperationFailure(EventExecTimeout, sandboxID, OperationExecStream, metricsProvider, goroutineErr)
			select {
			case out <- providers.StreamChunk{Stream: "stderr", Data: goroutineErr.Error()}:
			case <-ctx.Done():
			}
		} else if interrupted && execCtx.Err() != nil {
			goroutineErr = execCtx.Err()
			m.publishOperationFailure(EventExecFailed, sandboxID, OperationExecStream, metricsProvider, goroutineErr)
		}
		m.recordOperation(OperationExecStream, metricsProvider, time.Since(start), goroutineErr)
	}()
	return out, nil
}

func (m *Manager) execStreamRemote(ctx context.Context, sb *Sandbox, req ExecRequest, workDir string) (<-chan providers.StreamChunk, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = sb.ID
	}
	chunks, errs, err := client.ExecStreamLive(ctx, "exec-stream-"+sb.ID, workerproto.ExecParams{
		SandboxID: sb.ID,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
		Command:   req.Command,
		Args:      req.Args,
		Mode:      req.Mode,
		Env:       req.Env,
		WorkDir:   workDir,
		Timeout:   req.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("remote worker exec stream: %w", err)
	}
	out := make(chan providers.StreamChunk, 64)
	go func() {
		defer close(out)
		for chunk := range chunks {
			select {
			case out <- providers.StreamChunk{Stream: chunk.Stream, Data: chunk.Data}:
			case <-ctx.Done():
				return
			}
		}
		for err := range errs {
			if err != nil {
				select {
				case out <- providers.StreamChunk{Stream: "stderr", Data: err.Error()}:
				case <-ctx.Done():
				}
			}
		}
	}()
	return out, nil
}

func validateExecRequest(req ExecRequest) error {
	switch strings.TrimSpace(req.Mode) {
	case "", providers.ExecModeShell:
		return nil
	case providers.ExecModeArgv:
		if strings.TrimSpace(req.Command) == "" {
			return InvalidInputError("argv exec mode requires command")
		}
		return nil
	default:
		return InvalidInputError(fmt.Sprintf("unsupported exec mode %q", req.Mode))
	}
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

	path, err := m.scopedPathForOperation(sb, req.Path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileWrite, metricsProvider, err)
		m.auditOperation(ctx, "file.write", sb, sandboxID, req.Path, "failure", err.Error())
		return err
	}
	if m.isRemoteOwnedSandbox(sb) {
		err = m.remoteFileWrite(ctx, sb, path, []byte(req.Content), mode)
	} else {
		err = prov.WriteFile(ctx, m.resolveVMID(sb), path, strings.NewReader(req.Content), mode)
	}
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileWrite, metricsProvider, err)
		m.auditOperation(ctx, "file.write", sb, sandboxID, req.Path, "failure", err.Error())
		return fmt.Errorf("writing file: %w", err)
	}

	m.events.Publish(Event{
		Type:      EventFileWritten,
		SandboxID: sandboxID,
	})
	m.auditOperation(ctx, "file.write", sb, sandboxID, req.Path, "success", "mode="+mode)
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

	scopedPath, err := m.scopedPathForOperation(sb, path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
		m.auditOperation(ctx, "file.read", sb, sandboxID, path, "failure", err.Error())
		return nil, err
	}
	var buf []byte
	if m.isRemoteOwnedSandbox(sb) {
		buf, err = m.remoteFileRead(ctx, sb, scopedPath)
		if err != nil {
			metricsErr = err
			m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
			m.auditOperation(ctx, "file.read", sb, sandboxID, path, "failure", err.Error())
			return nil, fmt.Errorf("reading file: %w", err)
		}
	} else {
		rc, err := prov.ReadFile(ctx, m.resolveVMID(sb), scopedPath)
		if err != nil {
			metricsErr = err
			m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
			m.auditOperation(ctx, "file.read", sb, sandboxID, path, "failure", err.Error())
			return nil, fmt.Errorf("reading file: %w", err)
		}
		defer rc.Close()

		buf, err = io.ReadAll(rc)
		if err != nil {
			metricsErr = err
			m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileRead, metricsProvider, err)
			return nil, fmt.Errorf("reading file content: %w", err)
		}
	}

	m.events.Publish(Event{
		Type:      EventFileRead,
		SandboxID: sandboxID,
	})
	m.auditOperation(ctx, "file.read", sb, sandboxID, path, "success", fmt.Sprintf("bytes=%d", len(buf)))
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

	scopedPath, err := m.scopedPathForOperation(sb, path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileList, metricsProvider, err)
		m.auditOperation(ctx, "file.list", sb, sandboxID, path, "failure", err.Error())
		return nil, err
	}
	var pFiles []providers.FileInfo
	if m.isRemoteOwnedSandbox(sb) {
		pFiles, err = m.remoteFileList(ctx, sb, scopedPath)
	} else {
		pFiles, err = prov.ListFiles(ctx, m.resolveVMID(sb), scopedPath)
	}
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileList, metricsProvider, err)
		m.auditOperation(ctx, "file.list", sb, sandboxID, path, "failure", err.Error())
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
	m.auditOperation(ctx, "file.list", sb, sandboxID, path, "success", fmt.Sprintf("count=%d", len(files)))
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

	path, err := m.scopedPathForOperation(sb, req.Path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileDelete, metricsProvider, err)
		m.auditOperation(ctx, "file.delete", sb, sandboxID, req.Path, "failure", err.Error())
		return err
	}
	if m.isRemoteOwnedSandbox(sb) {
		metricsErr = m.remoteFileDelete(ctx, sb, path, req.Recursive)
	} else {
		metricsErr = prov.DeleteFile(ctx, m.resolveVMID(sb), path, req.Recursive)
	}
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileDelete, metricsProvider, metricsErr)
		m.auditOperation(ctx, "file.delete", sb, sandboxID, req.Path, "failure", metricsErr.Error())
		return metricsErr
	}
	m.auditOperation(ctx, "file.delete", sb, sandboxID, req.Path, "success", fmt.Sprintf("recursive=%t", req.Recursive))
	return nil
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

	oldPath, err := m.scopedPathForOperation(sb, req.OldPath)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileMove, metricsProvider, err)
		m.auditOperation(ctx, "file.move", sb, sandboxID, req.OldPath, "failure", err.Error())
		return err
	}
	newPath, err := m.scopedPathForOperation(sb, req.NewPath)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileMove, metricsProvider, err)
		m.auditOperation(ctx, "file.move", sb, sandboxID, req.NewPath, "failure", err.Error())
		return err
	}
	if m.isRemoteOwnedSandbox(sb) {
		metricsErr = m.remoteFileMove(ctx, sb, oldPath, newPath)
	} else {
		metricsErr = prov.MoveFile(ctx, m.resolveVMID(sb), oldPath, newPath)
	}
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileMove, metricsProvider, metricsErr)
		m.auditOperation(ctx, "file.move", sb, sandboxID, req.OldPath+" -> "+req.NewPath, "failure", metricsErr.Error())
		return metricsErr
	}
	m.auditOperation(ctx, "file.move", sb, sandboxID, req.OldPath+" -> "+req.NewPath, "success", "")
	return nil
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

	path, err := m.scopedPathForOperation(sb, req.Path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileChmod, metricsProvider, err)
		m.auditOperation(ctx, "file.chmod", sb, sandboxID, req.Path, "failure", err.Error())
		return err
	}
	if m.isRemoteOwnedSandbox(sb) {
		metricsErr = m.remoteFileChmod(ctx, sb, path, req.Mode)
	} else {
		metricsErr = prov.ChmodFile(ctx, m.resolveVMID(sb), path, req.Mode)
	}
	if metricsErr != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileChmod, metricsProvider, metricsErr)
		m.auditOperation(ctx, "file.chmod", sb, sandboxID, req.Path, "failure", metricsErr.Error())
		return metricsErr
	}
	m.auditOperation(ctx, "file.chmod", sb, sandboxID, req.Path, "success", "mode="+req.Mode)
	return nil
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

	scopedPath, err := m.scopedPathForOperation(sb, path)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileStat, metricsProvider, err)
		m.auditOperation(ctx, "file.stat", sb, sandboxID, path, "failure", err.Error())
		return nil, err
	}
	var fi *providers.FileInfo
	if m.isRemoteOwnedSandbox(sb) {
		fi, err = m.remoteFileStat(ctx, sb, scopedPath)
	} else {
		fi, err = prov.StatFile(ctx, m.resolveVMID(sb), scopedPath)
	}
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileStat, metricsProvider, err)
		m.auditOperation(ctx, "file.stat", sb, sandboxID, path, "failure", err.Error())
		return nil, err
	}
	m.auditOperation(ctx, "file.stat", sb, sandboxID, path, "success", "")
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

	scopedPattern, err := m.scopedPathForOperation(sb, pattern)
	if err != nil {
		metricsErr = err
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileGlob, metricsProvider, err)
		m.auditOperation(ctx, "file.glob", sb, sandboxID, pattern, "failure", err.Error())
		return nil, err
	}
	var matches []string
	if m.isRemoteOwnedSandbox(sb) {
		matches, err = m.remoteFileGlob(ctx, sb, scopedPattern)
	} else {
		matches, err = prov.GlobFiles(ctx, m.resolveVMID(sb), scopedPattern)
	}
	metricsErr = err
	if err != nil {
		m.publishOperationFailure(EventOperationFailed, sandboxID, OperationFileGlob, metricsProvider, err)
		m.auditOperation(ctx, "file.glob", sb, sandboxID, pattern, "failure", err.Error())
		return matches, err
	}
	m.auditOperation(ctx, "file.glob", sb, sandboxID, pattern, "success", fmt.Sprintf("count=%d", len(matches)))
	return matches, err
}

func (m *Manager) remoteFileParams(sb *Sandbox) workerproto.FileParams {
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = sb.ID
	}
	return workerproto.FileParams{
		SandboxID: sb.ID,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
	}
}

func (m *Manager) remoteFileWrite(ctx context.Context, sb *Sandbox, path string, content []byte, mode string) error {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	params.Content = content
	params.Mode = mode
	return client.FileWrite(ctx, "file-write-"+sb.ID, params)
}

func (m *Manager) remoteFileRead(ctx context.Context, sb *Sandbox, path string) ([]byte, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	result, err := client.FileRead(ctx, "file-read-"+sb.ID, params)
	if err != nil {
		return nil, err
	}
	return result.Content, nil
}

func (m *Manager) remoteFileList(ctx context.Context, sb *Sandbox, path string) ([]providers.FileInfo, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	result, err := client.FileList(ctx, "file-list-"+sb.ID, params)
	if err != nil {
		return nil, err
	}
	return fromWorkerFileInfo(result.Files), nil
}

func (m *Manager) remoteFileDelete(ctx context.Context, sb *Sandbox, path string, recursive bool) error {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	params.Recursive = recursive
	return client.FileDelete(ctx, "file-delete-"+sb.ID, params)
}

func (m *Manager) remoteFileMove(ctx context.Context, sb *Sandbox, oldPath, newPath string) error {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return err
	}
	params := m.remoteFileParams(sb)
	params.OldPath = oldPath
	params.NewPath = newPath
	return client.FileMove(ctx, "file-move-"+sb.ID, params)
}

func (m *Manager) remoteFileChmod(ctx context.Context, sb *Sandbox, path, mode string) error {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	params.Mode = mode
	return client.FileChmod(ctx, "file-chmod-"+sb.ID, params)
}

func (m *Manager) remoteFileStat(ctx context.Context, sb *Sandbox, path string) (*providers.FileInfo, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	params := m.remoteFileParams(sb)
	params.Path = path
	result, err := client.FileStat(ctx, "file-stat-"+sb.ID, params)
	if err != nil {
		return nil, err
	}
	file := providers.FileInfo{
		Path:    result.File.Path,
		Size:    result.File.Size,
		Mode:    result.File.Mode,
		IsDir:   result.File.IsDir,
		ModTime: result.File.ModTime,
	}
	return &file, nil
}

func (m *Manager) remoteFileGlob(ctx context.Context, sb *Sandbox, pattern string) ([]string, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	params := m.remoteFileParams(sb)
	params.Pattern = pattern
	result, err := client.FileGlob(ctx, "file-glob-"+sb.ID, params)
	if err != nil {
		return nil, err
	}
	return result.Matches, nil
}

func fromWorkerFileInfo(files []workerproto.FileInfo) []providers.FileInfo {
	out := make([]providers.FileInfo, len(files))
	for i, file := range files {
		out[i] = providers.FileInfo{
			Path:    file.Path,
			Size:    file.Size,
			Mode:    file.Mode,
			IsDir:   file.IsDir,
			ModTime: file.ModTime,
		}
	}
	return out
}

// scopedPath prefixes a path with the sandbox workspace when running in pool mode.
func (m *Manager) scopedPath(sb *Sandbox, path string) string {
	if sb.VMID == "" || m.isRemoteOwnedSandbox(sb) {
		return path // dedicated VM, no scoping
	}
	base := "/workspace/" + sb.ID
	cleaned := filepath.Clean(filepath.Join(base, path))
	if !strings.HasPrefix(cleaned, base+"/") && cleaned != base {
		return base
	}
	return cleaned
}

func (m *Manager) scopedPathForOperation(sb *Sandbox, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", InvalidInputError("path is required")
	}
	if sb.VMID == "" || m.isRemoteOwnedSandbox(sb) {
		return path, nil
	}
	base := "/workspace/" + sb.ID
	cleaned := filepath.Clean(filepath.Join(base, path))
	if !strings.HasPrefix(cleaned, base+"/") && cleaned != base {
		return "", InvalidInputError(fmt.Sprintf("path %q escapes sandbox workspace", path))
	}
	return cleaned, nil
}

func (m *Manager) auditOperation(ctx context.Context, action string, sb *Sandbox, sandboxID, resource, status, detail string) {
	rec := &store.OperationAuditRecord{
		Action:    action,
		SandboxID: sandboxID,
		Resource:  resource,
		Status:    status,
		Detail:    truncateAuditDetail(detail),
		CreatedAt: time.Now().UTC(),
	}
	if sb != nil {
		rec.Actor = sb.OwnerID
		rec.Provider = sb.Provider
		if rec.SandboxID == "" {
			rec.SandboxID = sb.ID
		}
	}
	if err := m.store.CreateOperationAudit(ctx, rec); err != nil {
		m.logger.Debug().Err(err).Str("action", action).Str("sandbox", sandboxID).Msg("operation audit write failed")
	}
}

func truncateAuditDetail(detail string) string {
	const maxAuditDetailLen = 2048
	if len(detail) <= maxAuditDetailLen {
		return detail
	}
	return detail[:maxAuditDetailLen]
}

// resolveVMID returns the VM sandbox ID for provider calls.
// In pool mode, operations go to the VM (sb.VMID); in 1:1 mode, to the sandbox itself.
func (m *Manager) resolveVMID(sb *Sandbox) string {
	if sb.VMID != "" {
		return sb.VMID
	}
	return sb.ID
}

func (m *Manager) isRemoteOwnedSandbox(sb *Sandbox) bool {
	return sb != nil && strings.TrimSpace(sb.WorkerID) != "" && sb.WorkerID != m.workerID
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
	waitAvg := time.Duration(0)
	if m.queueStats.waitCount > 0 {
		waitAvg = time.Duration(int64(m.queueStats.waitTotal) / int64(m.queueStats.waitCount))
	}
	queueWaiters := m.queueWaiters
	queuedTotal := m.queueStats.queuedTotal
	dequeuedTotal := m.queueStats.dequeuedTotal
	timeoutTotal := m.queueStats.timeoutTotal
	waitCount := m.queueStats.waitCount
	waitTotal := m.queueStats.waitTotal
	waitMax := m.queueStats.waitMax
	m.queueMu.Unlock()

	placement := workerPlacement{SelectedID: m.workerID, Eligible: 1}
	if records, err := m.store.ListSandboxes(context.Background()); err == nil {
		placement = m.evaluateWorkerPlacement(context.Background(), m.registry.Default(), records)
	}
	return SchedulerStatus{
		SpawnOverflow:         m.limits.SpawnOverflow,
		SpawnQueueDepth:       queueWaiters,
		MaxSpawnQueue:         m.limits.MaxSpawnQueue,
		SpawnQueueTimeout:     m.limits.SpawnQueueTimeout.String(),
		AdmissionControl:      "worker_aware_local",
		WorkerID:              m.workerID,
		SelectedWorkerID:      placement.SelectedID,
		EligibleWorkers:       placement.Eligible,
		SpawnQueuedTotal:      queuedTotal,
		SpawnDequeuedTotal:    dequeuedTotal,
		SpawnQueueTimeouts:    timeoutTotal,
		SpawnQueueWaitCount:   waitCount,
		SpawnQueueWaitTotal:   waitTotal.String(),
		SpawnQueueWaitMax:     waitMax.String(),
		SpawnQueueWaitAvg:     waitAvg.String(),
		SpawnQueueWaitTotalMS: waitTotal.Milliseconds(),
		SpawnQueueWaitMaxMS:   waitMax.Milliseconds(),
		SpawnQueueWaitAvgMS:   waitAvg.Milliseconds(),
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
	m.notifySpawnCapacity()
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
	m.notifySpawnCapacity()
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
		decision, err := m.evaluateSpawnAdmission(ctx, ownerID, ttl, provider)
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
	decision, err := m.evaluateSpawnAdmission(ctx, ownerID, ttl, provider)
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
	m.queueStats.queuedTotal++
	depth := m.queueWaiters
	capacityCh := m.capacityCh
	m.queueMu.Unlock()
	queuedAt := time.Now()

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
				waitDuration := time.Since(queuedAt)
				m.recordSpawnQueueTimeout(waitDuration)
				m.publishOperationalEvent(EventSpawnQueueTimeout, "", map[string]interface{}{
					"operation": OperationSpawn,
					"provider":  provider,
					"owner_id":  ownerID,
					"error":     err.Error(),
					"wait_ms":   waitDuration.Milliseconds(),
				})
				return err
			}
			return waitCtx.Err()
		case <-capacityCh:
			decision, err = m.evaluateSpawnAdmission(ctx, ownerID, ttl, provider)
			if err != nil {
				return err
			}
			if decision.Allowed {
				waitDuration := time.Since(queuedAt)
				m.recordSpawnDequeued(waitDuration)
				m.publishOperationalEvent(EventSpawnDequeued, "", map[string]interface{}{
					"operation": OperationSpawn,
					"provider":  provider,
					"owner_id":  ownerID,
					"wait_ms":   waitDuration.Milliseconds(),
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

func (m *Manager) recordSpawnDequeued(waitDuration time.Duration) {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	m.queueStats.dequeuedTotal++
	m.queueStats.waitCount++
	m.queueStats.waitTotal += waitDuration
	if waitDuration > m.queueStats.waitMax {
		m.queueStats.waitMax = waitDuration
	}
}

func (m *Manager) recordSpawnQueueTimeout(waitDuration time.Duration) {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()
	m.queueStats.timeoutTotal++
	m.queueStats.waitCount++
	m.queueStats.waitTotal += waitDuration
	if waitDuration > m.queueStats.waitMax {
		m.queueStats.waitMax = waitDuration
	}
}

func (m *Manager) notifySpawnCapacity() {
	m.queueMu.Lock()
	close(m.capacityCh)
	m.capacityCh = make(chan struct{})
	m.queueMu.Unlock()
}

func (m *Manager) acquireSandboxLease(ctx context.Context, sandboxID string, expiresAt time.Time) (*store.LeaseRecord, error) {
	return m.acquireSandboxLeaseFor(ctx, sandboxID, m.workerID, expiresAt)
}

func (m *Manager) acquireSandboxLeaseFor(ctx context.Context, sandboxID, workerID string, expiresAt time.Time) (*store.LeaseRecord, error) {
	ttl := time.Until(expiresAt) + sandboxLeaseGrace
	if ttl <= 0 {
		ttl = sandboxLeaseGrace
	}
	return m.store.AcquireLease(ctx, sandboxID, "sandbox", workerID, ttl)
}

func (m *Manager) releaseSandboxLease(ctx context.Context, sandboxID string) error {
	err := m.store.ReleaseLease(ctx, sandboxID, m.workerID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}

func (m *Manager) EvaluateSpawnAdmission(ctx context.Context, ownerID string, ttl time.Duration) (SpawnAdmissionDecision, error) {
	return m.evaluateSpawnAdmission(ctx, ownerID, ttl, "")
}

func (m *Manager) evaluateSpawnAdmission(ctx context.Context, ownerID string, ttl time.Duration, provider string) (SpawnAdmissionDecision, error) {
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

	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return SpawnAdmissionDecision{}, fmt.Errorf("checking sandbox limits: %w", err)
	}
	placement := m.evaluateWorkerPlacement(ctx, provider, records)
	decision.SelectedWorkerID = placement.SelectedID
	decision.EligibleWorkers = placement.Eligible
	decision.WorkerReason = placement.Reason
	if placement.SelectedID == "" {
		decision.Allowed = false
		decision.Reason = "worker_unavailable"
		return decision, nil
	}
	if placement.SelectedID != m.workerID {
		decision.WorkerReason = "remote_worker_selected"
		if !m.canUseRemoteWorker(ctx, placement.SelectedID) {
			decision.Allowed = false
			decision.Reason = "remote_worker_rpc_unavailable"
			return decision, nil
		}
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

func (m *Manager) currentSpawnPlacement(ctx context.Context, provider string) workerPlacement {
	records, err := m.store.ListSandboxes(ctx)
	if err != nil {
		return workerPlacement{SelectedID: m.workerID, Eligible: 1, Reason: "local_fallback"}
	}
	return m.evaluateWorkerPlacement(ctx, provider, records)
}

func (m *Manager) canUseRemoteWorker(ctx context.Context, workerID string) bool {
	_, err := m.remoteWorkerRPCClient(ctx, workerID)
	return err == nil
}

func (m *Manager) remoteWorkerRPCClient(ctx context.Context, workerID string) (worker.RPCClient, error) {
	var zero worker.RPCClient
	if strings.TrimSpace(m.workerToken) == "" && strings.TrimSpace(m.workerSigningKey) == "" {
		return zero, fmt.Errorf("worker token is required for remote worker RPC")
	}
	rec, err := m.store.GetWorker(ctx, workerID)
	if err != nil {
		return zero, err
	}
	rpcURL := workerRPCURL(rec)
	if rpcURL == "" {
		return zero, fmt.Errorf("worker %s has no rpc_url", workerID)
	}
	client := worker.RPCClient{
		BaseURL:  rpcURL,
		WorkerID: workerID,
		Token:    m.workerToken,
		RPCTLS:   m.workerRPCTLS,
	}
	if strings.TrimSpace(client.Token) == "" && strings.TrimSpace(m.workerSigningKey) != "" {
		client.TokenFunc = func() (string, error) {
			now := time.Now().UTC()
			tokenID, err := middleware.NewWorkerTokenID()
			if err != nil {
				return "", err
			}
			return middleware.SignWorkerToken(m.workerSigningKey, middleware.WorkerTokenClaims{
				WorkerID:  workerID,
				TokenID:   tokenID,
				Audience:  middleware.WorkerTokenAudienceRPC,
				IssuedAt:  now.Unix(),
				ExpiresAt: now.Add(5 * time.Minute).Unix(),
			})
		}
	}
	return client, nil
}

func leaseTokenFromStore(rec *store.LeaseRecord) workerproto.LeaseToken {
	if rec == nil {
		return workerproto.LeaseToken{}
	}
	return workerproto.LeaseToken{
		ResourceID: rec.ResourceID,
		HolderID:   rec.HolderID,
		Generation: rec.Generation,
		ExpiresAt:  rec.ExpiresAt,
	}
}

func generateSandboxID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sb-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("sb-%08x", b)
}

// EvaluateSpawnRequestAdmission evaluates a spawn request against the current
// quota and scheduler limits without creating provider resources.
func (m *Manager) EvaluateSpawnRequestAdmission(ctx context.Context, req SpawnRequest) (SpawnAdmissionDecision, error) {
	ttl := m.defaultTTL
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			return SpawnAdmissionDecision{}, fmt.Errorf("%w: parsing TTL: %v", ErrInvalidInput, err)
		}
		ttl = parsed
	}
	ownerID, err := normalizeOptionalOwnerID(req.OwnerID)
	if err != nil {
		return SpawnAdmissionDecision{}, err
	}
	providerName := req.Provider
	if providerName == "" {
		providerName = m.registry.Default()
	}
	decision, err := m.evaluateSpawnAdmission(ctx, ownerID, ttl, providerName)
	if err != nil {
		return SpawnAdmissionDecision{}, err
	}
	if decision.Queueable && !strings.EqualFold(m.limits.SpawnOverflow, "queue") {
		decision.Queueable = false
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
	case "worker_unavailable":
		return providers.ResourceLimitError("no eligible worker available")
	case "remote_worker_rpc_unavailable":
		return providers.ResourceLimitError("selected worker requires remote worker RPC")
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

func normalizeOptionalOwnerID(ownerID string) (string, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return "", nil
	}
	return normalizeOwnerID(ownerID)
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
		WorkerID:      m.workerID,
		PreviewDomain: m.previewDomain,
	}
	if _, err := m.acquireSandboxLease(ctx, sb.ID, sb.ExpiresAt); err != nil {
		_ = prov.Destroy(ctx, id)
		return nil, fmt.Errorf("acquiring pool VM lease: %w", err)
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
		WorkerID:  sb.WorkerID,
		CreatedAt: sb.CreatedAt,
		ExpiresAt: sb.ExpiresAt,
		UpdatedAt: now,
	}); err != nil {
		prov.Destroy(ctx, id)
		_ = m.releaseSandboxLease(ctx, id)
		return nil, fmt.Errorf("persisting pool VM: %w", err)
	}

	m.mu.Lock()
	m.sandboxes[id] = sb
	m.mu.Unlock()

	m.logger.Info().Str("vm_id", id).Str("image", image).Msg("pool VM spawned")
	return sb, nil
}

func (m *Manager) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	sb, prov, err := m.getSandboxAndProvider(sandboxID)
	if err != nil {
		return nil, err
	}
	if m.isRemoteOwnedSandbox(sb) {
		return m.remoteConsoleLog(ctx, sb, lines)
	}
	return prov.ConsoleLog(ctx, m.resolveVMID(sb), lines)
}

func (m *Manager) remoteConsoleLog(ctx context.Context, sb *Sandbox, lines int) ([]string, error) {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return nil, err
	}
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = sb.ID
	}
	result, err := client.Logs(ctx, "logs-"+sb.ID, workerproto.LogsParams{
		SandboxID: sb.ID,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
		Lines:     lines,
	})
	if err != nil {
		return nil, fmt.Errorf("remote worker logs: %w", err)
	}
	return result.Lines, nil
}

func (m *Manager) applyPreviewDomain(ctx context.Context, sb *Sandbox) {
	if sb == nil {
		return
	}
	sb.PreviewDomain = m.previewDomainForWorker(ctx, sb.WorkerID)
}

func (m *Manager) previewDomainForWorker(ctx context.Context, workerID string) string {
	if strings.TrimSpace(workerID) == "" || workerID == m.workerID {
		return m.previewDomain
	}
	rec, err := m.store.GetWorker(ctx, workerID)
	if err != nil {
		return m.previewDomain
	}
	if domain := workerPreviewDomain(rec); domain != "" {
		return domain
	}
	return m.previewDomain
}

func (m *Manager) Get(ctx context.Context, id string) (*Sandbox, error) {
	m.mu.RLock()
	sb, ok := m.sandboxes[id]
	m.mu.RUnlock()

	if ok {
		m.applyPreviewDomain(ctx, sb)
		if refreshed, err := m.refreshRemoteSandboxStatus(ctx, sb); err == nil {
			return refreshed, nil
		}
		return sb, nil
	}

	// Fall back to store
	rec, err := m.store.GetSandbox(ctx, id)
	if err != nil {
		return nil, providers.SandboxNotFoundError(id)
	}
	sb = recordToSandbox(rec)
	m.applyPreviewDomain(ctx, sb)
	if sb.State == StateDestroyed {
		return nil, providers.SandboxDestroyedError(id)
	}
	if refreshed, err := m.refreshRemoteSandboxStatus(ctx, sb); err == nil {
		return refreshed, nil
	}
	return sb, nil
}

func (m *Manager) refreshRemoteSandboxStatus(ctx context.Context, sb *Sandbox) (*Sandbox, error) {
	m.applyPreviewDomain(ctx, sb)
	if sb == nil || strings.TrimSpace(sb.WorkerID) == "" || sb.WorkerID == m.workerID {
		return sb, nil
	}
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		m.logger.Debug().Err(err).Str("sandbox", sb.ID).Str("worker", sb.WorkerID).Msg("remote status unavailable")
		return nil, err
	}
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = sb.ID
	}
	status, err := client.Status(ctx, "status-"+sb.ID, workerproto.StatusParams{
		SandboxID: sb.ID,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
	})
	if err != nil {
		m.logger.Debug().Err(err).Str("sandbox", sb.ID).Str("worker", sb.WorkerID).Msg("remote status failed")
		return nil, err
	}
	state := SandboxState(status.State)
	if state == "" {
		return sb, nil
	}
	if state != sb.State {
		sb.State = state
		_ = m.store.UpdateSandboxState(ctx, sb.ID, string(state))
		m.mu.Lock()
		if current, ok := m.sandboxes[sb.ID]; ok {
			current.State = state
		} else {
			m.sandboxes[sb.ID] = sb
		}
		m.mu.Unlock()
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
		m.applyPreviewDomain(ctx, sandboxes[i])
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

	if sb == nil {
		if rec, err := m.store.GetSandbox(ctx, id); err == nil {
			sb = recordToSandbox(rec)
			metricsProvider = sb.Provider
		}
	}
	if sb != nil {
		if strings.TrimSpace(sb.WorkerID) != "" && sb.WorkerID != m.workerID {
			metricsErr = m.destroyRemote(ctx, id, sb)
			if metricsErr != nil {
				m.publishOperationFailure(EventOperationFailed, id, OperationDestroy, metricsProvider, metricsErr)
				m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "failure", metricsErr.Error())
				return metricsErr
			}
			m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "success", "remote_worker="+sb.WorkerID)
			return nil
		}
		if _, err := m.acquireSandboxLease(ctx, id, sb.ExpiresAt); err != nil {
			metricsErr = err
			m.publishOperationFailure(EventOperationFailed, id, OperationDestroy, metricsProvider, err)
			m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "failure", "lease_unavailable: "+err.Error())
			return err
		}
	}

	if sb != nil && sb.VMID != "" && m.vmPoolMgr != nil {
		metricsErr = m.destroyPooled(ctx, id, sb)
		if metricsErr != nil {
			m.publishOperationFailure(EventOperationFailed, id, OperationDestroy, metricsProvider, metricsErr)
			m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "failure", metricsErr.Error())
			return metricsErr
		}
		m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "success", "pooled=true")
		return nil
	}

	prov, err := m.getProvider(id)
	if err != nil {
		// If we can't find the provider, just update the store
		m.store.DeleteSandbox(ctx, id)
		_ = m.releaseSandboxLease(ctx, id)
		m.mu.Lock()
		delete(m.sandboxes, id)
		m.mu.Unlock()
		m.notifySpawnCapacity()
		m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "success", "provider_unavailable=true")
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
	_ = m.releaseSandboxLease(ctx, id)
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

	m.auditOperation(ctx, "sandbox.destroy", sb, id, "", "success", "")
	m.logger.Info().Str("sandbox", id).Msg("sandbox destroyed")
	return nil
}

func (m *Manager) destroyRemote(ctx context.Context, id string, sb *Sandbox) error {
	client, err := m.remoteWorkerRPCClient(ctx, sb.WorkerID)
	if err != nil {
		return err
	}
	lease, err := m.store.GetLease(ctx, id)
	if err != nil {
		return fmt.Errorf("getting remote sandbox lease: %w", err)
	}
	if lease.HolderID != sb.WorkerID {
		return fmt.Errorf("remote sandbox lease holder %q does not match worker %q", lease.HolderID, sb.WorkerID)
	}
	runtimeID := strings.TrimSpace(sb.VMID)
	if runtimeID == "" {
		runtimeID = id
	}
	if err := client.Destroy(ctx, "destroy-"+id, leaseTokenFromStore(lease), workerproto.DestroyParams{
		SandboxID: id,
		Provider:  sb.Provider,
		RuntimeID: runtimeID,
	}); err != nil {
		return fmt.Errorf("remote worker destroy: %w", err)
	}
	if err := m.store.UpdateSandboxState(ctx, id, string(StateDestroyed)); err != nil {
		return err
	}
	_ = m.store.ReleaseLease(ctx, id, sb.WorkerID)
	m.mu.Lock()
	if current, ok := m.sandboxes[id]; ok {
		current.State = StateDestroyed
	}
	delete(m.sandboxes, id)
	m.mu.Unlock()
	m.events.Publish(Event{
		Type:      EventSandboxDestroyed,
		SandboxID: id,
	})
	m.notifySpawnCapacity()
	m.logger.Info().Str("sandbox", id).Str("worker", sb.WorkerID).Str("runtime", runtimeID).Msg("remote sandbox destroyed")
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
		_ = m.releaseSandboxLease(ctx, vmID)
		m.mu.Lock()
		delete(m.sandboxes, vmID)
		m.mu.Unlock()
		m.logger.Info().Str("vm_id", vmID).Msg("empty pool VM destroyed")
	}

	// Clean up the logical sandbox record.
	m.store.UpdateSandboxState(ctx, id, string(StateDestroyed))
	_ = m.releaseSandboxLease(ctx, id)
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
		TenantID:  r.TenantID,
		VMID:      r.VMID,
		WorkerID:  r.WorkerID,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
		Metadata:  metadata,
	}
}
