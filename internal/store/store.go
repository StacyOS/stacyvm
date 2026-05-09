package store

import (
	"context"
	"time"
)

type SandboxRecord struct {
	ID        string
	State     string
	Provider  string
	Image     string
	MemoryMB  int
	VCPUs     int
	Metadata  string // JSON
	OwnerID   string
	VMID      string
	WorkerID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	UpdatedAt time.Time
}

type ExecLogRecord struct {
	ID        int64
	SandboxID string
	Command   string
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  string
	CreatedAt time.Time
}

type ProviderConfigRecord struct {
	Name      string
	Config    string // JSON
	Enabled   bool
	UpdatedAt time.Time
}

type TemplateRecord struct {
	Name         string
	Version      int
	Image        string
	Description  string
	Setup        string // JSON array
	AllowedHosts string // JSON array
	MemoryMB     int
	CPUCores     int
	TTLSeconds   int
	Env          string // JSON object
	Secrets      string // JSON array
	PoolSize     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type EnvironmentSpecRecord struct {
	ID             string
	OwnerID        string
	Name           string
	BaseImage      string
	PythonPackages string // JSON array
	AptPackages    string // JSON array
	PythonVersion  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type EnvironmentBuildRecord struct {
	ID             string
	SpecID         string
	Status         string
	CurrentStep    string
	LogBlob        string
	ImageSizeBytes int64
	DigestLocal    string
	Error          string
	CreatedAt      time.Time
	FinishedAt     *time.Time
	UpdatedAt      time.Time
}

type EnvironmentArtifactRecord struct {
	ID        int64
	BuildID   string
	Target    string
	ImageRef  string
	Digest    string
	Status    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type RegistryConnectionRecord struct {
	ID        string
	OwnerID   string
	Provider  string
	Username  string
	SecretRef string
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OwnerQuotaRecord struct {
	OwnerID               string
	MaxSandboxes          int
	MaxTTLSeconds         int64
	MaxExecTimeoutSeconds int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AdminAuditRecord struct {
	ID         int64     `json:"id" example:"42"`
	Actor      string    `json:"actor" example:"admin"`
	Method     string    `json:"method" example:"PUT"`
	Path       string    `json:"path" example:"/api/v1/admin/quotas/owner-a"`
	Status     int       `json:"status" example:"200"`
	DurationMS int64     `json:"duration_ms" example:"4"`
	RequestID  string    `json:"request_id" example:"req-abc123"`
	RemoteAddr string    `json:"remote_addr" example:"127.0.0.1"`
	UserAgent  string    `json:"user_agent" example:"stacyvm-web"`
	CreatedAt  time.Time `json:"created_at" example:"2026-05-08T10:30:00Z"`
}

type AdminAuditQuery struct {
	Limit    int
	Actor    string
	Method   string
	Status   int
	PathLike string
}

type OperationAuditRecord struct {
	ID        int64     `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	SandboxID string    `json:"sandbox_id"`
	Resource  string    `json:"resource"`
	Provider  string    `json:"provider"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type OperationAuditQuery struct {
	Limit     int
	Actor     string
	Action    string
	SandboxID string
	Resource  string
	Status    string
}

type WorkerRecord struct {
	ID            string    `json:"id" example:"worker-local"`
	Hostname      string    `json:"hostname" example:"stacyvm-host-1"`
	Status        string    `json:"status" example:"online"`
	Providers     string    `json:"providers"`    // JSON array
	Capabilities  string    `json:"capabilities"` // JSON array
	Capacity      string    `json:"capacity"`     // JSON object
	LastHeartbeat time.Time `json:"last_heartbeat" example:"2026-05-09T10:30:00Z"`
	CreatedAt     time.Time `json:"created_at" example:"2026-05-09T10:00:00Z"`
	UpdatedAt     time.Time `json:"updated_at" example:"2026-05-09T10:30:00Z"`
}

type LeaseRecord struct {
	ResourceID   string    `json:"resource_id" example:"sb-abc123"`
	ResourceType string    `json:"resource_type" example:"sandbox"`
	HolderID     string    `json:"holder_id" example:"worker-local"`
	Generation   int64     `json:"generation" example:"3"`
	ExpiresAt    time.Time `json:"expires_at" example:"2026-05-09T10:31:00Z"`
	CreatedAt    time.Time `json:"created_at" example:"2026-05-09T10:30:00Z"`
	UpdatedAt    time.Time `json:"updated_at" example:"2026-05-09T10:30:30Z"`
}

// Store defines the persistence interface.
type Store interface {
	// Sandbox CRUD
	CreateSandbox(ctx context.Context, sb *SandboxRecord) error
	GetSandbox(ctx context.Context, id string) (*SandboxRecord, error)
	ListSandboxes(ctx context.Context) ([]*SandboxRecord, error)
	UpdateSandboxState(ctx context.Context, id string, state string) error
	UpdateSandboxExpiresAt(ctx context.Context, id string, expiresAt time.Time) error
	DeleteSandbox(ctx context.Context, id string) error
	ListExpiredSandboxes(ctx context.Context, before time.Time) ([]*SandboxRecord, error)
	ListSandboxesByOwner(ctx context.Context, ownerID string) ([]*SandboxRecord, error)
	CountSandboxesByVM(ctx context.Context, vmID string) (int, error)

	// Owner quotas
	SaveOwnerQuota(ctx context.Context, quota *OwnerQuotaRecord) error
	GetOwnerQuota(ctx context.Context, ownerID string) (*OwnerQuotaRecord, error)
	ListOwnerQuotas(ctx context.Context) ([]*OwnerQuotaRecord, error)
	DeleteOwnerQuota(ctx context.Context, ownerID string) error

	// Admin audit
	CreateAdminAudit(ctx context.Context, rec *AdminAuditRecord) error
	ListAdminAudit(ctx context.Context, query AdminAuditQuery) ([]*AdminAuditRecord, error)
	DeleteAdminAuditBefore(ctx context.Context, before time.Time) (int64, error)

	// Operation audit
	CreateOperationAudit(ctx context.Context, rec *OperationAuditRecord) error
	ListOperationAudit(ctx context.Context, query OperationAuditQuery) ([]*OperationAuditRecord, error)

	// Workers
	SaveWorker(ctx context.Context, rec *WorkerRecord) error
	GetWorker(ctx context.Context, id string) (*WorkerRecord, error)
	ListWorkers(ctx context.Context) ([]*WorkerRecord, error)
	DeleteWorker(ctx context.Context, id string) error

	// Leases
	AcquireLease(ctx context.Context, resourceID, resourceType, holderID string, ttl time.Duration) (*LeaseRecord, error)
	RenewLease(ctx context.Context, resourceID, holderID string, ttl time.Duration) (*LeaseRecord, error)
	GetLease(ctx context.Context, resourceID string) (*LeaseRecord, error)
	ListLeases(ctx context.Context) ([]*LeaseRecord, error)
	ReleaseLease(ctx context.Context, resourceID, holderID string) error

	// Exec logs
	CreateExecLog(ctx context.Context, log *ExecLogRecord) error
	ListExecLogs(ctx context.Context, sandboxID string) ([]*ExecLogRecord, error)

	// Provider configs
	GetProviderConfig(ctx context.Context, name string) (*ProviderConfigRecord, error)
	SaveProviderConfig(ctx context.Context, cfg *ProviderConfigRecord) error
	ListProviderConfigs(ctx context.Context) ([]*ProviderConfigRecord, error)

	// Templates
	CreateTemplate(ctx context.Context, t *TemplateRecord) error
	GetTemplate(ctx context.Context, name string) (*TemplateRecord, error)
	ListTemplates(ctx context.Context) ([]*TemplateRecord, error)
	UpdateTemplate(ctx context.Context, t *TemplateRecord) error
	DeleteTemplate(ctx context.Context, name string) error

	// Environment specs
	CreateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error
	GetEnvironmentSpec(ctx context.Context, id string) (*EnvironmentSpecRecord, error)
	ListEnvironmentSpecs(ctx context.Context, ownerID string) ([]*EnvironmentSpecRecord, error)
	UpdateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error
	DeleteEnvironmentSpec(ctx context.Context, id string) error

	// Environment builds
	CreateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error
	GetEnvironmentBuild(ctx context.Context, id string) (*EnvironmentBuildRecord, error)
	ListEnvironmentBuilds(ctx context.Context, specID string) ([]*EnvironmentBuildRecord, error)
	UpdateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error

	// Environment artifacts
	SaveEnvironmentArtifact(ctx context.Context, artifact *EnvironmentArtifactRecord) error
	ListEnvironmentArtifacts(ctx context.Context, buildID string) ([]*EnvironmentArtifactRecord, error)

	// Registry connections
	SaveRegistryConnection(ctx context.Context, conn *RegistryConnectionRecord) error
	GetRegistryConnection(ctx context.Context, id string) (*RegistryConnectionRecord, error)
	ListRegistryConnections(ctx context.Context, ownerID string) ([]*RegistryConnectionRecord, error)
	DeleteRegistryConnection(ctx context.Context, id string) error

	// Lifecycle
	Close() error
}
