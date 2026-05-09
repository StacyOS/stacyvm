package store

import (
	"strings"
	"testing"
)

func TestPostgresMigrationsTrackSQLiteVersions(t *testing.T) {
	if len(postgresMigrations) != len(migrations) {
		t.Fatalf("postgres migration count = %d, want %d", len(postgresMigrations), len(migrations))
	}
	for i, sqliteMigration := range migrations {
		if postgresMigrations[i].version != sqliteMigration.version {
			t.Fatalf("postgres migration[%d] version = %d, want %d", i, postgresMigrations[i].version, sqliteMigration.version)
		}
	}
}

func TestPostgresMigrationsCoverStoreTables(t *testing.T) {
	sql := joinedMigrationSQL(postgresMigrations)
	for _, table := range []string{
		"schema_migrations",
		"sandboxes",
		"exec_logs",
		"provider_configs",
		"templates",
		"environment_specs",
		"environment_builds",
		"environment_artifacts",
		"registry_connections",
		"owner_quotas",
		"admin_audit_logs",
		"operation_audit_logs",
		"workers",
		"leases",
	} {
		if !strings.Contains(sql, table) {
			t.Fatalf("postgres migrations missing table %s", table)
		}
	}
}

func TestPostgresMigrationsUsePostgresDialect(t *testing.T) {
	sql := joinedMigrationSQL(postgresMigrations)
	for _, forbidden := range []string{
		"AUTOINCREMENT",
		"DATETIME",
		"datetime('now')",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("postgres migrations contain sqlite dialect token %q", forbidden)
		}
	}
	for _, required := range []string{
		"TIMESTAMPTZ",
		"BIGSERIAL",
		"DEFAULT now()",
		"ADD COLUMN IF NOT EXISTS",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("postgres migrations missing postgres dialect token %q", required)
		}
	}
}

func joinedMigrationSQL(migrations []migration) string {
	var b strings.Builder
	for _, migration := range migrations {
		b.WriteString(migration.sql)
		b.WriteByte('\n')
	}
	return b.String()
}
