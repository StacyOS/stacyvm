package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	// Ensure schema_migrations table exists
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// --- Sandbox CRUD ---

func (s *SQLiteStore) CreateSandbox(ctx context.Context, sb *SandboxRecord) error {
	if strings.TrimSpace(sb.WorkerID) == "" {
		sb.WorkerID = "local"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandboxes (id, state, provider, image, memory_mb, vcpus, metadata, owner_id, vm_id, worker_id, created_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sb.ID, sb.State, sb.Provider, sb.Image, sb.MemoryMB, sb.VCPUs, sb.Metadata,
		sb.OwnerID, sb.VMID, sb.WorkerID,
		sb.CreatedAt.UTC(), sb.ExpiresAt.UTC(), sb.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetSandbox(ctx context.Context, id string) (*SandboxRecord, error) {
	sb := &SandboxRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE id = ?`, id,
	).Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
		&sb.Metadata, &sb.OwnerID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("sandbox", id)
	}
	return sb, err
}

func (s *SQLiteStore) ListSandboxes(ctx context.Context) ([]*SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE state != 'destroyed' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.OwnerID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *SQLiteStore) UpdateSandboxState(ctx context.Context, id string, state string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes SET state = ?, updated_at = ? WHERE id = ?`,
		state, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("sandbox", id)
	}
	return nil
}

func (s *SQLiteStore) UpdateSandboxExpiresAt(ctx context.Context, id string, expiresAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sandboxes SET expires_at = ?, updated_at = ? WHERE id = ? AND state != 'destroyed'`,
		expiresAt.UTC(), time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("sandbox", id)
	}
	return nil
}

func (s *SQLiteStore) DeleteSandbox(ctx context.Context, id string) error {
	return s.UpdateSandboxState(ctx, id, "destroyed")
}

func (s *SQLiteStore) ListExpiredSandboxes(ctx context.Context, before time.Time) ([]*SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE state NOT IN ('destroyed') AND expires_at < ?`,
		before.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.OwnerID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *SQLiteStore) ListSandboxesByOwner(ctx context.Context, ownerID string) ([]*SandboxRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE state != 'destroyed' AND owner_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.OwnerID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *SQLiteStore) CountSandboxesByVM(ctx context.Context, vmID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sandboxes WHERE state != 'destroyed' AND vm_id = ?`, vmID).Scan(&count)
	return count, err
}

func (s *SQLiteStore) SaveOwnerQuota(ctx context.Context, quota *OwnerQuotaRecord) error {
	now := time.Now().UTC()
	if quota.CreatedAt.IsZero() {
		quota.CreatedAt = now
	}
	quota.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO owner_quotas (owner_id, max_sandboxes, max_ttl_seconds, max_exec_timeout_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_id) DO UPDATE SET
			max_sandboxes = excluded.max_sandboxes,
			max_ttl_seconds = excluded.max_ttl_seconds,
			max_exec_timeout_seconds = excluded.max_exec_timeout_seconds,
			updated_at = excluded.updated_at`,
		quota.OwnerID, quota.MaxSandboxes, quota.MaxTTLSeconds, quota.MaxExecTimeoutSeconds,
		quota.CreatedAt.UTC(), quota.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetOwnerQuota(ctx context.Context, ownerID string) (*OwnerQuotaRecord, error) {
	quota := &OwnerQuotaRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_id, max_sandboxes, max_ttl_seconds, max_exec_timeout_seconds, created_at, updated_at
		FROM owner_quotas WHERE owner_id = ?`, ownerID,
	).Scan(&quota.OwnerID, &quota.MaxSandboxes, &quota.MaxTTLSeconds, &quota.MaxExecTimeoutSeconds, &quota.CreatedAt, &quota.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("owner_quota", ownerID)
	}
	return quota, err
}

func (s *SQLiteStore) ListOwnerQuotas(ctx context.Context) ([]*OwnerQuotaRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_id, max_sandboxes, max_ttl_seconds, max_exec_timeout_seconds, created_at, updated_at
		FROM owner_quotas ORDER BY owner_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotas []*OwnerQuotaRecord
	for rows.Next() {
		quota := &OwnerQuotaRecord{}
		if err := rows.Scan(&quota.OwnerID, &quota.MaxSandboxes, &quota.MaxTTLSeconds,
			&quota.MaxExecTimeoutSeconds, &quota.CreatedAt, &quota.UpdatedAt); err != nil {
			return nil, err
		}
		quotas = append(quotas, quota)
	}
	return quotas, rows.Err()
}

func (s *SQLiteStore) DeleteOwnerQuota(ctx context.Context, ownerID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM owner_quotas WHERE owner_id = ?`, ownerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("owner_quota", ownerID)
	}
	return nil
}

func (s *SQLiteStore) CreateAdminAudit(ctx context.Context, rec *AdminAuditRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_audit_logs (actor, method, path, status, duration_ms, request_id, remote_addr, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Actor, rec.Method, rec.Path, rec.Status, rec.DurationMS, rec.RequestID,
		rec.RemoteAddr, rec.UserAgent, rec.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteStore) ListAdminAudit(ctx context.Context, query AdminAuditQuery) ([]*AdminAuditRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}

	clauses := []string{"1=1"}
	args := make([]interface{}, 0, 5)
	if query.Actor != "" {
		clauses = append(clauses, "actor = ?")
		args = append(args, query.Actor)
	}
	if query.Method != "" {
		clauses = append(clauses, "method = ?")
		args = append(args, query.Method)
	}
	if query.Status > 0 {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if query.PathLike != "" {
		clauses = append(clauses, "path LIKE ?")
		args = append(args, "%"+query.PathLike+"%")
	}
	args = append(args, query.Limit)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor, method, path, status, duration_ms, request_id, remote_addr, user_agent, created_at
		FROM admin_audit_logs WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*AdminAuditRecord
	for rows.Next() {
		rec := &AdminAuditRecord{}
		if err := rows.Scan(&rec.ID, &rec.Actor, &rec.Method, &rec.Path, &rec.Status,
			&rec.DurationMS, &rec.RequestID, &rec.RemoteAddr, &rec.UserAgent, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) DeleteAdminAuditBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM admin_audit_logs WHERE created_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) CreateOperationAudit(ctx context.Context, rec *OperationAuditRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO operation_audit_logs (actor, action, sandbox_id, resource, provider, status, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Actor, rec.Action, rec.SandboxID, rec.Resource, rec.Provider, rec.Status, rec.Detail, rec.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLiteStore) ListOperationAudit(ctx context.Context, query OperationAuditQuery) ([]*OperationAuditRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}

	clauses := []string{"1=1"}
	args := make([]interface{}, 0, 7)
	if query.Actor != "" {
		clauses = append(clauses, "actor = ?")
		args = append(args, query.Actor)
	}
	if query.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, query.Action)
	}
	if query.SandboxID != "" {
		clauses = append(clauses, "sandbox_id = ?")
		args = append(args, query.SandboxID)
	}
	if query.Resource != "" {
		clauses = append(clauses, "resource = ?")
		args = append(args, query.Resource)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	args = append(args, query.Limit)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor, action, sandbox_id, resource, provider, status, detail, created_at
		FROM operation_audit_logs WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*OperationAuditRecord
	for rows.Next() {
		rec := &OperationAuditRecord{}
		if err := rows.Scan(&rec.ID, &rec.Actor, &rec.Action, &rec.SandboxID, &rec.Resource,
			&rec.Provider, &rec.Status, &rec.Detail, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// --- Workers ---

func (s *SQLiteStore) SaveWorker(ctx context.Context, rec *WorkerRecord) error {
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.LastHeartbeat.IsZero() {
		rec.LastHeartbeat = now
	}
	rec.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workers (id, hostname, status, providers, capabilities, capacity, last_heartbeat, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			status = excluded.status,
			providers = excluded.providers,
			capabilities = excluded.capabilities,
			capacity = excluded.capacity,
			last_heartbeat = excluded.last_heartbeat,
			updated_at = excluded.updated_at`,
		rec.ID, rec.Hostname, rec.Status, rec.Providers, rec.Capabilities, rec.Capacity,
		rec.LastHeartbeat.UTC(), rec.CreatedAt.UTC(), rec.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetWorker(ctx context.Context, id string) (*WorkerRecord, error) {
	rec := &WorkerRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, hostname, status, providers, capabilities, capacity, last_heartbeat, created_at, updated_at
		FROM workers WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.Hostname, &rec.Status, &rec.Providers, &rec.Capabilities, &rec.Capacity,
		&rec.LastHeartbeat, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("worker", id)
	}
	return rec, err
}

func (s *SQLiteStore) ListWorkers(ctx context.Context) ([]*WorkerRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, hostname, status, providers, capabilities, capacity, last_heartbeat, created_at, updated_at
		FROM workers ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*WorkerRecord
	for rows.Next() {
		rec := &WorkerRecord{}
		if err := rows.Scan(&rec.ID, &rec.Hostname, &rec.Status, &rec.Providers, &rec.Capabilities,
			&rec.Capacity, &rec.LastHeartbeat, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) DeleteWorker(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("worker", id)
	}
	return nil
}

// --- Exec Logs ---

func (s *SQLiteStore) CreateExecLog(ctx context.Context, log *ExecLogRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO exec_logs (sandbox_id, command, exit_code, stdout, stderr, duration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.SandboxID, log.Command, log.ExitCode, log.Stdout, log.Stderr, log.Duration, log.CreatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) ListExecLogs(ctx context.Context, sandboxID string) ([]*ExecLogRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, sandbox_id, command, exit_code, stdout, stderr, duration, created_at
		FROM exec_logs WHERE sandbox_id = ? ORDER BY created_at DESC`, sandboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*ExecLogRecord
	for rows.Next() {
		l := &ExecLogRecord{}
		if err := rows.Scan(&l.ID, &l.SandboxID, &l.Command, &l.ExitCode, &l.Stdout, &l.Stderr,
			&l.Duration, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// --- Provider Configs ---

func (s *SQLiteStore) GetProviderConfig(ctx context.Context, name string) (*ProviderConfigRecord, error) {
	cfg := &ProviderConfigRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, config, enabled, updated_at FROM provider_configs WHERE name = ?`, name,
	).Scan(&cfg.Name, &cfg.Config, &cfg.Enabled, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("provider config", name)
	}
	return cfg, err
}

func (s *SQLiteStore) SaveProviderConfig(ctx context.Context, cfg *ProviderConfigRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_configs (name, config, enabled, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET config = excluded.config, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		cfg.Name, cfg.Config, cfg.Enabled, time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) ListProviderConfigs(ctx context.Context) ([]*ProviderConfigRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, config, enabled, updated_at FROM provider_configs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*ProviderConfigRecord
	for rows.Next() {
		cfg := &ProviderConfigRecord{}
		if err := rows.Scan(&cfg.Name, &cfg.Config, &cfg.Enabled, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// --- Templates ---

func (s *SQLiteStore) CreateTemplate(ctx context.Context, t *TemplateRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO templates (name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Version, t.Image, t.Description, t.Setup, t.AllowedHosts,
		t.MemoryMB, t.CPUCores, t.TTLSeconds, t.Env, t.Secrets, t.PoolSize,
		t.CreatedAt.UTC(), t.UpdatedAt.UTC(),
	)
	if IsConstraintError(err) {
		return ConflictError("template already exists")
	}
	return err
}

func (s *SQLiteStore) GetTemplate(ctx context.Context, name string) (*TemplateRecord, error) {
	t := &TemplateRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at
		FROM templates WHERE name = ?`, name,
	).Scan(&t.Name, &t.Version, &t.Image, &t.Description, &t.Setup, &t.AllowedHosts,
		&t.MemoryMB, &t.CPUCores, &t.TTLSeconds, &t.Env, &t.Secrets, &t.PoolSize,
		&t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("template", name)
	}
	return t, err
}

func (s *SQLiteStore) ListTemplates(ctx context.Context) ([]*TemplateRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, version, image, description, setup, allowed_hosts, memory_mb, cpu_cores, ttl_seconds, env, secrets, pool_size, created_at, updated_at
		FROM templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*TemplateRecord
	for rows.Next() {
		t := &TemplateRecord{}
		if err := rows.Scan(&t.Name, &t.Version, &t.Image, &t.Description, &t.Setup, &t.AllowedHosts,
			&t.MemoryMB, &t.CPUCores, &t.TTLSeconds, &t.Env, &t.Secrets, &t.PoolSize,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (s *SQLiteStore) UpdateTemplate(ctx context.Context, t *TemplateRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE templates SET version = ?, image = ?, description = ?, setup = ?, allowed_hosts = ?,
		memory_mb = ?, cpu_cores = ?, ttl_seconds = ?, env = ?, secrets = ?, pool_size = ?, updated_at = ?
		WHERE name = ?`,
		t.Version, t.Image, t.Description, t.Setup, t.AllowedHosts,
		t.MemoryMB, t.CPUCores, t.TTLSeconds, t.Env, t.Secrets, t.PoolSize,
		time.Now().UTC(), t.Name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("template", t.Name)
	}
	return nil
}

func (s *SQLiteStore) DeleteTemplate(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("template", name)
	}
	return nil
}

// --- Environment Specs ---

func (s *SQLiteStore) CreateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_specs (id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.ID, spec.OwnerID, spec.Name, spec.BaseImage, spec.PythonPackages, spec.AptPackages, spec.PythonVersion,
		spec.CreatedAt.UTC(), spec.UpdatedAt.UTC(),
	)
	if IsConstraintError(err) {
		return ConflictError("spec name already exists for this owner")
	}
	return err
}

func (s *SQLiteStore) GetEnvironmentSpec(ctx context.Context, id string) (*EnvironmentSpecRecord, error) {
	spec := &EnvironmentSpecRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at
		FROM environment_specs WHERE id = ?`, id,
	).Scan(
		&spec.ID, &spec.OwnerID, &spec.Name, &spec.BaseImage, &spec.PythonPackages, &spec.AptPackages,
		&spec.PythonVersion, &spec.CreatedAt, &spec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("environment spec", id)
	}
	return spec, err
}

func (s *SQLiteStore) ListEnvironmentSpecs(ctx context.Context, ownerID string) ([]*EnvironmentSpecRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, name, base_image, python_packages, apt_packages, python_version, created_at, updated_at
		FROM environment_specs
		WHERE owner_id = ?
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []*EnvironmentSpecRecord
	for rows.Next() {
		spec := &EnvironmentSpecRecord{}
		if err := rows.Scan(
			&spec.ID, &spec.OwnerID, &spec.Name, &spec.BaseImage, &spec.PythonPackages, &spec.AptPackages,
			&spec.PythonVersion, &spec.CreatedAt, &spec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

func (s *SQLiteStore) UpdateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE environment_specs
		SET name = ?, base_image = ?, python_packages = ?, apt_packages = ?, python_version = ?, updated_at = ?
		WHERE id = ?`,
		spec.Name, spec.BaseImage, spec.PythonPackages, spec.AptPackages, spec.PythonVersion,
		time.Now().UTC(), spec.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("environment spec", spec.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteEnvironmentSpec(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM environment_specs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("environment spec", id)
	}
	return nil
}

// --- Environment Builds ---

func (s *SQLiteStore) CreateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_builds (
			id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		build.ID, build.SpecID, build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes,
		build.DigestLocal, build.Error, build.CreatedAt.UTC(), nullableTime(build.FinishedAt), build.UpdatedAt.UTC(),
	)
	return err
}

func (s *SQLiteStore) GetEnvironmentBuild(ctx context.Context, id string) (*EnvironmentBuildRecord, error) {
	build := &EnvironmentBuildRecord{}
	var finishedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		FROM environment_builds WHERE id = ?`, id,
	).Scan(
		&build.ID, &build.SpecID, &build.Status, &build.CurrentStep, &build.LogBlob, &build.ImageSizeBytes,
		&build.DigestLocal, &build.Error, &build.CreatedAt, &finishedAt, &build.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("environment build", id)
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		build.FinishedAt = &t
	}
	return build, nil
}

func (s *SQLiteStore) ListEnvironmentBuilds(ctx context.Context, specID string) ([]*EnvironmentBuildRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		FROM environment_builds
		WHERE spec_id = ?
		ORDER BY created_at DESC`, specID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*EnvironmentBuildRecord
	for rows.Next() {
		build := &EnvironmentBuildRecord{}
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&build.ID, &build.SpecID, &build.Status, &build.CurrentStep, &build.LogBlob, &build.ImageSizeBytes,
			&build.DigestLocal, &build.Error, &build.CreatedAt, &finishedAt, &build.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			build.FinishedAt = &t
		}
		builds = append(builds, build)
	}
	return builds, rows.Err()
}

func (s *SQLiteStore) UpdateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE environment_builds
		SET status = ?, current_step = ?, log_blob = ?, image_size_bytes = ?, digest_local = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ?`,
		build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes, build.DigestLocal, build.Error,
		nullableTime(build.FinishedAt), time.Now().UTC(), build.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("environment build", build.ID)
	}
	return nil
}

// --- Environment Artifacts ---

func (s *SQLiteStore) SaveEnvironmentArtifact(ctx context.Context, artifact *EnvironmentArtifactRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environment_artifacts (build_id, target, image_ref, digest, status, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(build_id, target) DO UPDATE SET
			image_ref = excluded.image_ref,
			digest = excluded.digest,
			status = excluded.status,
			error = excluded.error,
			updated_at = excluded.updated_at`,
		artifact.BuildID, artifact.Target, artifact.ImageRef, artifact.Digest, artifact.Status, artifact.Error,
		time.Now().UTC(), time.Now().UTC(),
	)
	return err
}

func (s *SQLiteStore) ListEnvironmentArtifacts(ctx context.Context, buildID string) ([]*EnvironmentArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, build_id, target, image_ref, digest, status, error, created_at, updated_at
		FROM environment_artifacts
		WHERE build_id = ?
		ORDER BY id ASC`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*EnvironmentArtifactRecord
	for rows.Next() {
		artifact := &EnvironmentArtifactRecord{}
		if err := rows.Scan(
			&artifact.ID, &artifact.BuildID, &artifact.Target, &artifact.ImageRef, &artifact.Digest,
			&artifact.Status, &artifact.Error, &artifact.CreatedAt, &artifact.UpdatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

// --- Registry Connections ---

func (s *SQLiteStore) SaveRegistryConnection(ctx context.Context, conn *RegistryConnectionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registry_connections (id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			owner_id = excluded.owner_id,
			provider = excluded.provider,
			username = excluded.username,
			secret_ref = excluded.secret_ref,
			is_default = excluded.is_default,
			updated_at = excluded.updated_at`,
		conn.ID, conn.OwnerID, conn.Provider, conn.Username, conn.SecretRef, conn.IsDefault,
		time.Now().UTC(), time.Now().UTC(),
	)
	if IsConstraintError(err) {
		return ConflictError("registry connection already exists")
	}
	return err
}

func (s *SQLiteStore) GetRegistryConnection(ctx context.Context, id string) (*RegistryConnectionRecord, error) {
	conn := &RegistryConnectionRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at
		FROM registry_connections
		WHERE id = ?`, id,
	).Scan(&conn.ID, &conn.OwnerID, &conn.Provider, &conn.Username, &conn.SecretRef, &conn.IsDefault, &conn.CreatedAt, &conn.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("registry connection", id)
	}
	return conn, err
}

func (s *SQLiteStore) ListRegistryConnections(ctx context.Context, ownerID string) ([]*RegistryConnectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at
		FROM registry_connections
		WHERE owner_id = ?
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []*RegistryConnectionRecord
	for rows.Next() {
		conn := &RegistryConnectionRecord{}
		if err := rows.Scan(
			&conn.ID, &conn.OwnerID, &conn.Provider, &conn.Username, &conn.SecretRef, &conn.IsDefault, &conn.CreatedAt, &conn.UpdatedAt,
		); err != nil {
			return nil, err
		}
		conns = append(conns, conn)
	}
	return conns, rows.Err()
}

func (s *SQLiteStore) DeleteRegistryConnection(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registry_connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("registry connection", id)
	}
	return nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
