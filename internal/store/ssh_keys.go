package store

import (
	"context"
	"database/sql"
	"time"
)

const sshKeyColumns = `id, owner_id, tenant_id, fingerprint, public_key, label, created_at`

func scanSSHKey(scan func(dest ...any) error) (*SSHKeyRecord, error) {
	key := &SSHKeyRecord{}
	err := scan(&key.ID, &key.OwnerID, &key.TenantID, &key.Fingerprint, &key.PublicKey, &key.Label, &key.CreatedAt)
	return key, err
}

// --- SQLite ---------------------------------------------------------------

func (s *SQLiteStore) CreateSSHKey(ctx context.Context, key *SSHKeyRecord) error {
	createdAt := key.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ssh_keys (`+sshKeyColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.OwnerID, key.TenantID, key.Fingerprint, key.PublicKey, key.Label, createdAt,
	)
	if IsConstraintError(err) {
		return ConflictError("ssh key already exists")
	}
	return err
}

func (s *SQLiteStore) GetSSHKeyByFingerprint(ctx context.Context, fingerprint string) (*SSHKeyRecord, error) {
	key, err := scanSSHKey(s.db.QueryRowContext(ctx,
		`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE fingerprint = ?`, fingerprint).Scan)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("ssh key", fingerprint)
	}
	return key, err
}

func (s *SQLiteStore) ListSSHKeysByOwner(ctx context.Context, ownerID string) ([]*SSHKeyRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE owner_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectSSHKeys(rows)
}

func (s *SQLiteStore) DeleteSSHKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ssh_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return NotFoundError("ssh key", id)
	}
	return nil
}

// --- Postgres -------------------------------------------------------------

func (s *PostgresStore) CreateSSHKey(ctx context.Context, key *SSHKeyRecord) error {
	createdAt := key.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.execContext(ctx, `
		INSERT INTO ssh_keys (`+sshKeyColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.OwnerID, key.TenantID, key.Fingerprint, key.PublicKey, key.Label, createdAt,
	)
	if IsConstraintError(err) {
		return ConflictError("ssh key already exists")
	}
	return err
}

func (s *PostgresStore) GetSSHKeyByFingerprint(ctx context.Context, fingerprint string) (*SSHKeyRecord, error) {
	key, err := scanSSHKey(s.queryRowContext(ctx,
		`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE fingerprint = ?`, fingerprint).Scan)
	if err == sql.ErrNoRows {
		return nil, NotFoundError("ssh key", fingerprint)
	}
	return key, err
}

func (s *PostgresStore) ListSSHKeysByOwner(ctx context.Context, ownerID string) ([]*SSHKeyRecord, error) {
	rows, err := s.queryContext(ctx,
		`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE owner_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectSSHKeys(rows)
}

func (s *PostgresStore) DeleteSSHKey(ctx context.Context, id string) error {
	res, err := s.execContext(ctx, `DELETE FROM ssh_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return NotFoundError("ssh key", id)
	}
	return nil
}

func collectSSHKeys(rows *sql.Rows) ([]*SSHKeyRecord, error) {
	var keys []*SSHKeyRecord
	for rows.Next() {
		key, err := scanSSHKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
