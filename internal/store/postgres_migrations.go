package store

var postgresMigrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sandboxes (
    id         TEXT PRIMARY KEY,
    state      TEXT NOT NULL DEFAULT 'creating',
    provider   TEXT NOT NULL,
    image      TEXT NOT NULL DEFAULT '',
    memory_mb  INTEGER NOT NULL DEFAULT 512,
    vcpus      INTEGER NOT NULL DEFAULT 1,
    metadata   TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS exec_logs (
    id         BIGSERIAL PRIMARY KEY,
    sandbox_id TEXT NOT NULL REFERENCES sandboxes(id),
    command    TEXT NOT NULL,
    exit_code  INTEGER NOT NULL DEFAULT 0,
    stdout     TEXT NOT NULL DEFAULT '',
    stderr     TEXT NOT NULL DEFAULT '',
    duration   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_exec_logs_sandbox ON exec_logs(sandbox_id);

CREATE TABLE IF NOT EXISTS provider_configs (
    name       TEXT PRIMARY KEY,
    config     TEXT NOT NULL DEFAULT '{}',
    enabled    BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`,
	},
	{
		version: 2,
		sql: `
CREATE TABLE IF NOT EXISTS templates (
    name          TEXT PRIMARY KEY,
    version       INTEGER NOT NULL DEFAULT 1,
    image         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    setup         TEXT NOT NULL DEFAULT '[]',
    allowed_hosts TEXT NOT NULL DEFAULT '[]',
    memory_mb     INTEGER NOT NULL DEFAULT 512,
    cpu_cores     INTEGER NOT NULL DEFAULT 1,
    ttl_seconds   INTEGER NOT NULL DEFAULT 300,
    env           TEXT NOT NULL DEFAULT '{}',
    secrets       TEXT NOT NULL DEFAULT '[]',
    pool_size     INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS template TEXT NOT NULL DEFAULT '';
`,
	},
	{
		version: 3,
		sql: `
CREATE TABLE IF NOT EXISTS environment_specs (
    id               TEXT PRIMARY KEY,
    owner_id         TEXT NOT NULL,
    name             TEXT NOT NULL,
    base_image       TEXT NOT NULL,
    python_packages  TEXT NOT NULL DEFAULT '[]',
    apt_packages     TEXT NOT NULL DEFAULT '[]',
    python_version   TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(owner_id, name)
);

CREATE INDEX IF NOT EXISTS idx_environment_specs_owner ON environment_specs(owner_id);

CREATE TABLE IF NOT EXISTS environment_builds (
    id               TEXT PRIMARY KEY,
    spec_id          TEXT NOT NULL REFERENCES environment_specs(id),
    status           TEXT NOT NULL DEFAULT 'queued',
    current_step     TEXT NOT NULL DEFAULT '',
    log_blob         TEXT NOT NULL DEFAULT '',
    image_size_bytes BIGINT NOT NULL DEFAULT 0,
    digest_local     TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at      TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_environment_builds_spec ON environment_builds(spec_id);
CREATE INDEX IF NOT EXISTS idx_environment_builds_status ON environment_builds(status);

CREATE TABLE IF NOT EXISTS environment_artifacts (
    id         BIGSERIAL PRIMARY KEY,
    build_id   TEXT NOT NULL REFERENCES environment_builds(id),
    target     TEXT NOT NULL,
    image_ref  TEXT NOT NULL,
    digest     TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'pending',
    error      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(build_id, target)
);

CREATE INDEX IF NOT EXISTS idx_environment_artifacts_build ON environment_artifacts(build_id);
CREATE INDEX IF NOT EXISTS idx_environment_artifacts_target ON environment_artifacts(target);

CREATE TABLE IF NOT EXISTS registry_connections (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    provider   TEXT NOT NULL,
    username   TEXT NOT NULL,
    secret_ref TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(owner_id, provider, username)
);

CREATE INDEX IF NOT EXISTS idx_registry_connections_owner ON registry_connections(owner_id);
CREATE INDEX IF NOT EXISTS idx_registry_connections_provider ON registry_connections(provider);
`,
	},
	{
		version: 4,
		sql: `
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS vm_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sandboxes_owner ON sandboxes(owner_id);
CREATE INDEX IF NOT EXISTS idx_sandboxes_vm ON sandboxes(vm_id);
`,
	},
	{
		version: 5,
		sql: `
CREATE TABLE IF NOT EXISTS owner_quotas (
    owner_id                 TEXT PRIMARY KEY,
    max_sandboxes            INTEGER NOT NULL DEFAULT 0,
    max_ttl_seconds          BIGINT NOT NULL DEFAULT 0,
    max_exec_timeout_seconds BIGINT NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
`,
	},
	{
		version: 6,
		sql: `
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    actor       TEXT NOT NULL DEFAULT '',
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status      INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    request_id  TEXT NOT NULL DEFAULT '',
    remote_addr TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created_at ON admin_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_actor ON admin_audit_logs(actor);
`,
	},
	{
		version: 7,
		sql: `
CREATE TABLE IF NOT EXISTS operation_audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    actor      TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    sandbox_id TEXT NOT NULL DEFAULT '',
    resource   TEXT NOT NULL DEFAULT '',
    provider   TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_operation_audit_logs_created_at ON operation_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_audit_logs_actor ON operation_audit_logs(actor);
CREATE INDEX IF NOT EXISTS idx_operation_audit_logs_sandbox ON operation_audit_logs(sandbox_id);
CREATE INDEX IF NOT EXISTS idx_operation_audit_logs_action ON operation_audit_logs(action);
`,
	},
	{
		version: 8,
		sql: `
CREATE TABLE IF NOT EXISTS workers (
    id             TEXT PRIMARY KEY,
    hostname       TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'online',
    providers      TEXT NOT NULL DEFAULT '[]',
    capabilities   TEXT NOT NULL DEFAULT '[]',
    capacity       TEXT NOT NULL DEFAULT '{}',
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);
CREATE INDEX IF NOT EXISTS idx_workers_last_heartbeat ON workers(last_heartbeat DESC);
`,
	},
	{
		version: 9,
		sql: `
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT 'local';
CREATE INDEX IF NOT EXISTS idx_sandboxes_worker ON sandboxes(worker_id);
`,
	},
	{
		version: 10,
		sql: `
CREATE TABLE IF NOT EXISTS leases (
    resource_id   TEXT PRIMARY KEY,
    resource_type TEXT NOT NULL DEFAULT '',
    holder_id     TEXT NOT NULL,
    generation    BIGINT NOT NULL DEFAULT 1,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_leases_holder ON leases(holder_id);
CREATE INDEX IF NOT EXISTS idx_leases_expires_at ON leases(expires_at);
`,
	},
	{
		version: 11,
		sql: `
CREATE TABLE IF NOT EXISTS tenants (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    owner_id   TEXT NOT NULL DEFAULT '',
    settings   TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenants_owner ON tenants(owner_id);

CREATE TABLE IF NOT EXISTS tenant_members (
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_tenant_members_user ON tenant_members(user_id);

CREATE TABLE IF NOT EXISTS policies (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL DEFAULT '',
    resource_type TEXT NOT NULL,
    effect        TEXT NOT NULL DEFAULT 'allow',
    pattern       TEXT NOT NULL,
    priority      INTEGER NOT NULL DEFAULT 10,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_policies_tenant ON policies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_policies_resource ON policies(resource_type);

ALTER TABLE admin_audit_logs ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';
ALTER TABLE operation_audit_logs ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sandboxes_tenant ON sandboxes(tenant_id);
`,
	},
	{
		version: 12,
		sql: `
CREATE TABLE IF NOT EXISTS ssh_keys (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL DEFAULT '',
    tenant_id   TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL UNIQUE,
    public_key  TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ssh_keys_owner ON ssh_keys(owner_id);
`,
	},
}
