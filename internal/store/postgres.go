package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	s := &PostgresStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *PostgresStore) migrate() error {
	// Ensure schema_migrations table exists
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return err
	}

	for _, m := range postgresMigrations {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", m.version).Scan(&count)
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
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func (s *PostgresStore) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, rebindPostgres(query), args...)
}

func (s *PostgresStore) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, rebindPostgres(query), args...)
}

func (s *PostgresStore) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, rebindPostgres(query), args...)
}

func rebindPostgres(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)
	arg := 1
	for _, r := range query {
		if r == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(arg))
			arg++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// --- Sandbox CRUD ---

func (s *PostgresStore) CreateSandbox(ctx context.Context, sb *SandboxRecord) error {
	if strings.TrimSpace(sb.WorkerID) == "" {
		sb.WorkerID = "local"
	}
	_, err := s.execContext(ctx, `
		INSERT INTO sandboxes (id, state, provider, image, memory_mb, vcpus, metadata, owner_id, tenant_id, vm_id, worker_id, created_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sb.ID, sb.State, sb.Provider, sb.Image, sb.MemoryMB, sb.VCPUs, sb.Metadata,
		sb.OwnerID, sb.TenantID, sb.VMID, sb.WorkerID,
		sb.CreatedAt.UTC(), sb.ExpiresAt.UTC(), sb.UpdatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) GetSandbox(ctx context.Context, id string) (*SandboxRecord, error) {
	sb := &SandboxRecord{}
	err := s.queryRowContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, tenant_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE id = ?`, id,
	).Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
		&sb.Metadata, &sb.OwnerID, &sb.TenantID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("sandbox", id)
	}
	return sb, err
}

