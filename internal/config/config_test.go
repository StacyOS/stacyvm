package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("stacyvm.yaml", []byte(`
defaults:
  spawn_queue_timeout: "soon"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "defaults.spawn_queue_timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidEnums(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("stacyvm.yaml", []byte(`
defaults:
  spawn_overflow: "stall"
rate_limit:
  key_by: "cookie"
pool:
  overflow: "stall"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid enum error")
	}
	if !strings.Contains(err.Error(), "defaults.spawn_overflow") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsNegativeLimits(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("stacyvm.yaml", []byte(`
defaults:
  max_spawn_queue: -1
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected negative limit error")
	}
	if !strings.Contains(err.Error(), "defaults.max_spawn_queue") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAcceptsPhaseThreeConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("stacyvm.yaml", []byte(`
defaults:
  spawn_overflow: "queue"
  spawn_queue_timeout: "45s"
  max_spawn_queue: 25
rate_limit:
  enabled: true
  requests_per_minute: 240
  burst: 80
  key_by: "api_key"
  bucket_ttl: "30m"
  cleanup_interval: "2m"
auth:
  admin_api_key: "admin-secret"
  admin_fallback_enabled: false
  admin_audit_retention: "2160h"
pool:
  overflow: "queue"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Defaults.SpawnOverflow != "queue" || cfg.Defaults.SpawnQueueTimeout != "45s" || cfg.Defaults.MaxSpawnQueue != 25 {
		t.Fatalf("unexpected defaults config: %+v", cfg.Defaults)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.KeyBy != "api_key" || cfg.RateLimit.BucketTTL != "30m" {
		t.Fatalf("unexpected rate limit config: %+v", cfg.RateLimit)
	}
	if cfg.Auth.AdminAPIKey != "admin-secret" {
		t.Fatalf("admin api key = %q, want admin-secret", cfg.Auth.AdminAPIKey)
	}
	if cfg.Auth.AdminFallbackEnabled {
		t.Fatal("admin fallback enabled = true, want false")
	}
	if cfg.Auth.AdminAuditRetention != "2160h" {
		t.Fatalf("admin audit retention = %q, want 2160h", cfg.Auth.AdminAuditRetention)
	}
}

func TestLoadAcceptsWorkerRuntimeConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("stacyvm.yaml", []byte(`
worker:
  id: "worker-a"
  control_plane_url: "http://control-plane:7423"
  heartbeat_interval: "5s"
  shutdown_timeout: "15s"
auth:
  worker_token: "worker-secret"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Worker.ID != "worker-a" {
		t.Fatalf("worker id = %q, want worker-a", cfg.Worker.ID)
	}
	if cfg.Worker.ControlPlaneURL != "http://control-plane:7423" {
		t.Fatalf("control plane URL = %q", cfg.Worker.ControlPlaneURL)
	}
	if cfg.Auth.WorkerToken != "worker-secret" {
		t.Fatalf("worker token = %q, want worker-secret", cfg.Auth.WorkerToken)
	}
}
