package orchestrator

import "time"

type SandboxState string

const (
	StateCreating  SandboxState = "creating"
	StateRunning   SandboxState = "running"
	StateIdle      SandboxState = "idle"
	StateDestroyed SandboxState = "destroyed"
	StateError     SandboxState = "error"
)

type Sandbox struct {
	ID            string            `json:"id"`
	State         SandboxState      `json:"state"`
	Provider      string            `json:"provider"`
	Image         string            `json:"image"`
	MemoryMB      int               `json:"memory_mb"`
	VCPUs         int               `json:"vcpus"`
	OwnerID       string            `json:"owner_id,omitempty"`
	VMID          string            `json:"vm_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	PreviewDomain string            `json:"preview_domain,omitempty"`
}

type SpawnRequest struct {
	Image    string            `json:"image"`
	Provider string            `json:"provider,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	VCPUs    int               `json:"vcpus,omitempty"`
	TTL      string            `json:"ttl,omitempty"`
	Template string            `json:"template,omitempty"`
	OwnerID  string            `json:"owner_id,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ExecRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Mode    string            `json:"mode,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"workdir,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
	Stream  bool              `json:"stream,omitempty"`
}

type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
}

type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

type FileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

type FileDeleteRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type FileMoveRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type FileChmodRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type SandboxInfo struct {
	ID            string            `json:"id"`
	State         SandboxState      `json:"state"`
	Provider      string            `json:"provider"`
	Image         string            `json:"image"`
	MemoryMB      int               `json:"memory_mb"`
	VCPUs         int               `json:"vcpus"`
	CreatedAt     string            `json:"created_at"`
	ExpiresAt     string            `json:"expires_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	ExecCount     int               `json:"exec_count"`
	FileCount     int               `json:"file_count"`
	PreviewDomain string            `json:"preview_domain,omitempty"`
}

type OperationalLimits struct {
	MaxSandboxes         int           `json:"max_sandboxes"`
	MaxSandboxesPerOwner int           `json:"max_sandboxes_per_owner"`
	DefaultExecTimeout   time.Duration `json:"default_exec_timeout"`
	MaxExecTimeout       time.Duration `json:"max_exec_timeout"`
	MaxTTL               time.Duration `json:"max_ttl"`
	SpawnOverflow        string        `json:"spawn_overflow"`
	SpawnQueueTimeout    time.Duration `json:"spawn_queue_timeout"`
	MaxSpawnQueue        int           `json:"max_spawn_queue"`
}

type OperationalLimitsInfo struct {
	MaxSandboxes         int    `json:"max_sandboxes"`
	MaxSandboxesPerOwner int    `json:"max_sandboxes_per_owner"`
	DefaultExecTimeout   string `json:"default_exec_timeout"`
	MaxExecTimeout       string `json:"max_exec_timeout"`
	MaxTTL               string `json:"max_ttl"`
	SpawnOverflow        string `json:"spawn_overflow"`
	SpawnQueueTimeout    string `json:"spawn_queue_timeout"`
	MaxSpawnQueue        int    `json:"max_spawn_queue"`
}

type SchedulerStatus struct {
	SpawnOverflow         string `json:"spawn_overflow"`
	SpawnQueueDepth       int    `json:"spawn_queue_depth"`
	MaxSpawnQueue         int    `json:"max_spawn_queue"`
	SpawnQueueTimeout     string `json:"spawn_queue_timeout"`
	AdmissionControl      string `json:"admission_control"`
	SpawnQueuedTotal      uint64 `json:"spawn_queued_total"`
	SpawnDequeuedTotal    uint64 `json:"spawn_dequeued_total"`
	SpawnQueueTimeouts    uint64 `json:"spawn_queue_timeouts"`
	SpawnQueueWaitCount   uint64 `json:"spawn_queue_wait_count"`
	SpawnQueueWaitTotal   string `json:"spawn_queue_wait_total"`
	SpawnQueueWaitMax     string `json:"spawn_queue_wait_max"`
	SpawnQueueWaitAvg     string `json:"spawn_queue_wait_avg"`
	SpawnQueueWaitTotalMS int64  `json:"spawn_queue_wait_total_ms"`
	SpawnQueueWaitMaxMS   int64  `json:"spawn_queue_wait_max_ms"`
	SpawnQueueWaitAvgMS   int64  `json:"spawn_queue_wait_avg_ms"`
}

type SpawnAdmissionDecision struct {
	Allowed              bool   `json:"allowed"`
	Queueable            bool   `json:"queueable"`
	Reason               string `json:"reason,omitempty"`
	ActiveSandboxes      int    `json:"active_sandboxes"`
	MaxSandboxes         int    `json:"max_sandboxes"`
	ActiveOwnerSandboxes int    `json:"active_owner_sandboxes,omitempty"`
	MaxOwnerSandboxes    int    `json:"max_owner_sandboxes,omitempty"`
	MaxTTL               string `json:"max_ttl,omitempty"`
}

type OwnerQuota struct {
	OwnerID        string    `json:"owner_id"`
	MaxSandboxes   int       `json:"max_sandboxes"`
	MaxTTL         string    `json:"max_ttl"`
	MaxExecTimeout string    `json:"max_exec_timeout"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type OwnerUsage struct {
	OwnerID         string `json:"owner_id"`
	ActiveSandboxes int    `json:"active_sandboxes"`
	MaxSandboxes    int    `json:"max_sandboxes"`
	MaxTTL          string `json:"max_ttl"`
	MaxExecTimeout  string `json:"max_exec_timeout"`
	QuotaConfigured bool   `json:"quota_configured"`
}

type QuotaSummary struct {
	Total              int `json:"total"`
	WithMaxSandboxes   int `json:"with_max_sandboxes"`
	WithMaxTTL         int `json:"with_max_ttl"`
	WithMaxExecTimeout int `json:"with_max_exec_timeout"`
}
