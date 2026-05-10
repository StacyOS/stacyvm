package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Worker    WorkerConfig    `mapstructure:"worker"`
	Providers ProvidersConfig `mapstructure:"providers"`
	Defaults  DefaultsConfig  `mapstructure:"defaults"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Pool      PoolConfig      `mapstructure:"pool"`
}

type PoolConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	MaxVMs        int    `mapstructure:"max_vms"`
	MaxUsersPerVM int    `mapstructure:"max_users_per_vm"`
	Image         string `mapstructure:"image"`
	MemoryMB      int    `mapstructure:"memory_mb"`
	VCPUs         int    `mapstructure:"vcpus"`
	Overflow      string `mapstructure:"overflow"` // "reject" or "queue"
}

type ServerConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	PreviewDomain string `mapstructure:"preview_domain"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type WorkerConfig struct {
	ID                string             `mapstructure:"id"`
	ControlPlaneURL   string             `mapstructure:"control_plane_url"`
	ListenAddr        string             `mapstructure:"listen_addr"`
	PreviewDomain     string             `mapstructure:"preview_domain"`
	HeartbeatInterval string             `mapstructure:"heartbeat_interval"`
	ShutdownTimeout   string             `mapstructure:"shutdown_timeout"`
	RPCTLS            WorkerRPCTLSConfig `mapstructure:"rpc_tls"`
}

type WorkerRPCTLSConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	ServerCertFile     string `mapstructure:"server_cert_file"`
	ServerKeyFile      string `mapstructure:"server_key_file"`
	ClientCAFile       string `mapstructure:"client_ca_file"`
	CAFile             string `mapstructure:"ca_file"`
	ClientCertFile     string `mapstructure:"client_cert_file"`
	ClientKeyFile      string `mapstructure:"client_key_file"`
	ServerName         string `mapstructure:"server_name"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

type ProvidersConfig struct {
	Default     string            `mapstructure:"default"`
	Mock        MockConfig        `mapstructure:"mock"`
	Firecracker FirecrackerConfig `mapstructure:"firecracker"`
	Docker      DockerConfig      `mapstructure:"docker"`
	E2B         E2BConfig         `mapstructure:"e2b"`
	Custom      CustomConfig      `mapstructure:"custom"`
	PRoot       PRootConfig       `mapstructure:"proot"`
}

type PRootConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	RootfsPath     string   `mapstructure:"rootfs_path"`
	PRootBinary    string   `mapstructure:"proot_binary"`
	WorkspaceBase  string   `mapstructure:"workspace_base"`
	DefaultTimeout string   `mapstructure:"default_timeout"`
	MaxSandboxes   int      `mapstructure:"max_sandboxes"`
	MaxMemoryMB    int      `mapstructure:"max_memory_mb"`
	MaxDiskMB      int      `mapstructure:"max_disk_mb"`
	Languages      []string `mapstructure:"languages"`
}

type DockerConfig struct {
	Enabled        bool               `mapstructure:"enabled"`
	Socket         string             `mapstructure:"socket"`
	Runtime        string             `mapstructure:"runtime"`
	DefaultImage   string             `mapstructure:"default_image"`
	NetworkMode    string             `mapstructure:"network_mode"`
	SeccompProfile string             `mapstructure:"seccomp_profile"`
	ReadOnlyRootfs bool               `mapstructure:"read_only_rootfs"`
	Memory         string             `mapstructure:"memory"`
	CPUs           string             `mapstructure:"cpus"`
	PidsLimit      int64              `mapstructure:"pids_limit"`
	User           string             `mapstructure:"user"`
	DroppedCaps    []string           `mapstructure:"dropped_caps"`
	AddedCaps      []string           `mapstructure:"added_caps"`
	Tmpfs          map[string]string  `mapstructure:"tmpfs"`
	PoolSecurity   PoolSecurityConfig `mapstructure:"pool_security"`
}

type PoolSecurityConfig struct {
	PerUserUID           bool `mapstructure:"per_user_uid"`
	PIDNamespace         bool `mapstructure:"pid_namespace"`
	WorkspacePermissions bool `mapstructure:"workspace_permissions"`
	HidePID              bool `mapstructure:"hidepid"`
}

type E2BConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

type CustomConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Name    string `mapstructure:"name"`
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Timeout string `mapstructure:"timeout"`
}

type MockConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type FirecrackerConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	FirecrackerPath string `mapstructure:"firecracker_path"`
	KernelPath      string `mapstructure:"kernel_path"`
	DefaultRootfs   string `mapstructure:"default_rootfs"`
	AgentPath       string `mapstructure:"agent_path"`
	DataDir         string `mapstructure:"data_dir"`
}

type DefaultsConfig struct {
	TTL                  string `mapstructure:"ttl"`
	Image                string `mapstructure:"image"`
	MemoryMB             int    `mapstructure:"memory_mb"`
	VCPUs                int    `mapstructure:"vcpus"`
	DiskSizeMB           int    `mapstructure:"disk_size_mb"`
	PoolSize             int    `mapstructure:"pool_size"`
	PoolTemplate         string `mapstructure:"pool_template"`
	MaxTTL               string `mapstructure:"max_ttl"`
	DefaultExecTimeout   string `mapstructure:"default_exec_timeout"`
	MaxExecTimeout       string `mapstructure:"max_exec_timeout"`
	MaxSandboxes         int    `mapstructure:"max_sandboxes"`
	MaxSandboxesPerOwner int    `mapstructure:"max_sandboxes_per_owner"`
	SpawnOverflow        string `mapstructure:"spawn_overflow"`
	SpawnQueueTimeout    string `mapstructure:"spawn_queue_timeout"`
	MaxSpawnQueue        int    `mapstructure:"max_spawn_queue"`
}

type AuthConfig struct {
	Enabled               bool              `mapstructure:"enabled"`
	APIKey                string            `mapstructure:"api_key"`
	AdminAPIKey           string            `mapstructure:"admin_api_key"`
	WorkerToken           string            `mapstructure:"worker_token"`
	WorkerTokenFile       string            `mapstructure:"worker_token_file"`
	WorkerTokens          map[string]string `mapstructure:"worker_tokens"`
	WorkerSigningKey      string            `mapstructure:"worker_signing_key"`
	WorkerSigningKeyFile  string            `mapstructure:"worker_signing_key_file"`
	WorkerSigningKeys     []string          `mapstructure:"worker_signing_keys"`
	WorkerRevokedTokenIDs []string          `mapstructure:"worker_revoked_token_ids"`
	AdminFallbackEnabled  bool              `mapstructure:"admin_fallback_enabled"`
	AdminAuditRetention   string            `mapstructure:"admin_audit_retention"`

	// OIDC/JWT configuration for enterprise SSO
	OIDCEnabled      bool     `mapstructure:"oidc_enabled"`
	OIDCIssuer       string   `mapstructure:"oidc_issuer"`
	OIDCAudience     string   `mapstructure:"oidc_audience"`
	OIDCJWKSUrl      string   `mapstructure:"oidc_jwks_url"`
	OIDCPublicKey    string   `mapstructure:"oidc_public_key"`
	OIDCPublicKeyFile string  `mapstructure:"oidc_public_key_file"`
	OIDCGroupsClaim  string   `mapstructure:"oidc_groups_claim"`
	OIDCTenantClaim  string   `mapstructure:"oidc_tenant_claim"`
	OIDCAdminGroups  []string `mapstructure:"oidc_admin_groups"`
	OIDCOperatorGroups []string `mapstructure:"oidc_operator_groups"`
	OIDCViewerGroups []string `mapstructure:"oidc_viewer_groups"`
}

type RateLimitConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	RequestsPerMinute int    `mapstructure:"requests_per_minute"`
	Burst             int    `mapstructure:"burst"`
	KeyBy             string `mapstructure:"key_by"`
	BucketTTL         string `mapstructure:"bucket_ttl"`
	CleanupInterval   string `mapstructure:"cleanup_interval"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	Path   string `mapstructure:"path"`
	DSN    string `mapstructure:"dsn"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 7423)
	v.SetDefault("server.preview_domain", "localhost")
	v.SetDefault("worker.id", "")
	v.SetDefault("worker.control_plane_url", "http://localhost:7423")
	v.SetDefault("worker.listen_addr", "")
	v.SetDefault("worker.preview_domain", "")
	v.SetDefault("worker.heartbeat_interval", "30s")
	v.SetDefault("worker.shutdown_timeout", "10s")
	v.SetDefault("worker.rpc_tls.enabled", false)
	v.SetDefault("worker.rpc_tls.server_cert_file", "")
	v.SetDefault("worker.rpc_tls.server_key_file", "")
	v.SetDefault("worker.rpc_tls.client_ca_file", "")
	v.SetDefault("worker.rpc_tls.ca_file", "")
	v.SetDefault("worker.rpc_tls.client_cert_file", "")
	v.SetDefault("worker.rpc_tls.client_key_file", "")
	v.SetDefault("worker.rpc_tls.server_name", "")
	v.SetDefault("worker.rpc_tls.insecure_skip_verify", false)

	v.SetDefault("providers.default", "docker")
	v.SetDefault("providers.mock.enabled", false)
	v.SetDefault("providers.firecracker.enabled", true)
	v.SetDefault("providers.firecracker.firecracker_path", "/usr/local/bin/firecracker")
	v.SetDefault("providers.firecracker.kernel_path", "/var/lib/stacyvm/vmlinux.bin")
	v.SetDefault("providers.firecracker.default_rootfs", "")
	v.SetDefault("providers.firecracker.agent_path", "./bin/stacyvm-agent")
	v.SetDefault("providers.firecracker.data_dir", "/var/lib/stacyvm")
	v.SetDefault("providers.e2b.enabled", false)
	v.SetDefault("providers.e2b.api_key", "")
	v.SetDefault("providers.e2b.base_url", "https://api.e2b.dev")
	v.SetDefault("providers.custom.enabled", false)
	v.SetDefault("providers.custom.name", "custom")
	v.SetDefault("providers.custom.base_url", "")
	v.SetDefault("providers.custom.api_key", "")
	v.SetDefault("providers.custom.timeout", "60s")

	v.SetDefault("providers.docker.enabled", true)
	v.SetDefault("providers.docker.socket", "unix:///var/run/docker.sock")
	v.SetDefault("providers.docker.runtime", "runc")
	v.SetDefault("providers.docker.default_image", "alpine:latest")
	v.SetDefault("providers.docker.network_mode", "bridge")
	v.SetDefault("providers.docker.seccomp_profile", "default")
	v.SetDefault("providers.docker.read_only_rootfs", false)
	v.SetDefault("providers.docker.memory", "512m")
	v.SetDefault("providers.docker.cpus", "1")
	v.SetDefault("providers.docker.pids_limit", 256)
	v.SetDefault("providers.docker.user", "")
	v.SetDefault("providers.docker.dropped_caps", []string{"ALL"})
	v.SetDefault("providers.docker.added_caps", []string{})
	v.SetDefault("providers.docker.pool_security.per_user_uid", false)
	v.SetDefault("providers.docker.pool_security.pid_namespace", false)
	v.SetDefault("providers.docker.pool_security.workspace_permissions", true)
	v.SetDefault("providers.docker.pool_security.hidepid", false)

	v.SetDefault("providers.proot.enabled", false)
	v.SetDefault("providers.proot.proot_binary", "proot")
	v.SetDefault("providers.proot.rootfs_path", "/var/lib/stacyvm/rootfs")
	v.SetDefault("providers.proot.workspace_base", "/var/lib/stacyvm/workspaces")
	v.SetDefault("providers.proot.default_timeout", "60s")
	v.SetDefault("providers.proot.max_sandboxes", 10)
	v.SetDefault("providers.proot.max_memory_mb", 512)
	v.SetDefault("providers.proot.max_disk_mb", 1024)
	v.SetDefault("providers.proot.languages", []string{"python3", "node", "bash"})

	v.SetDefault("defaults.ttl", "30m")
	v.SetDefault("defaults.image", "alpine:latest")
	v.SetDefault("defaults.memory_mb", 1024)
	v.SetDefault("defaults.vcpus", 1)
	v.SetDefault("defaults.disk_size_mb", 1024)
	v.SetDefault("defaults.pool_size", 0)
	v.SetDefault("defaults.pool_template", "")
	v.SetDefault("defaults.max_ttl", "24h")
	v.SetDefault("defaults.default_exec_timeout", "0s")
	v.SetDefault("defaults.max_exec_timeout", "10m")
	v.SetDefault("defaults.max_sandboxes", 0)
	v.SetDefault("defaults.max_sandboxes_per_owner", 0)
	v.SetDefault("defaults.spawn_overflow", "reject")
	v.SetDefault("defaults.spawn_queue_timeout", "30s")
	v.SetDefault("defaults.max_spawn_queue", 100)

	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.api_key", "")
	v.SetDefault("auth.admin_api_key", "")
	v.SetDefault("auth.worker_token", "")
	v.SetDefault("auth.worker_token_file", "")
	v.SetDefault("auth.worker_tokens", map[string]string{})
	v.SetDefault("auth.worker_signing_key", "")
	v.SetDefault("auth.worker_signing_key_file", "")
	v.SetDefault("auth.worker_signing_keys", []string{})
	v.SetDefault("auth.worker_revoked_token_ids", []string{})
	v.SetDefault("auth.admin_fallback_enabled", true)
	v.SetDefault("auth.admin_audit_retention", "0s")
	v.SetDefault("auth.oidc_enabled", false)
	v.SetDefault("auth.oidc_issuer", "")
	v.SetDefault("auth.oidc_audience", "")
	v.SetDefault("auth.oidc_jwks_url", "")
	v.SetDefault("auth.oidc_public_key", "")
	v.SetDefault("auth.oidc_public_key_file", "")
	v.SetDefault("auth.oidc_groups_claim", "groups")
	v.SetDefault("auth.oidc_tenant_claim", "tenant_id")
	v.SetDefault("auth.oidc_admin_groups", []string{})
	v.SetDefault("auth.oidc_operator_groups", []string{})
	v.SetDefault("auth.oidc_viewer_groups", []string{})

	v.SetDefault("rate_limit.enabled", false)
	v.SetDefault("rate_limit.requests_per_minute", 120)
	v.SetDefault("rate_limit.burst", 60)
	v.SetDefault("rate_limit.key_by", "owner")
	v.SetDefault("rate_limit.bucket_ttl", "15m")
	v.SetDefault("rate_limit.cleanup_interval", "1m")

	v.SetDefault("database.path", "stacyvm.db")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	v.SetDefault("pool.enabled", false)
	v.SetDefault("pool.max_vms", 10)
	v.SetDefault("pool.max_users_per_vm", 5)
	v.SetDefault("pool.image", "alpine:latest")
	v.SetDefault("pool.memory_mb", 2048)
	v.SetDefault("pool.vcpus", 2)
	v.SetDefault("pool.overflow", "reject")
}

// ResolveAgentPath resolves the agent_path to an absolute path.
// If the path is relative, it tries (in order):
//  1. Relative to the executable's directory
//  2. Relative to CWD
//  3. PATH lookup via exec.LookPath (for /usr/local/bin installs)
func (c *Config) ResolveAgentPath() string {
	p := c.Providers.Firecracker.AgentPath
	if filepath.IsAbs(p) {
		return p
	}
	// Try relative to executable first
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), p)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Try CWD-relative
	if abs, err := filepath.Abs(p); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	// Fallback: look up the binary name in PATH
	base := filepath.Base(p)
	if found, err := exec.LookPath(base); err == nil {
		return found
	}
	// Last resort: return as-is
	return p
}

func Load() (*Config, error) {
	configPaths := []string{"stacyvm.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths, filepath.Join(home, ".stacyvm", "config.yaml"))
	}
	return load(configPaths, false)
}

func LoadFile(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("config path is required")
	}
	return load([]string{path}, true)
}

func load(configPaths []string, requireConfig bool) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigType("yaml")

	v.SetEnvPrefix("STACYVM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	loaded := false
	for _, p := range configPaths {
		if _, err := os.Stat(p); err == nil {
			v.SetConfigFile(p)
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("reading config %s: %w", p, err)
			}
			loaded = true
			break
		} else if requireConfig {
			return nil, fmt.Errorf("config file %s: %w", p, err)
		}
	}
	_ = loaded // defaults are fine if no config file found

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}
	if err := cfg.resolveAuthSecretFiles(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) resolveAuthSecretFiles() error {
	if err := resolveSecretFile("auth.worker_token", &c.Auth.WorkerToken, "auth.worker_token_file", c.Auth.WorkerTokenFile); err != nil {
		return err
	}
	if err := resolveSecretFile("auth.worker_signing_key", &c.Auth.WorkerSigningKey, "auth.worker_signing_key_file", c.Auth.WorkerSigningKeyFile); err != nil {
		return err
	}
	if err := resolveSecretFile("auth.oidc_public_key", &c.Auth.OIDCPublicKey, "auth.oidc_public_key_file", c.Auth.OIDCPublicKeyFile); err != nil {
		return err
	}
	return nil
}

func resolveSecretFile(valueName string, value *string, fileName, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if strings.TrimSpace(*value) != "" {
		return fmt.Errorf("%s and %s cannot both be set", valueName, fileName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", fileName, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return fmt.Errorf("%s is empty", fileName)
	}
	*value = secret
	return nil
}

func (c *Config) Validate() error {
	switch driver := strings.ToLower(strings.TrimSpace(c.Database.Driver)); driver {
	case "", "sqlite", "sqlite3":
		if strings.TrimSpace(c.Database.Path) == "" {
			return fmt.Errorf("database.path is required for sqlite")
		}
	case "postgres", "postgresql":
		if strings.TrimSpace(c.Database.DSN) == "" {
			return fmt.Errorf("database.dsn is required for postgres")
		}
	default:
		return fmt.Errorf("unsupported database.driver %q", c.Database.Driver)
	}

	durationFields := map[string]string{
		"defaults.ttl":                    c.Defaults.TTL,
		"defaults.max_ttl":                c.Defaults.MaxTTL,
		"defaults.default_exec_timeout":   c.Defaults.DefaultExecTimeout,
		"defaults.max_exec_timeout":       c.Defaults.MaxExecTimeout,
		"defaults.spawn_queue_timeout":    c.Defaults.SpawnQueueTimeout,
		"rate_limit.bucket_ttl":           c.RateLimit.BucketTTL,
		"rate_limit.cleanup_interval":     c.RateLimit.CleanupInterval,
		"auth.admin_audit_retention":      c.Auth.AdminAuditRetention,
		"worker.heartbeat_interval":       c.Worker.HeartbeatInterval,
		"worker.shutdown_timeout":         c.Worker.ShutdownTimeout,
		"providers.custom.timeout":        c.Providers.Custom.Timeout,
		"providers.proot.default_timeout": c.Providers.PRoot.DefaultTimeout,
	}
	for name, value := range durationFields {
		if err := validateDuration(name, value); err != nil {
			return err
		}
	}

	if c.Defaults.MaxSandboxes < 0 {
		return fmt.Errorf("defaults.max_sandboxes cannot be negative")
	}
	if c.Defaults.MaxSandboxesPerOwner < 0 {
		return fmt.Errorf("defaults.max_sandboxes_per_owner cannot be negative")
	}
	if c.Defaults.MaxSpawnQueue < 0 {
		return fmt.Errorf("defaults.max_spawn_queue cannot be negative")
	}
	if c.RateLimit.RequestsPerMinute < 0 {
		return fmt.Errorf("rate_limit.requests_per_minute cannot be negative")
	}
	if c.RateLimit.Burst < 0 {
		return fmt.Errorf("rate_limit.burst cannot be negative")
	}
	if !isOneOf(c.Defaults.SpawnOverflow, "", "reject", "queue") {
		return fmt.Errorf("defaults.spawn_overflow must be reject or queue")
	}
	if !isOneOf(c.RateLimit.KeyBy, "", "owner", "api_key", "ip") {
		return fmt.Errorf("rate_limit.key_by must be owner, api_key, or ip")
	}
	if !isOneOf(c.Pool.Overflow, "", "reject", "queue") {
		return fmt.Errorf("pool.overflow must be reject or queue")
	}
	return nil
}

func validateDuration(name, value string) error {
	if value == "" {
		return nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid duration: %w", name, err)
	}
	if d < 0 {
		return fmt.Errorf("%s cannot be negative", name)
	}
	return nil
}

func isOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
