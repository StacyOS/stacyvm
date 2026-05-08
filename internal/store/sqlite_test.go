package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open creates tables
	s1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	// Second open is idempotent
	s2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s2.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}
}

func TestSQLiteStoreMigratesLegacyDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := db.Exec(migrations[0].sql); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (1)"); err != nil {
		t.Fatalf("record v1 migration: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate legacy db: %v", err)
	}

	var migrated int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrated); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrated != len(migrations) {
		t.Fatalf("migration count = %d, want %d", migrated, len(migrations))
	}

	for _, col := range []string{"template", "owner_id", "vm_id"} {
		if !sqliteColumnExists(t, s.db, "sandboxes", col) {
			t.Fatalf("sandboxes missing migrated column %s", col)
		}
	}

	for _, table := range []string{
		"templates",
		"environment_specs",
		"environment_builds",
		"environment_artifacts",
		"registry_connections",
		"owner_quotas",
		"admin_audit_logs",
		"operation_audit_logs",
	} {
		if !sqliteTableExists(t, s.db, table) {
			t.Fatalf("missing migrated table %s", table)
		}
	}

	if err := s.Close(); err != nil {
		t.Fatalf("close migrated db: %v", err)
	}
	s, err = NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen migrated db: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close reopened db: %v", err)
	}
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
	return false
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return count == 1
}

func TestSandboxCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID:        "sb-00000001",
		State:     "running",
		Provider:  "mock",
		Image:     "alpine:latest",
		MemoryMB:  512,
		VCPUs:     1,
		Metadata:  `{"env":"test"}`,
		CreatedAt: now,
		ExpiresAt: now.Add(30 * time.Minute),
		UpdatedAt: now,
	}

	// Create
	if err := s.CreateSandbox(ctx, sb); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get
	got, err := s.GetSandbox(ctx, "sb-00000001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != "running" {
		t.Fatalf("expected running, got %s", got.State)
	}
	if got.Image != "alpine:latest" {
		t.Fatalf("expected alpine:latest, got %s", got.Image)
	}

	// List
	list, err := s.ListSandboxes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	// Update state
	if err := s.UpdateSandboxState(ctx, "sb-00000001", "idle"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetSandbox(ctx, "sb-00000001")
	if got.State != "idle" {
		t.Fatalf("expected idle, got %s", got.State)
	}

	// Delete (sets state to destroyed)
	if err := s.DeleteSandbox(ctx, "sb-00000001"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = s.ListSandboxes(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestExecLogs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Need a sandbox first
	sb := &SandboxRecord{
		ID: "sb-00000002", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	log := &ExecLogRecord{
		SandboxID: "sb-00000002",
		Command:   "echo hello",
		ExitCode:  0,
		Stdout:    "hello\n",
		Stderr:    "",
		Duration:  "5ms",
		CreatedAt: now,
	}

	if err := s.CreateExecLog(ctx, log); err != nil {
		t.Fatalf("create exec log: %v", err)
	}

	logs, err := s.ListExecLogs(ctx, "sb-00000002")
	if err != nil {
		t.Fatalf("list exec logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Command != "echo hello" {
		t.Fatalf("expected 'echo hello', got %q", logs[0].Command)
	}
}

func TestAdminAuditLogs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := s.CreateAdminAudit(ctx, &AdminAuditRecord{
		Actor:      "operator-a",
		Method:     "PUT",
		Path:       "/api/v1/admin/quotas/owner-a",
		Status:     200,
		DurationMS: 7,
		RequestID:  "req-a",
		RemoteAddr: "127.0.0.1",
		UserAgent:  "test-agent",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("create admin audit: %v", err)
	}

	if err := s.CreateAdminAudit(ctx, &AdminAuditRecord{
		Actor:     "operator-b",
		Method:    "GET",
		Path:      "/api/v1/admin/diagnostics",
		Status:    200,
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create second admin audit: %v", err)
	}

	records, err := s.ListAdminAudit(ctx, AdminAuditQuery{Limit: 1})
	if err != nil {
		t.Fatalf("list admin audit: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Actor != "operator-b" || records[0].Path != "/api/v1/admin/diagnostics" {
		t.Fatalf("unexpected latest audit record: %+v", records[0])
	}

	records, err = s.ListAdminAudit(ctx, AdminAuditQuery{Actor: "operator-a", Method: "PUT", Status: 200, PathLike: "quotas"})
	if err != nil {
		t.Fatalf("filter admin audit: %v", err)
	}
	if len(records) != 1 || records[0].Actor != "operator-a" {
		t.Fatalf("unexpected filtered audit records: %+v", records)
	}

	deleted, err := s.DeleteAdminAuditBefore(ctx, now.Add(500*time.Millisecond))
	if err != nil {
		t.Fatalf("delete old admin audit: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	records, err = s.ListAdminAudit(ctx, AdminAuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list remaining admin audit: %v", err)
	}
	if len(records) != 1 || records[0].Actor != "operator-b" {
		t.Fatalf("unexpected remaining audit records: %+v", records)
	}
}

func TestOperationAuditLogs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := s.CreateOperationAudit(ctx, &OperationAuditRecord{
		Actor:     "owner-a",
		Action:    "file.write",
		SandboxID: "sb-00000003",
		Resource:  "/workspace/app.py",
		Provider:  "mock",
		Status:    "success",
		Detail:    "mode=0644",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation audit: %v", err)
	}
	if err := s.CreateOperationAudit(ctx, &OperationAuditRecord{
		Actor:     "owner-b",
		Action:    "exec",
		SandboxID: "sb-00000004",
		Provider:  "mock",
		Status:    "failure",
		Detail:    "exit=1",
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create second operation audit: %v", err)
	}

	records, err := s.ListOperationAudit(ctx, OperationAuditQuery{Limit: 10, Action: "file.write", SandboxID: "sb-00000003"})
	if err != nil {
		t.Fatalf("list operation audit: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 operation audit record, got %d", len(records))
	}
	if records[0].Actor != "owner-a" || records[0].Resource != "/workspace/app.py" {
		t.Fatalf("unexpected operation audit record: %+v", records[0])
	}
}

func TestProviderConfigs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	cfg := &ProviderConfigRecord{
		Name:    "mock",
		Config:  `{"enabled": true}`,
		Enabled: true,
	}

	if err := s.SaveProviderConfig(ctx, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetProviderConfig(ctx, "mock")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled")
	}

	// Upsert
	cfg.Config = `{"enabled": true, "fast": true}`
	if err := s.SaveProviderConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	list, err := s.ListProviderConfigs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestExpiredSandboxes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// One expired, one not
	s.CreateSandbox(ctx, &SandboxRecord{
		ID: "sb-expired", State: "running", Provider: "mock",
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), UpdatedAt: now,
	})
	s.CreateSandbox(ctx, &SandboxRecord{
		ID: "sb-active", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), UpdatedAt: now,
	})

	expired, err := s.ListExpiredSandboxes(ctx, now)
	if err != nil {
		t.Fatalf("list expired: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].ID != "sb-expired" {
		t.Fatalf("expected sb-expired, got %s", expired[0].ID)
	}
}

func TestUpdateSandboxExpiresAt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID: "sb-extend01", State: "running", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	// Extend by 30 minutes
	newExpiry := sb.ExpiresAt.Add(30 * time.Minute)
	if err := s.UpdateSandboxExpiresAt(ctx, "sb-extend01", newExpiry); err != nil {
		t.Fatalf("update expires_at: %v", err)
	}

	got, _ := s.GetSandbox(ctx, "sb-extend01")
	// Compare truncated to seconds since SQLite doesn't store sub-second
	if got.ExpiresAt.Truncate(time.Second) != newExpiry.Truncate(time.Second) {
		t.Fatalf("expected expires_at %v, got %v", newExpiry, got.ExpiresAt)
	}
}

func TestUpdateSandboxExpiresAt_Destroyed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	sb := &SandboxRecord{
		ID: "sb-extend02", State: "destroyed", Provider: "mock",
		CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute), UpdatedAt: now,
	}
	s.CreateSandbox(ctx, sb)

	err := s.UpdateSandboxExpiresAt(ctx, "sb-extend02", now.Add(time.Hour))
	if err == nil {
		t.Fatal("expected error extending destroyed sandbox")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateSandboxExpiresAt_NotFound(t *testing.T) {
	s := testStore(t)
	err := s.UpdateSandboxExpiresAt(context.Background(), "sb-nope", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetSandbox(context.Background(), "sb-nope")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEnvironmentSpecCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &EnvironmentSpecRecord{
		ID:             "envspec-1",
		OwnerID:        "user-1",
		Name:           "py-ds",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `["pandas","numpy"]`,
		AptPackages:    `["curl"]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}
	conflicting := *spec
	conflicting.ID = "envspec-conflict"
	if err := s.CreateEnvironmentSpec(ctx, &conflicting); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate owner/name, got %v", err)
	}

	got, err := s.GetEnvironmentSpec(ctx, spec.ID)
	if err != nil {
		t.Fatalf("get spec: %v", err)
	}
	if got.BaseImage != "python:3.12-slim" {
		t.Fatalf("base image mismatch: %s", got.BaseImage)
	}

	spec.Name = "py-ds-v2"
	spec.PythonPackages = `["pandas","numpy","matplotlib"]`
	if err := s.UpdateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("update spec: %v", err)
	}

	list, err := s.ListEnvironmentSpecs(ctx, "user-1")
	if err != nil {
		t.Fatalf("list specs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(list))
	}

	if err := s.DeleteEnvironmentSpec(ctx, spec.ID); err != nil {
		t.Fatalf("delete spec: %v", err)
	}
	if _, err := s.GetEnvironmentSpec(ctx, spec.ID); err == nil {
		t.Fatal("expected get to fail after delete")
	} else if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestEnvironmentBuildArtifactAndRegistryCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spec := &EnvironmentSpecRecord{
		ID:             "envspec-2",
		OwnerID:        "user-2",
		Name:           "py-tools",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `[]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	build := &EnvironmentBuildRecord{
		ID:             "envbuild-1",
		SpecID:         spec.ID,
		Status:         "queued",
		CurrentStep:    "validate_spec",
		LogBlob:        "",
		ImageSizeBytes: 0,
		DigestLocal:    "",
		Error:          "",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create build: %v", err)
	}

	build.Status = "ready"
	build.CurrentStep = "finalize"
	finished := time.Now().UTC()
	build.FinishedAt = &finished
	build.DigestLocal = "sha256:abc123"
	if err := s.UpdateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("update build: %v", err)
	}

	gb, err := s.GetEnvironmentBuild(ctx, build.ID)
	if err != nil {
		t.Fatalf("get build: %v", err)
	}
	if gb.Status != "ready" {
		t.Fatalf("expected ready build status, got %s", gb.Status)
	}

	artifact := &EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "local",
		ImageRef: "local/env:1",
		Digest:   "sha256:def456",
		Status:   "ready",
		Error:    "",
	}
	if err := s.SaveEnvironmentArtifact(ctx, artifact); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	artifacts, err := s.ListEnvironmentArtifacts(ctx, build.ID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	conn := &RegistryConnectionRecord{
		ID:        "reg-1",
		OwnerID:   "user-2",
		Provider:  "ghcr",
		Username:  "octocat",
		SecretRef: "secret://ghcr/user-2/default",
		IsDefault: true,
	}
	if err := s.SaveRegistryConnection(ctx, conn); err != nil {
		t.Fatalf("save registry connection: %v", err)
	}

	conns, err := s.ListRegistryConnections(ctx, "user-2")
	if err != nil {
		t.Fatalf("list registry connections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 registry connection, got %d", len(conns))
	}

	if err := s.DeleteRegistryConnection(ctx, conn.ID); err != nil {
		t.Fatalf("delete registry connection: %v", err)
	}
}

func TestOwnerQuotaStore(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	quota := &OwnerQuotaRecord{
		OwnerID:               "owner-1",
		MaxSandboxes:          3,
		MaxTTLSeconds:         3600,
		MaxExecTimeoutSeconds: 60,
	}
	if err := s.SaveOwnerQuota(ctx, quota); err != nil {
		t.Fatalf("save quota: %v", err)
	}

	got, err := s.GetOwnerQuota(ctx, "owner-1")
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if got.MaxSandboxes != 3 || got.MaxTTLSeconds != 3600 || got.MaxExecTimeoutSeconds != 60 {
		t.Fatalf("unexpected quota: %+v", got)
	}

	quota.MaxSandboxes = 5
	if err := s.SaveOwnerQuota(ctx, quota); err != nil {
		t.Fatalf("update quota: %v", err)
	}

	quotas, err := s.ListOwnerQuotas(ctx)
	if err != nil {
		t.Fatalf("list quotas: %v", err)
	}
	if len(quotas) != 1 || quotas[0].MaxSandboxes != 5 {
		t.Fatalf("unexpected quotas: %+v", quotas)
	}

	if err := s.DeleteOwnerQuota(ctx, "owner-1"); err != nil {
		t.Fatalf("delete quota: %v", err)
	}
	if _, err := s.GetOwnerQuota(ctx, "owner-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
