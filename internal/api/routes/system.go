package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/StacyOs/stacyvm/internal/httputil"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SystemRoutes struct {
	registry  *providers.Registry
	manager   *orchestrator.Manager
	events    *orchestrator.EventBus
	startTime time.Time
	version   string
}

func NewSystemRoutes(registry *providers.Registry, manager *orchestrator.Manager, events *orchestrator.EventBus, version string) *SystemRoutes {
	return &SystemRoutes{
		registry:  registry,
		manager:   manager,
		events:    events,
		startTime: time.Now(),
		version:   version,
	}
}

func (s *SystemRoutes) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/health", s.Health)
	r.Get("/live", s.Live)
	r.Get("/ready", s.Ready)
	r.Get("/metrics", s.Metrics)
	r.Get("/metrics/prometheus", s.PrometheusMetrics)
	r.Get("/events", s.Events)
	return r
}

// Health returns the health status of the server.
//
//	@Summary		Health check
//	@Description	Return the health status, version, and uptime
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Security		ApiKeyAuth
//	@Router			/health [get]
func (s *SystemRoutes) Health(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": s.version,
		"uptime":  time.Since(s.startTime).String(),
	})
}

// Live returns process liveness.
//
//	@Summary		Liveness check
//	@Description	Return whether the StacyVM API process is alive
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Security		ApiKeyAuth
//	@Router			/live [get]
func (s *SystemRoutes) Live(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "alive",
		"version": s.version,
		"uptime":  time.Since(s.startTime).String(),
	})
}

// Ready returns dependency readiness.
//
//	@Summary		Readiness check
//	@Description	Return whether the API is ready to serve sandbox traffic
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	ReadinessResponse
//	@Failure		503	{object}	ReadinessResponse
//	@Security		ApiKeyAuth
//	@Router			/ready [get]
func (s *SystemRoutes) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	providers := s.providerHealth(ctx)
	readyProviders := 0
	for _, provider := range providers {
		if provider.Healthy {
			readyProviders++
		}
	}

	statusCode := http.StatusOK
	status := "ready"
	if len(providers) == 0 || readyProviders == 0 {
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
	}

	httputil.WriteJSON(w, statusCode, map[string]interface{}{
		"status":          status,
		"version":         s.version,
		"uptime":          time.Since(s.startTime).String(),
		"providers":       providers,
		"ready_providers": readyProviders,
		"total_providers": len(providers),
	})
}

// Metrics returns runtime metrics.
//
//	@Summary		Get metrics
//	@Description	Return runtime metrics including sandbox count, goroutines, and memory usage
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	MetricsResponse
//	@Security		ApiKeyAuth
//	@Router			/metrics [get]
func (s *SystemRoutes) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.collectMetrics(r.Context())
	if err != nil {
		writeRouteError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, metrics.toResponse())
}

// PrometheusMetrics returns Prometheus-compatible operational metrics.
//
//	@Summary		Get Prometheus metrics
//	@Description	Return runtime, provider, sandbox, event, and operation metrics in Prometheus text format
//	@Tags			system
//	@Produce		text/plain
//	@Success		200	{string}	string
//	@Security		ApiKeyAuth
//	@Router			/metrics/prometheus [get]
func (s *SystemRoutes) PrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.collectMetrics(r.Context())
	if err != nil {
		writeRouteError(w, err)
		return
	}

	var buf bytes.Buffer
	writePrometheusMetrics(&buf, metrics)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

type systemMetricsSnapshot struct {
	uptime              time.Duration
	goroutines          int
	memoryAlloc         uint64
	memorySys           uint64
	memoryHeapAlloc     uint64
	gcCycles            uint32
	sandboxTotal        int
	sandboxActive       int
	sandboxesByState    map[string]int
	sandboxesByProvider map[string]int
	providerHealth      []ProviderHealth
	healthyProviders    int
	eventStats          orchestrator.EventBusStats
	operationMetrics    []orchestrator.OperationMetrics
}

func (s *SystemRoutes) collectMetrics(ctx context.Context) (systemMetricsSnapshot, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	sandboxes, err := s.manager.List(ctx)
	if err != nil {
		return systemMetricsSnapshot{}, err
	}

	byState := make(map[string]int)
	byProvider := make(map[string]int)
	for _, sb := range sandboxes {
		byState[string(sb.State)]++
		byProvider[sb.Provider]++
	}

	providerHealth := s.providerHealth(ctx)
	healthyProviders := 0
	for _, provider := range providerHealth {
		if provider.Healthy {
			healthyProviders++
		}
	}
	eventStats := s.events.Stats()

	return systemMetricsSnapshot{
		uptime:              time.Since(s.startTime),
		goroutines:          runtime.NumGoroutine(),
		memoryAlloc:         mem.Alloc,
		memorySys:           mem.Sys,
		memoryHeapAlloc:     mem.HeapAlloc,
		gcCycles:            mem.NumGC,
		sandboxTotal:        len(sandboxes),
		sandboxActive:       byState[string(orchestrator.StateRunning)],
		sandboxesByState:    byState,
		sandboxesByProvider: byProvider,
		providerHealth:      providerHealth,
		healthyProviders:    healthyProviders,
		eventStats:          eventStats,
		operationMetrics:    s.manager.OperationMetrics(),
	}, nil
}

func (m systemMetricsSnapshot) toResponse() map[string]interface{} {
	return map[string]interface{}{
		"uptime":            m.uptime.String(),
		"goroutines":        m.goroutines,
		"memory_alloc":      m.memoryAlloc,
		"memory_sys":        m.memorySys,
		"memory_heap_alloc": m.memoryHeapAlloc,
		"gc_cycles":         m.gcCycles,
		"sandboxes": map[string]interface{}{
			"total":       m.sandboxTotal,
			"active":      m.sandboxActive,
			"by_state":    m.sandboxesByState,
			"by_provider": m.sandboxesByProvider,
		},
		"providers": map[string]interface{}{
			"total":   len(m.providerHealth),
			"healthy": m.healthyProviders,
			"items":   m.providerHealth,
		},
		"events":     m.eventStats,
		"operations": m.operationMetrics,
	}
}

func (s *SystemRoutes) providerHealth(ctx context.Context) []ProviderHealth {
	return collectProviderHealth(ctx, s.registry)
}

// Events serves Server-Sent Events for real-time updates.
//
//	@Summary		Subscribe to events
//	@Description	Open an SSE stream for real-time sandbox and system events
//	@Tags			system
//	@Produce		text/event-stream
//	@Success		200	{object}	orchestrator.Event
//	@Security		ApiKeyAuth
//	@Router			/events [get]
func (s *SystemRoutes) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, httputil.CodeInternal, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	subID := uuid.New().String()
	ch := s.events.Subscribe(subID)
	defer s.events.Unsubscribe(subID)

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