func (s *PostgresStore) ListSandboxes(ctx context.Context) ([]*SandboxRecord, error) {
	rows, err := s.queryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, tenant_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE state != 'destroyed' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.OwnerID, &sb.TenantID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *PostgresStore) UpdateSandboxState(ctx context.Context, id string, state string) error {
	res, err := s.execContext(ctx, `
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

func (s *PostgresStore) UpdateSandboxExpiresAt(ctx context.Context, id string, expiresAt time.Time) error {
	res, err := s.execContext(ctx, `
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

func (s *PostgresStore) DeleteSandbox(ctx context.Context, id string) error {
	return s.UpdateSandboxState(ctx, id, "destroyed")
}

func (s *PostgresStore) ListExpiredSandboxes(ctx context.Context, before time.Time) ([]*SandboxRecord, error) {
	rows, err := s.queryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, tenant_id, vm_id, worker_id, created_at, expires_at, updated_at
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
			&sb.Metadata, &sb.OwnerID, &sb.TenantID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *PostgresStore) ListSandboxesByOwner(ctx context.Context, ownerID string) ([]*SandboxRecord, error) {
	rows, err := s.queryContext(ctx, `
		SELECT id, state, provider, image, memory_mb, vcpus, metadata, owner_id, tenant_id, vm_id, worker_id, created_at, expires_at, updated_at
		FROM sandboxes WHERE state != 'destroyed' AND owner_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sandboxes []*SandboxRecord
	for rows.Next() {
		sb := &SandboxRecord{}
		if err := rows.Scan(&sb.ID, &sb.State, &sb.Provider, &sb.Image, &sb.MemoryMB, &sb.VCPUs,
			&sb.Metadata, &sb.OwnerID, &sb.TenantID, &sb.VMID, &sb.WorkerID, &sb.CreatedAt, &sb.ExpiresAt, &sb.UpdatedAt); err != nil {
			return nil, err
		}
		sandboxes = append(sandboxes, sb)
	}
	return sandboxes, rows.Err()
}

func (s *PostgresStore) CountSandboxesByVM(ctx context.Context, vmID string) (int, error) {
	var count int
	err := s.queryRowContext(ctx, `
		SELECT COUNT(*) FROM sandboxes WHERE state != 'destroyed' AND vm_id = ?`, vmID).Scan(&count)
	return count, err
}

func (s *PostgresStore) SaveOwnerQuota(ctx context.Context, quota *OwnerQuotaRecord) error {
	now := time.Now().UTC()
	if quota.CreatedAt.IsZero() {
		quota.CreatedAt = now
	}
	quota.UpdatedAt = now
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) GetOwnerQuota(ctx context.Context, ownerID string) (*OwnerQuotaRecord, error) {
	quota := &OwnerQuotaRecord{}
	err := s.queryRowContext(ctx, `
		SELECT owner_id, max_sandboxes, max_ttl_seconds, max_exec_timeout_seconds, created_at, updated_at
		FROM owner_quotas WHERE owner_id = ?`, ownerID,
	).Scan(&quota.OwnerID, &quota.MaxSandboxes, &quota.MaxTTLSeconds, &quota.MaxExecTimeoutSeconds, &quota.CreatedAt, &quota.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("owner_quota", ownerID)
	}
	return quota, err
}

func (s *PostgresStore) ListOwnerQuotas(ctx context.Context) ([]*OwnerQuotaRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) DeleteOwnerQuota(ctx context.Context, ownerID string) error {
	res, err := s.execContext(ctx, `DELETE FROM owner_quotas WHERE owner_id = ?`, ownerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("owner_quota", ownerID)
	}
	return nil
}

func (s *PostgresStore) CreateAdminAudit(ctx context.Context, rec *AdminAuditRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	res, err := s.execContext(ctx, `
		INSERT INTO admin_audit_logs (actor, method, path, status, duration_ms, request_id, remote_addr, user_agent, tenant_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Actor, rec.Method, rec.Path, rec.Status, rec.DurationMS, rec.RequestID,
		rec.RemoteAddr, rec.UserAgent, rec.TenantID, rec.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

func (s *PostgresStore) ListAdminAudit(ctx context.Context, query AdminAuditQuery) ([]*AdminAuditRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}

	clauses := []string{"1=1"}
	args := make([]interface{}, 0, 6)
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
	if query.TenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, query.TenantID)
	}
	args = append(args, query.Limit)

	rows, err := s.queryContext(ctx, `
		SELECT id, actor, method, path, status, duration_ms, request_id, remote_addr, user_agent, tenant_id, created_at
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
			&rec.DurationMS, &rec.RequestID, &rec.RemoteAddr, &rec.UserAgent, &rec.TenantID, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *PostgresStore) DeleteAdminAuditBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.execContext(ctx, `DELETE FROM admin_audit_logs WHERE created_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *PostgresStore) CreateOperationAudit(ctx context.Context, rec *OperationAuditRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	res, err := s.execContext(ctx, `
		INSERT INTO operation_audit_logs (actor, action, sandbox_id, resource, provider, status, detail, tenant_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.Actor, rec.Action, rec.SandboxID, rec.Resource, rec.Provider, rec.Status, rec.Detail, rec.TenantID, rec.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

func (s *PostgresStore) ListOperationAudit(ctx context.Context, query OperationAuditQuery) ([]*OperationAuditRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 500 {
		query.Limit = 500
	}

	clauses := []string{"1=1"}
	args := make([]interface{}, 0, 8)
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
	if query.TenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, query.TenantID)
	}
	args = append(args, query.Limit)

	rows, err := s.queryContext(ctx, `
		SELECT id, actor, action, sandbox_id, resource, provider, status, detail, tenant_id, created_at
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
			&rec.Provider, &rec.Status, &rec.Detail, &rec.TenantID, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// --- Workers ---

func (s *PostgresStore) SaveWorker(ctx context.Context, rec *WorkerRecord) error {
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.LastHeartbeat.IsZero() {
		rec.LastHeartbeat = now
	}
	rec.UpdatedAt = now
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) GetWorker(ctx context.Context, id string) (*WorkerRecord, error) {
	rec := &WorkerRecord{}
	err := s.queryRowContext(ctx, `
		SELECT id, hostname, status, providers, capabilities, capacity, last_heartbeat, created_at, updated_at
		FROM workers WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.Hostname, &rec.Status, &rec.Providers, &rec.Capabilities, &rec.Capacity,
		&rec.LastHeartbeat, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("worker", id)
	}
	return rec, err
}

func (s *PostgresStore) ListWorkers(ctx context.Context) ([]*WorkerRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) DeleteWorker(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM workers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("worker", id)
	}
	return nil
}

// --- Leases ---

func (s *PostgresStore) AcquireLease(ctx context.Context, resourceID, resourceType, holderID string, ttl time.Duration) (*LeaseRecord, error) {
	if strings.TrimSpace(resourceID) == "" {
		return nil, ConflictError("lease resource id is required")
	}
	if strings.TrimSpace(holderID) == "" {
		return nil, ConflictError("lease holder id is required")
	}
	if ttl <= 0 {
		return nil, ConflictError("lease ttl must be positive")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	res, err := s.execContext(ctx, `
		INSERT INTO leases (resource_id, resource_type, holder_id, generation, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(resource_id) DO UPDATE SET
			resource_type = excluded.resource_type,
			holder_id = excluded.holder_id,
			generation = leases.generation + 1,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at
		WHERE leases.expires_at <= ? OR leases.holder_id = excluded.holder_id`,
		resourceID, resourceType, holderID, expiresAt, now, now, now,
	)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ConflictError("lease is held by another worker")
	}
	return s.GetLease(ctx, resourceID)
}

func (s *PostgresStore) RenewLease(ctx context.Context, resourceID, holderID string, ttl time.Duration) (*LeaseRecord, error) {
	if ttl <= 0 {
		return nil, ConflictError("lease ttl must be positive")
	}
	now := time.Now().UTC()
	res, err := s.execContext(ctx, `
		UPDATE leases
		SET generation = generation + 1, expires_at = ?, updated_at = ?
		WHERE resource_id = ? AND holder_id = ? AND expires_at > ?`,
		now.Add(ttl), now, resourceID, holderID, now,
	)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ConflictError("lease is not held by worker or has expired")
	}
	return s.GetLease(ctx, resourceID)
}

func (s *PostgresStore) GetLease(ctx context.Context, resourceID string) (*LeaseRecord, error) {
	rec := &LeaseRecord{}
	err := s.queryRowContext(ctx, `
		SELECT resource_id, resource_type, holder_id, generation, expires_at, created_at, updated_at
		FROM leases WHERE resource_id = ?`, resourceID,
	).Scan(&rec.ResourceID, &rec.ResourceType, &rec.HolderID, &rec.Generation, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("lease", resourceID)
	}
	return rec, err
}

func (s *PostgresStore) ListLeases(ctx context.Context) ([]*LeaseRecord, error) {
	rows, err := s.queryContext(ctx, `
		SELECT resource_id, resource_type, holder_id, generation, expires_at, created_at, updated_at
		FROM leases ORDER BY resource_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*LeaseRecord
	for rows.Next() {
		rec := &LeaseRecord{}
		if err := rows.Scan(&rec.ResourceID, &rec.ResourceType, &rec.HolderID, &rec.Generation, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *PostgresStore) ReleaseLease(ctx context.Context, resourceID, holderID string) error {
	res, err := s.execContext(ctx, `DELETE FROM leases WHERE resource_id = ? AND holder_id = ?`, resourceID, holderID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return NotFoundError("lease", resourceID)
	}
	return nil
}

// --- Exec Logs ---

func (s *PostgresStore) CreateExecLog(ctx context.Context, log *ExecLogRecord) error {
	_, err := s.execContext(ctx, `
		INSERT INTO exec_logs (sandbox_id, command, exit_code, stdout, stderr, duration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.SandboxID, log.Command, log.ExitCode, log.Stdout, log.Stderr, log.Duration, log.CreatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) ListExecLogs(ctx context.Context, sandboxID string) ([]*ExecLogRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) GetProviderConfig(ctx context.Context, name string) (*ProviderConfigRecord, error) {
	cfg := &ProviderConfigRecord{}
	err := s.queryRowContext(ctx, `
		SELECT name, config, enabled, updated_at FROM provider_configs WHERE name = ?`, name,
	).Scan(&cfg.Name, &cfg.Config, &cfg.Enabled, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("provider config", name)
	}
	return cfg, err
}

func (s *PostgresStore) SaveProviderConfig(ctx context.Context, cfg *ProviderConfigRecord) error {
	_, err := s.execContext(ctx, `
		INSERT INTO provider_configs (name, config, enabled, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET config = excluded.config, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		cfg.Name, cfg.Config, cfg.Enabled, time.Now().UTC(),
	)
	return err
}

func (s *PostgresStore) ListProviderConfigs(ctx context.Context) ([]*ProviderConfigRecord, error) {
	rows, err := s.queryContext(ctx, `SELECT name, config, enabled, updated_at FROM provider_configs`)
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

func (s *PostgresStore) CreateTemplate(ctx context.Context, t *TemplateRecord) error {
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) GetTemplate(ctx context.Context, name string) (*TemplateRecord, error) {
	t := &TemplateRecord{}
	err := s.queryRowContext(ctx, `
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

func (s *PostgresStore) ListTemplates(ctx context.Context) ([]*TemplateRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) UpdateTemplate(ctx context.Context, t *TemplateRecord) error {
	res, err := s.execContext(ctx, `
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

func (s *PostgresStore) DeleteTemplate(ctx context.Context, name string) error {
	res, err := s.execContext(ctx, `DELETE FROM templates WHERE name = ?`, name)
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

func (s *PostgresStore) CreateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) GetEnvironmentSpec(ctx context.Context, id string) (*EnvironmentSpecRecord, error) {
	spec := &EnvironmentSpecRecord{}
	err := s.queryRowContext(ctx, `
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

func (s *PostgresStore) ListEnvironmentSpecs(ctx context.Context, ownerID string) ([]*EnvironmentSpecRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) UpdateEnvironmentSpec(ctx context.Context, spec *EnvironmentSpecRecord) error {
	res, err := s.execContext(ctx, `
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

func (s *PostgresStore) DeleteEnvironmentSpec(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM environment_specs WHERE id = ?`, id)
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

func (s *PostgresStore) CreateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	_, err := s.execContext(ctx, `
		INSERT INTO environment_builds (
			id, spec_id, status, current_step, log_blob, image_size_bytes, digest_local, error, created_at, finished_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		build.ID, build.SpecID, build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes,
		build.DigestLocal, build.Error, build.CreatedAt.UTC(), postgresNullableTime(build.FinishedAt), build.UpdatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) GetEnvironmentBuild(ctx context.Context, id string) (*EnvironmentBuildRecord, error) {
	build := &EnvironmentBuildRecord{}
	var finishedAt sql.NullTime

	err := s.queryRowContext(ctx, `
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

func (s *PostgresStore) ListEnvironmentBuilds(ctx context.Context, specID string) ([]*EnvironmentBuildRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) UpdateEnvironmentBuild(ctx context.Context, build *EnvironmentBuildRecord) error {
	res, err := s.execContext(ctx, `
		UPDATE environment_builds
		SET status = ?, current_step = ?, log_blob = ?, image_size_bytes = ?, digest_local = ?, error = ?, finished_at = ?, updated_at = ?
		WHERE id = ?`,
		build.Status, build.CurrentStep, build.LogBlob, build.ImageSizeBytes, build.DigestLocal, build.Error,
		postgresNullableTime(build.FinishedAt), time.Now().UTC(), build.ID,
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

func (s *PostgresStore) SaveEnvironmentArtifact(ctx context.Context, artifact *EnvironmentArtifactRecord) error {
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) ListEnvironmentArtifacts(ctx context.Context, buildID string) ([]*EnvironmentArtifactRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) SaveRegistryConnection(ctx context.Context, conn *RegistryConnectionRecord) error {
	_, err := s.execContext(ctx, `
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

func (s *PostgresStore) GetRegistryConnection(ctx context.Context, id string) (*RegistryConnectionRecord, error) {
	conn := &RegistryConnectionRecord{}
	err := s.queryRowContext(ctx, `
		SELECT id, owner_id, provider, username, secret_ref, is_default, created_at, updated_at
		FROM registry_connections
		WHERE id = ?`, id,
	).Scan(&conn.ID, &conn.OwnerID, &conn.Provider, &conn.Username, &conn.SecretRef, &conn.IsDefault, &conn.CreatedAt, &conn.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("registry connection", id)
	}
	return conn, err
}

func (s *PostgresStore) ListRegistryConnections(ctx context.Context, ownerID string) ([]*RegistryConnectionRecord, error) {
	rows, err := s.queryContext(ctx, `
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

func (s *PostgresStore) DeleteRegistryConnection(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM registry_connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("registry connection", id)
	}
	return nil
}

func postgresNullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// --- Tenants ---

func (s *PostgresStore) CreateTenant(ctx context.Context, t *TenantRecord) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Settings == "" {
		t.Settings = "{}"
	}
	_, err := s.execContext(ctx,
		`INSERT INTO tenants (id, name, owner_id, settings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.OwnerID, t.Settings, t.CreatedAt.UTC(), t.UpdatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) GetTenant(ctx context.Context, id string) (*TenantRecord, error) {
	t := &TenantRecord{}
	err := s.queryRowContext(ctx,
		`SELECT id, name, owner_id, settings, created_at, updated_at FROM tenants WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.OwnerID, &t.Settings, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("tenant", id)
	}
	return t, err
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]*TenantRecord, error) {
	rows, err := s.queryContext(ctx,
		`SELECT id, name, owner_id, settings, created_at, updated_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenants []*TenantRecord
	for rows.Next() {
		t := &TenantRecord{}
		if err := rows.Scan(&t.ID, &t.Name, &t.OwnerID, &t.Settings, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *PostgresStore) UpdateTenant(ctx context.Context, t *TenantRecord) error {
	t.UpdatedAt = time.Now().UTC()
	res, err := s.execContext(ctx,
		`UPDATE tenants SET name = ?, owner_id = ?, settings = ?, updated_at = ? WHERE id = ?`,
		t.Name, t.OwnerID, t.Settings, t.UpdatedAt.UTC(), t.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("tenant", t.ID)
	}
	return nil
}

func (s *PostgresStore) DeleteTenant(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM tenants WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("tenant", id)
	}
	return nil
}

// --- Tenant members ---

func (s *PostgresStore) SaveTenantMember(ctx context.Context, m *TenantMemberRecord) error {
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	_, err := s.execContext(ctx, `
		INSERT INTO tenant_members (tenant_id, user_id, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = EXCLUDED.role, updated_at = EXCLUDED.updated_at`,
		m.TenantID, m.UserID, m.Role, m.CreatedAt.UTC(), m.UpdatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) GetTenantMember(ctx context.Context, tenantID, userID string) (*TenantMemberRecord, error) {
	m := &TenantMemberRecord{}
	err := s.queryRowContext(ctx,
		`SELECT tenant_id, user_id, role, created_at, updated_at FROM tenant_members WHERE tenant_id = ? AND user_id = ?`,
		tenantID, userID,
	).Scan(&m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("tenant_member", tenantID+"/"+userID)
	}
	return m, err
}

func (s *PostgresStore) ListTenantMembers(ctx context.Context, tenantID string) ([]*TenantMemberRecord, error) {
	rows, err := s.queryContext(ctx,
		`SELECT tenant_id, user_id, role, created_at, updated_at FROM tenant_members WHERE tenant_id = ? ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []*TenantMemberRecord
	for rows.Next() {
		m := &TenantMemberRecord{}
		if err := rows.Scan(&m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *PostgresStore) DeleteTenantMember(ctx context.Context, tenantID, userID string) error {
	res, err := s.execContext(ctx, `DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?`, tenantID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("tenant_member", tenantID+"/"+userID)
	}
	return nil
}

// --- Policies ---

func (s *PostgresStore) CreatePolicy(ctx context.Context, p *PolicyRecord) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := s.execContext(ctx,
		`INSERT INTO policies (id, tenant_id, resource_type, effect, pattern, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.TenantID, p.ResourceType, p.Effect, p.Pattern, p.Priority, p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
	)
	return err
}

func (s *PostgresStore) GetPolicy(ctx context.Context, id string) (*PolicyRecord, error) {
	p := &PolicyRecord{}
	err := s.queryRowContext(ctx,
		`SELECT id, tenant_id, resource_type, effect, pattern, priority, created_at, updated_at FROM policies WHERE id = ?`, id,
	).Scan(&p.ID, &p.TenantID, &p.ResourceType, &p.Effect, &p.Pattern, &p.Priority, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("policy", id)
	}
	return p, err
}

func (s *PostgresStore) ListPolicies(ctx context.Context, query PolicyQuery) ([]*PolicyRecord, error) {
	q := `SELECT id, tenant_id, resource_type, effect, pattern, priority, created_at, updated_at FROM policies WHERE 1=1`
	var args []any
	if query.TenantID != "" {
		q += " AND (tenant_id = ? OR tenant_id = '')"
		args = append(args, query.TenantID)
	} else {
		q += " AND tenant_id = ''"
	}
	if query.ResourceType != "" {
		q += " AND resource_type = ?"
		args = append(args, query.ResourceType)
	}
	q += " ORDER BY priority ASC, created_at ASC"
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []*PolicyRecord
	for rows.Next() {
		p := &PolicyRecord{}
		if err := rows.Scan(&p.ID, &p.TenantID, &p.ResourceType, &p.Effect, &p.Pattern, &p.Priority, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *PostgresStore) DeletePolicy(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM policies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return NotFoundError("policy", id)
	}
	return nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}
