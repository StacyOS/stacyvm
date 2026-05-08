package main

import (
	"testing"

	"github.com/StacyOs/stacyvm/internal/config"
)

func TestLintConfigProductionBaselinePasses(t *testing.T) {
	cfg := validProductionConfig()

	checks := lintConfig(cfg, true)
	for _, check := range checks {
		if check.Status == doctorFail {
			t.Fatalf("%s failed: %s", check.Name, check.Message)
		}
	}
}

func TestLintConfigProductionCatchesUnsafeSettings(t *testing.T) {
	cfg := validProductionConfig()
	cfg.Auth.Enabled = false
	cfg.Auth.APIKey = "change-me-generate-at-least-32-bytes"
	cfg.Auth.AdminAPIKey = cfg.Auth.APIKey
	cfg.Auth.AdminFallbackEnabled = true
	cfg.Auth.AdminAuditRetention = "0s"
	cfg.RateLimit.Enabled = false
	cfg.Database.Path = "stacyvm.db"
	cfg.Defaults.MaxSandboxes = 0
	cfg.Defaults.DefaultExecTimeout = "0s"
	cfg.Logging.Format = "console"
	cfg.Providers.Docker.SeccompProfile = "unconfined"
	cfg.Providers.Docker.AddedCaps = []string{"SYS_ADMIN"}
	cfg.Providers.Docker.DroppedCaps = nil
	cfg.Providers.Docker.PidsLimit = 0
	cfg.Providers.Docker.User = ""

	checks := lintConfig(cfg, true)
	statuses := map[string]doctorStatus{}
	for _, check := range checks {
		statuses[check.Name] = check.Status
	}

	for _, name := range []string{
		"auth.enabled",
		"auth.api_key",
		"auth.key_separation",
		"auth.admin_fallback_enabled",
		"auth.admin_audit_retention",
		"rate_limit.enabled",
		"database.path",
		"defaults.sandbox_caps",
		"defaults.exec_timeouts",
		"logging.format",
		"docker.seccomp_profile",
		"docker.added_caps",
		"docker.dropped_caps",
		"docker.resource_limits",
		"docker.user",
	} {
		if statuses[name] != doctorFail {
			t.Fatalf("%s status = %s, want %s", name, statuses[name], doctorFail)
		}
	}
}

func validProductionConfig() *config.Config {
	return &config.Config{
		Auth: config.AuthConfig{
			Enabled:              true,
			APIKey:               "regular-api-key-with-at-least-32-bytes",
			AdminAPIKey:          "admin-api-key-with-at-least-32-bytesxx",
			AdminFallbackEnabled: false,
			AdminAuditRetention:  "2160h",
		},
		RateLimit: config.RateLimitConfig{
			Enabled:           true,
			RequestsPerMinute: 120,
			Burst:             60,
			KeyBy:             "api_key",
		},
		Database: config.DatabaseConfig{
			Path: "/var/lib/stacyvm/stacyvm.db",
		},
		Defaults: config.DefaultsConfig{
			MaxSandboxes:         100,
			MaxSandboxesPerOwner: 10,
			MaxSpawnQueue:        100,
			DefaultExecTimeout:   "30s",
			MaxExecTimeout:       "10m",
			MaxTTL:               "24h",
		},
		Logging: config.LoggingConfig{
			Format: "json",
		},
		Providers: config.ProvidersConfig{
			Default: "docker",
			Docker: config.DockerConfig{
				Enabled:        true,
				Runtime:        "runc",
				NetworkMode:    "stacyvm-network",
				SeccompProfile: "default",
				Memory:         "512m",
				CPUs:           "1",
				PidsLimit:      256,
				User:           "1000:1000",
				DroppedCaps:    []string{"ALL"},
				AddedCaps:      []string{},
			},
		},
	}
}
