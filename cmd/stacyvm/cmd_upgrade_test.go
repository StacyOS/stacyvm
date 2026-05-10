package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunUpgradeRehearsalPassesWithValidInputs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stacyvm.db")
	configPath := writeUpgradeTestConfig(t, dir, dbPath)
	createTestSQLiteDB(t, dbPath)

	report, err := runUpgradeRehearsal(context.Background(), upgradeRehearsalOptions{
		ConfigPath:   configPath,
		DatabasePath: dbPath,
		BackupOutput: filepath.Join(dir, "backup.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ProductionReady {
		t.Fatalf("ProductionReady = false, config=%v database=%v", report.ConfigLint, report.DatabaseChecks)
	}
	if !report.RequiresLiveCheck {
		t.Fatalf("RequiresLiveCheck = false, want true when doctor is skipped")
	}
}

func TestRunUpgradeRehearsalFailsWhenBackupExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stacyvm.db")
	backupPath := filepath.Join(dir, "backup.db")
	configPath := writeUpgradeTestConfig(t, dir, dbPath)
	createTestSQLiteDB(t, dbPath)
	if err := os.WriteFile(backupPath, []byte("exists"), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := runUpgradeRehearsal(context.Background(), upgradeRehearsalOptions{
		ConfigPath:   configPath,
		DatabasePath: dbPath,
		BackupOutput: backupPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ProductionReady {
		t.Fatalf("ProductionReady = true, want false")
	}
	if got := statusFor(report.DatabaseChecks, "backup.output"); got != doctorFail {
		t.Fatalf("backup.output status = %s, want %s", got, doctorFail)
	}
}

func writeUpgradeTestConfig(t *testing.T, dir, dbPath string) string {
	t.Helper()
	path := filepath.Join(dir, "stacyvm.yaml")
	body := `server:
  cors_allowed_origins:
    - "https://console.example.com"
auth:
  enabled: true
  api_key: "regular-api-key-with-at-least-32-bytes"
  admin_api_key: "admin-api-key-with-at-least-32-bytesxx"
  admin_fallback_enabled: false
  admin_audit_retention: "2160h"
rate_limit:
  enabled: true
  requests_per_minute: 120
  burst: 60
  key_by: "api_key"
database:
  path: "` + dbPath + `"
defaults:
  max_sandboxes: 100
  max_sandboxes_per_owner: 10
  max_spawn_queue: 100
  default_exec_timeout: "30s"
  max_exec_timeout: "10m"
  max_ttl: "24h"
logging:
  format: "json"
providers:
  default: "docker"
  docker:
    enabled: true
    runtime: "runc"
    network_mode: "stacyvm-network"
    seccomp_profile: "default"
    memory: "512m"
    cpus: "1"
    pids_limit: 256
    user: "1000:1000"
    dropped_caps: ["ALL"]
    added_caps: []
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func createTestSQLiteDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE sandboxes (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
}

func statusFor(checks []doctorCheck, name string) doctorStatus {
	for _, check := range checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}
