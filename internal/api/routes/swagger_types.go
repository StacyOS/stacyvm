package routes

import (
	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
)

// StatusResponse is a generic status response.
type StatusResponse struct {
	Status string `json:"status" example:"destroyed"`
}

// PruneResponse is the response from pruning sandboxes.
type PruneResponse struct {
	Pruned int `json:"pruned" example:"3"`
}

// HealthResponse is the response from the health check endpoint.
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"1.0.0"`
	Uptime  string `json:"uptime" example:"2h30m15s"`
}

// ProviderHealth is a provider readiness item.
type ProviderHealth struct {
	Name         string   `json:"name" example:"docker"`
	Healthy      bool     `json:"healthy" example:"true"`
	Default      bool     `json:"default" example:"true"`
	LatencyMS    int64    `json:"latency_ms" example:"3"`
	LastChecked  string   `json:"last_checked" example:"2026-05-08T10:30:00Z"`
	Error        string   `json:"error,omitempty" example:"health check returned false"`
	Capabilities []string `json:"capabilities" example:"spawn,exec,files"`
	RuntimeCount *int     `json:"runtime_count,omitempty" example:"2"`
}

// ReadinessResponse is the response from the readiness endpoint.
type ReadinessResponse struct {
	Status         string           `json:"status" example:"ready"`
	Version        string           `json:"version" example:"1.0.0"`
	Uptime         string           `json:"uptime" example:"2h30m15s"`
	Providers      []ProviderHealth `json:"providers"`
	ReadyProviders int              `json:"ready_providers" example:"1"`
	TotalProviders int              `json:"total_providers" example:"2"`
}

// DiagnosticsResponse is the response from the diagnostics endpoint.
type DiagnosticsResponse struct {
	GeneratedAt string                             `json:"generated_at" example:"2026-05-08T10:30:00Z"`
	Build       map[string]interface{}             `json:"build"`
	Process     map[string]interface{}             `json:"process"`
	Store       map[string]interface{}             `json:"store"`
	Limits      orchestrator.OperationalLimitsInfo `json:"limits"`
	Providers   []ProviderHealth                   `json:"providers"`
	Sandboxes   map[string]interface{}             `json:"sandboxes"`
	Events      orchestrator.EventBusStats         `json:"events"`
	Operations  []orchestrator.OperationMetrics    `json:"operations"`
	Scheduler   orchestrator.SchedulerStatus       `json:"scheduler"`
	Quotas      orchestrator.QuotaSummary          `json:"quotas"`
	RateLimit   middleware.RateLimitStats          `json:"rate_limit"`
	Redactions  []string                           `json:"redactions"`
}

// OwnerQuotaResponse is the response for owner quota configuration.
type OwnerQuotaResponse = orchestrator.OwnerQuota

// OwnerUsageResponse is the response for owner quota usage.
type OwnerUsageResponse = orchestrator.OwnerUsage

// MetricsResponse is the response from the metrics endpoint.
type MetricsResponse struct {
	SandboxesActive int    `json:"sandboxes_active" example:"5"`
	Providers       int    `json:"providers" example:"2"`
	Goroutines      int    `json:"goroutines" example:"12"`
	MemoryAllocMB   uint64 `json:"memory_alloc_mb" example:"64"`
	Uptime          string `json:"uptime" example:"2h30m15s"`
}

// TemplateSpawnOverrides are optional overrides when spawning from a template.
type TemplateSpawnOverrides struct {
	Provider string `json:"provider,omitempty" example:"firecracker"`
	TTL      string `json:"ttl,omitempty" example:"30m"`
}

// ConsoleLogResponse is the response from the console log endpoint.
type ConsoleLogResponse struct {
	Lines []string `json:"lines"`
}
