package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type storeContractOpenFunc func(t *testing.T) Store

func TestSQLiteStoreContract(t *testing.T) {
	runStoreContract(t, "sqlite", func(t *testing.T) Store {
		t.Helper()
		st, err := Open(Config{
			Driver: DriverSQLite,
			Path:   filepath.Join(t.TempDir(), "contract.db"),
		})
		if err != nil {
			t.Fatalf("open sqlite store: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	})
}

func TestPostgresStoreContract(t *testing.T) {
	dsn := os.Getenv("STACYVM_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set STACYVM_POSTGRES_TEST_DSN to run Postgres store contract")
	}
	runStoreContract(t, "postgres", func(t *testing.T) Store {
		t.Helper()
		st, err := Open(Config{Driver: DriverPostgres, DSN: dsn})
		if err != nil {
			t.Fatalf("open postgres store: %v", err)
		}
		if pg, ok := st.(*PostgresStore); ok {
			resetPostgresContractStore(t, pg)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	})
}

func resetPostgresContractStore(t *testing.T, st *PostgresStore) {
	t.Helper()
	_, err := st.db.Exec(`
TRUNCATE
	sandboxes,
	exec_logs,
	provider_configs,
	templates,
	environment_artifacts,
	environment_builds,
	environment_specs,
	registry_connections,
	owner_quotas,
	admin_audit_logs,
	operation_audit_logs,
	workers,
	leases,
	tenants,
	tenant_members,
	policies
RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("reset postgres contract store: %v", err)
	}
}

func runStoreContract(t *testing.T, name string, open storeContractOpenFunc) {
	t.Helper()

	t.Run(name+"/sandboxes", func(t *testing.T) {
		contractSandboxes(t, open(t))
	})
	t.Run(name+"/workers", func(t *testing.T) {
		contractWorkers(t, open(t))
	})
	t.Run(name+"/leases", func(t *testing.T) {
		contractLeases(t, open(t))
	})
	t.Run(name+"/audits_and_logs", func(t *testing.T) {
		contractAuditsAndLogs(t, open(t))
	})
	t.Run(name+"/quotas_and_provider_configs", func(t *testing.T) {
		contractQuotasAndProviderConfigs(t, open(t))
	})
	t.Run(name+"/templates", func(t *testing.T) {
		contractTemplates(t, open(t))
	})
	t.Run(name+"/environments_and_registry", func(t *testing.T) {
		contractEnvironmentsAndRegistry(t, open(t))
	})
	t.Run(name+"/tenants_and_policies", func(t *testing.T) {
		contractTenantsAndPolicies(t, open(t))
	})
}

func contractSandboxes(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	sb := &SandboxRecord{
		ID:        "contract-sb-a",
		State:     "running",
		Provider:  "mock",
		Image:     "alpine:3.20",
		MemoryMB:  256,
		VCPUs:     1,
		Metadata:  `{"contract":true}`,
		OwnerID:   "owner-a",
		VMID:      "vm-a",
		WorkerID:  "worker-a",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
		UpdatedAt: now,
	}
	if err := st.CreateSandbox(ctx, sb); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := st.CreateSandbox(ctx, &SandboxRecord{
		ID:        "contract-sb-b",
		State:     "running",
		Provider:  "mock",
		Image:     "ubuntu:24.04",
		OwnerID:   "owner-b",
		VMID:      "vm-b",
		WorkerID:  "worker-b",
		CreatedAt: now,
		ExpiresAt: now.Add(2 * time.Hour),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create second sandbox: %v", err)
	}

	got, err := st.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get sandbox: %v", err)
	}
	if got.OwnerID != "owner-a" || got.WorkerID != "worker-a" || got.Metadata != `{"contract":true}` {
		t.Fatalf("unexpected sandbox fields: %+v", got)
	}

	byOwner, err := st.ListSandboxesByOwner(ctx, "owner-a")
	if err != nil {
		t.Fatalf("list by owner: %v", err)
	}
	if len(byOwner) != 1 || byOwner[0].ID != sb.ID {
		t.Fatalf("unexpected owner list: %+v", byOwner)
	}

	count, err := st.CountSandboxesByVM(ctx, "vm-a")
	if err != nil {
		t.Fatalf("count by vm: %v", err)
	}
	if count != 1 {
		t.Fatalf("count by vm = %d, want 1", count)
	}

	expiredAt := now.Add(-time.Hour)
	if err := st.UpdateSandboxExpiresAt(ctx, sb.ID, expiredAt); err != nil {
		t.Fatalf("update expires: %v", err)
	}
	expired, err := st.ListExpiredSandboxes(ctx, now)
	if err != nil {
		t.Fatalf("list expired: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != sb.ID {
		t.Fatalf("unexpected expired list: %+v", expired)
	}

	if err := st.UpdateSandboxState(ctx, sb.ID, "destroyed"); err != nil {
		t.Fatalf("update state: %v", err)
	}
	active, err := st.ListSandboxes(ctx)
	if err != nil {
		t.Fatalf("list sandboxes: %v", err)
	}
	if len(active) != 1 || active[0].ID != "contract-sb-b" {
		t.Fatalf("destroyed sandbox should not be listed as active: %+v", active)
	}

	if err := st.DeleteSandbox(ctx, "contract-sb-b"); err != nil {
		t.Fatalf("delete sandbox: %v", err)
	}
	deleted, err := st.GetSandbox(ctx, "contract-sb-b")
	if err != nil {
		t.Fatalf("get soft-deleted sandbox: %v", err)
	}
	if deleted.State != "destroyed" {
		t.Fatalf("deleted sandbox state = %q, want destroyed", deleted.State)
	}
}

func contractWorkers(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	rec := &WorkerRecord{
		ID:            "worker-a",
		Hostname:      "host-a",
		Status:        "online",
		Providers:     `["docker","mock"]`,
		Capabilities:  `["spawn","exec","files"]`,
		Capacity:      `{"max_sandboxes":8}`,
		LastHeartbeat: now,
	}
	if err := st.SaveWorker(ctx, rec); err != nil {
		t.Fatalf("save worker: %v", err)
	}
	if rec.CreatedAt.IsZero() || rec.UpdatedAt.IsZero() {
		t.Fatalf("worker timestamps were not populated: %+v", rec)
	}

	rec.Status = "draining"
	rec.Capacity = `{"max_sandboxes":4}`
	if err := st.SaveWorker(ctx, rec); err != nil {
		t.Fatalf("update worker: %v", err)
	}
	got, err := st.GetWorker(ctx, rec.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if got.Status != "draining" || got.Capacity != `{"max_sandboxes":4}` {
		t.Fatalf("unexpected updated worker: %+v", got)
	}

	workers, err := st.ListWorkers(ctx)
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}
	if len(workers) != 1 || workers[0].ID != rec.ID {
		t.Fatalf("unexpected workers: %+v", workers)
	}

	if err := st.DeleteWorker(ctx, rec.ID); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	if _, err := st.GetWorker(ctx, rec.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted worker error = %v, want ErrNotFound", err)
	}
}

func contractLeases(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()

	lease, err := st.AcquireLease(ctx, "sandbox-a", "sandbox", "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if lease.HolderID != "worker-a" || lease.Generation != 1 {
		t.Fatalf("unexpected lease: %+v", lease)
	}
	if _, err := st.AcquireLease(ctx, "sandbox-a", "sandbox", "worker-b", time.Minute); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting lease error = %v, want ErrConflict", err)
	}

	renewed, err := st.RenewLease(ctx, "sandbox-a", "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if renewed.Generation != lease.Generation+1 {
		t.Fatalf("renewed generation = %d, want %d", renewed.Generation, lease.Generation+1)
	}
	if err := st.ReleaseLease(ctx, "sandbox-a", "worker-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong holder release error = %v, want ErrNotFound", err)
	}

	leases, err := st.ListLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 1 || leases[0].ResourceID != "sandbox-a" {
		t.Fatalf("unexpected leases: %+v", leases)
	}

	if err := st.ReleaseLease(ctx, "sandbox-a", "worker-a"); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	if _, err := st.GetLease(ctx, "sandbox-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get released lease error = %v, want ErrNotFound", err)
	}

	if _, err := st.AcquireLease(ctx, "sandbox-expiring", "sandbox", "worker-a", time.Nanosecond); err != nil {
		t.Fatalf("acquire expiring lease: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	taken, err := st.AcquireLease(ctx, "sandbox-expiring", "sandbox", "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("take over expired lease: %v", err)
	}
	if taken.HolderID != "worker-b" || taken.Generation != 2 {
		t.Fatalf("unexpected takeover lease: %+v", taken)
	}
}

func contractAuditsAndLogs(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	if err := st.CreateSandbox(ctx, &SandboxRecord{
		ID:        "sandbox-a",
		State:     "running",
		Provider:  "mock",
		Image:     "alpine:3.20",
		MemoryMB:  256,
		VCPUs:     1,
		Metadata:  `{}`,
		OwnerID:   "owner-a",
		VMID:      "vm-a",
		WorkerID:  "worker-a",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create sandbox for exec log: %v", err)
	}

	if err := st.CreateExecLog(ctx, &ExecLogRecord{
		SandboxID: "sandbox-a",
		Command:   "echo ok",
		ExitCode:  0,
		Stdout:    "ok\n",
		Duration:  "10ms",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create exec log: %v", err)
	}
	logs, err := st.ListExecLogs(ctx, "sandbox-a")
	if err != nil {
		t.Fatalf("list exec logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Command != "echo ok" || logs[0].ExitCode != 0 {
		t.Fatalf("unexpected exec logs: %+v", logs)
	}

	if err := st.CreateAdminAudit(ctx, &AdminAuditRecord{
		Actor:      "admin",
		Method:     "POST",
		Path:       "/api/v1/admin/quotas/owner-a",
		Status:     201,
		DurationMS: 7,
		RequestID:  "req-a",
		RemoteAddr: "127.0.0.1",
		UserAgent:  "contract",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("create admin audit: %v", err)
	}
	adminLogs, err := st.ListAdminAudit(ctx, AdminAuditQuery{Actor: "admin", Status: 201, Limit: 10})
	if err != nil {
		t.Fatalf("list admin audit: %v", err)
	}
	if len(adminLogs) != 1 || adminLogs[0].RequestID != "req-a" {
		t.Fatalf("unexpected admin audit logs: %+v", adminLogs)
	}
	deleted, err := st.DeleteAdminAuditBefore(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("delete admin audit: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted admin audit rows = %d, want 1", deleted)
	}

	if err := st.CreateOperationAudit(ctx, &OperationAuditRecord{
		Actor:     "owner-a",
		Action:    "sandbox.exec",
		SandboxID: "sandbox-a",
		Resource:  "exec",
		Provider:  "mock",
		Status:    "success",
		Detail:    `{"exit_code":0}`,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation audit: %v", err)
	}
	ops, err := st.ListOperationAudit(ctx, OperationAuditQuery{Actor: "owner-a", Status: "success", Limit: 10})
	if err != nil {
		t.Fatalf("list operation audit: %v", err)
	}
	if len(ops) != 1 || ops[0].Action != "sandbox.exec" {
		t.Fatalf("unexpected operation audit logs: %+v", ops)
	}
}

func contractQuotasAndProviderConfigs(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()

	if err := st.SaveOwnerQuota(ctx, &OwnerQuotaRecord{
		OwnerID:               "owner-a",
		MaxSandboxes:          3,
		MaxTTLSeconds:         3600,
		MaxExecTimeoutSeconds: 120,
	}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	if err := st.SaveOwnerQuota(ctx, &OwnerQuotaRecord{
		OwnerID:               "owner-a",
		MaxSandboxes:          5,
		MaxTTLSeconds:         7200,
		MaxExecTimeoutSeconds: 300,
	}); err != nil {
		t.Fatalf("update quota: %v", err)
	}
	quota, err := st.GetOwnerQuota(ctx, "owner-a")
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if quota.MaxSandboxes != 5 || quota.MaxTTLSeconds != 7200 {
		t.Fatalf("unexpected quota: %+v", quota)
	}
	quotas, err := st.ListOwnerQuotas(ctx)
	if err != nil {
		t.Fatalf("list quotas: %v", err)
	}
	if len(quotas) != 1 || quotas[0].OwnerID != "owner-a" {
		t.Fatalf("unexpected quotas: %+v", quotas)
	}
	if err := st.DeleteOwnerQuota(ctx, "owner-a"); err != nil {
		t.Fatalf("delete quota: %v", err)
	}
	if _, err := st.GetOwnerQuota(ctx, "owner-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted quota error = %v, want ErrNotFound", err)
	}

	if err := st.SaveProviderConfig(ctx, &ProviderConfigRecord{
		Name:    "docker",
		Config:  `{"network":"none"}`,
		Enabled: true,
	}); err != nil {
		t.Fatalf("save provider config: %v", err)
	}
	cfg, err := st.GetProviderConfig(ctx, "docker")
	if err != nil {
		t.Fatalf("get provider config: %v", err)
	}
	if !cfg.Enabled || cfg.Config != `{"network":"none"}` {
		t.Fatalf("unexpected provider config: %+v", cfg)
	}
	configs, err := st.ListProviderConfigs(ctx)
	if err != nil {
		t.Fatalf("list provider configs: %v", err)
	}
	if len(configs) != 1 || configs[0].Name != "docker" {
		t.Fatalf("unexpected provider configs: %+v", configs)
	}
}

func contractTemplates(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	tpl := &TemplateRecord{
		Name:         "go-agent",
		Version:      1,
		Image:        "golang:1.23",
		Description:  "Go agent template",
		Setup:        `["go version"]`,
		AllowedHosts: `["proxy.golang.org"]`,
		MemoryMB:     1024,
		CPUCores:     2,
		TTLSeconds:   1800,
		Env:          `{"GOFLAGS":"-mod=mod"}`,
		Secrets:      `["GITHUB_TOKEN"]`,
		PoolSize:     2,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := st.CreateTemplate(ctx, tpl); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if err := st.CreateTemplate(ctx, tpl); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate template error = %v, want ErrConflict", err)
	}
	tpl.Version = 2
	tpl.PoolSize = 4
	if err := st.UpdateTemplate(ctx, tpl); err != nil {
		t.Fatalf("update template: %v", err)
	}
	got, err := st.GetTemplate(ctx, tpl.Name)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	if got.Version != 2 || got.PoolSize != 4 {
		t.Fatalf("unexpected template: %+v", got)
	}
	templates, err := st.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != tpl.Name {
		t.Fatalf("unexpected templates: %+v", templates)
	}
	if err := st.DeleteTemplate(ctx, tpl.Name); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if _, err := st.GetTemplate(ctx, tpl.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted template error = %v, want ErrNotFound", err)
	}
}

func contractEnvironmentsAndRegistry(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	spec := &EnvironmentSpecRecord{
		ID:             "spec-a",
		OwnerID:        "owner-a",
		Name:           "py-agent",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `["pytest"]`,
		AptPackages:    `["git"]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("create environment spec: %v", err)
	}
	spec.BaseImage = "python:3.12-bookworm"
	if err := st.UpdateEnvironmentSpec(ctx, spec); err != nil {
		t.Fatalf("update environment spec: %v", err)
	}
	gotSpec, err := st.GetEnvironmentSpec(ctx, spec.ID)
	if err != nil {
		t.Fatalf("get environment spec: %v", err)
	}
	if gotSpec.BaseImage != "python:3.12-bookworm" {
		t.Fatalf("unexpected environment spec: %+v", gotSpec)
	}
	specs, err := st.ListEnvironmentSpecs(ctx, "owner-a")
	if err != nil {
		t.Fatalf("list environment specs: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != spec.ID {
		t.Fatalf("unexpected specs: %+v", specs)
	}

	deleteSpec := &EnvironmentSpecRecord{
		ID:             "spec-delete",
		OwnerID:        "owner-a",
		Name:           "delete-me",
		BaseImage:      "python:3.12-slim",
		PythonPackages: `[]`,
		AptPackages:    `[]`,
		PythonVersion:  "3.12",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentSpec(ctx, deleteSpec); err != nil {
		t.Fatalf("create deletable environment spec: %v", err)
	}
	if err := st.DeleteEnvironmentSpec(ctx, deleteSpec.ID); err != nil {
		t.Fatalf("delete environment spec: %v", err)
	}
	if _, err := st.GetEnvironmentSpec(ctx, deleteSpec.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted environment spec error = %v, want ErrNotFound", err)
	}

	finishedAt := now.Add(time.Minute)
	build := &EnvironmentBuildRecord{
		ID:             "build-a",
		SpecID:         spec.ID,
		Status:         "building",
		CurrentStep:    "install",
		LogBlob:        "installing\n",
		ImageSizeBytes: 100,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := st.CreateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("create environment build: %v", err)
	}
	build.Status = "ready"
	build.CurrentStep = "complete"
	build.DigestLocal = "sha256:abc"
	build.FinishedAt = &finishedAt
	if err := st.UpdateEnvironmentBuild(ctx, build); err != nil {
		t.Fatalf("update environment build: %v", err)
	}
	gotBuild, err := st.GetEnvironmentBuild(ctx, build.ID)
	if err != nil {
		t.Fatalf("get environment build: %v", err)
	}
	if gotBuild.Status != "ready" || gotBuild.FinishedAt == nil {
		t.Fatalf("unexpected environment build: %+v", gotBuild)
	}
	builds, err := st.ListEnvironmentBuilds(ctx, spec.ID)
	if err != nil {
		t.Fatalf("list environment builds: %v", err)
	}
	if len(builds) != 1 || builds[0].ID != build.ID {
		t.Fatalf("unexpected builds: %+v", builds)
	}

	if err := st.SaveEnvironmentArtifact(ctx, &EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "linux/amd64",
		ImageRef: "registry.example.com/owner/py-agent:latest",
		Digest:   "sha256:one",
		Status:   "pushed",
	}); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	if err := st.SaveEnvironmentArtifact(ctx, &EnvironmentArtifactRecord{
		BuildID:  build.ID,
		Target:   "linux/amd64",
		ImageRef: "registry.example.com/owner/py-agent:v2",
		Digest:   "sha256:two",
		Status:   "pushed",
	}); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	artifacts, err := st.ListEnvironmentArtifacts(ctx, build.ID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Digest != "sha256:two" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}

	conn := &RegistryConnectionRecord{
		ID:        "registry-a",
		OwnerID:   "owner-a",
		Provider:  "dockerhub",
		Username:  "owner",
		SecretRef: "secret://registry-a",
		IsDefault: true,
	}
	if err := st.SaveRegistryConnection(ctx, conn); err != nil {
		t.Fatalf("save registry connection: %v", err)
	}
	conn.Username = "owner-updated"
	if err := st.SaveRegistryConnection(ctx, conn); err != nil {
		t.Fatalf("update registry connection: %v", err)
	}
	gotConn, err := st.GetRegistryConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("get registry connection: %v", err)
	}
	if gotConn.Username != "owner-updated" || !gotConn.IsDefault {
		t.Fatalf("unexpected registry connection: %+v", gotConn)
	}
	conns, err := st.ListRegistryConnections(ctx, "owner-a")
	if err != nil {
		t.Fatalf("list registry connections: %v", err)
	}
	if len(conns) != 1 || conns[0].ID != conn.ID {
		t.Fatalf("unexpected registry connections: %+v", conns)
	}
	if err := st.DeleteRegistryConnection(ctx, conn.ID); err != nil {
		t.Fatalf("delete registry connection: %v", err)
	}
	if _, err := st.GetRegistryConnection(ctx, conn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted registry connection error = %v, want ErrNotFound", err)
	}

}

func contractTenantsAndPolicies(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()

	// --- Tenants ---
	tenant := &TenantRecord{
		ID:      "tenant-contract-a",
		Name:    "Contract Tenant A",
		OwnerID: "owner-contract",
	}
	if err := st.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	got, err := st.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.Name != "Contract Tenant A" {
		t.Errorf("unexpected tenant name: %q", got.Name)
	}

	got.Name = "Updated Name"
	if err := st.UpdateTenant(ctx, got); err != nil {
		t.Fatalf("update tenant: %v", err)
	}
	got2, _ := st.GetTenant(ctx, tenant.ID)
	if got2.Name != "Updated Name" {
		t.Errorf("tenant name not updated, got %q", got2.Name)
	}

	tenants, err := st.ListTenants(ctx)
	if err != nil {
		t.Fatalf("list tenants: %v", err)
	}
	found := false
	for _, te := range tenants {
		if te.ID == tenant.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tenant not found in list")
	}

	// --- Tenant Members ---
	member := &TenantMemberRecord{
		TenantID: tenant.ID,
		UserID:   "user-contract-1",
		Role:     "operator",
	}
	if err := st.SaveTenantMember(ctx, member); err != nil {
		t.Fatalf("save tenant member: %v", err)
	}

	gotMember, err := st.GetTenantMember(ctx, tenant.ID, member.UserID)
	if err != nil {
		t.Fatalf("get tenant member: %v", err)
	}
	if gotMember.Role != "operator" {
		t.Errorf("unexpected member role: %q", gotMember.Role)
	}

	// Upsert role change.
	member.Role = "admin"
	if err := st.SaveTenantMember(ctx, member); err != nil {
		t.Fatalf("upsert tenant member: %v", err)
	}
	gotMember2, _ := st.GetTenantMember(ctx, tenant.ID, member.UserID)
	if gotMember2.Role != "admin" {
		t.Errorf("member role not updated after upsert, got %q", gotMember2.Role)
	}

	members, err := st.ListTenantMembers(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list tenant members: %v", err)
	}
	if len(members) != 1 || members[0].UserID != member.UserID {
		t.Errorf("unexpected member list: %+v", members)
	}

	if err := st.DeleteTenantMember(ctx, tenant.ID, member.UserID); err != nil {
		t.Fatalf("delete tenant member: %v", err)
	}
	if _, err := st.GetTenantMember(ctx, tenant.ID, member.UserID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted member error = %v, want ErrNotFound", err)
	}

	// --- Policies ---
	pol := &PolicyRecord{
		ID:           "pol-contract-1",
		TenantID:     tenant.ID,
		ResourceType: "image",
		Effect:       "allow",
		Pattern:      "alpine:*",
		Priority:     5,
	}
	if err := st.CreatePolicy(ctx, pol); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	gotPol, err := st.GetPolicy(ctx, pol.ID)
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if gotPol.Pattern != "alpine:*" || gotPol.Effect != "allow" {
		t.Errorf("unexpected policy: %+v", gotPol)
	}

	// Global policy (no tenant).
	globalPol := &PolicyRecord{
		ID:           "pol-contract-global",
		TenantID:     "",
		ResourceType: "image",
		Effect:       "deny",
		Pattern:      "evil:*",
		Priority:     1,
	}
	if err := st.CreatePolicy(ctx, globalPol); err != nil {
		t.Fatalf("create global policy: %v", err)
	}

	// ListPolicies with tenant should include both tenant and global policies.
	pols, err := st.ListPolicies(ctx, PolicyQuery{TenantID: tenant.ID, ResourceType: "image"})
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	if len(pols) < 2 {
		t.Errorf("expected at least 2 policies (tenant + global), got %d", len(pols))
	}

	// ListPolicies without tenant should return only global policies.
	globalPols, err := st.ListPolicies(ctx, PolicyQuery{ResourceType: "image"})
	if err != nil {
		t.Fatalf("list global policies: %v", err)
	}
	for _, p := range globalPols {
		if p.TenantID != "" {
			t.Errorf("expected only global policies, got tenant-scoped: %+v", p)
		}
	}

	// Delete policy.
	if err := st.DeletePolicy(ctx, pol.ID); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	if _, err := st.GetPolicy(ctx, pol.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted policy error = %v, want ErrNotFound", err)
	}
	_ = st.DeletePolicy(ctx, globalPol.ID) // cleanup

	// --- Tenant deletion ---
	if err := st.DeleteTenant(ctx, tenant.ID); err != nil {
		t.Fatalf("delete tenant: %v", err)
	}
	if _, err := st.GetTenant(ctx, tenant.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted tenant error = %v, want ErrNotFound", err)
	}
}
