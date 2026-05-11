package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupAndRestoreSQLite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "stacyvm.db")
	backup := filepath.Join(dir, "backup", "stacyvm.db")
	target := filepath.Join(dir, "restored.db")

	writeTestSQLite(t, source, "phase8")
	if err := backupSQLite(ctx, source, backup, false); err != nil {
		t.Fatalf("backup sqlite: %v", err)
	}
	if err := checkSQLiteIntegrity(ctx, backup); err != nil {
		t.Fatalf("backup integrity: %v", err)
	}

	writeTestSQLite(t, target, "old")
	if err := os.WriteFile(target+"-wal", []byte("stale wal"), 0600); err != nil {
		t.Fatalf("write stale wal: %v", err)
	}
	if err := os.WriteFile(target+"-shm", []byte("stale shm"), 0600); err != nil {
		t.Fatalf("write stale shm: %v", err)
	}
	if err := restoreSQLite(ctx, backup, target); err != nil {
		t.Fatalf("restore sqlite: %v", err)
	}
	got := readTestSQLiteValue(t, target)
	if got != "phase8" {
		t.Fatalf("restored value = %q, want phase8", got)
	}
	matches, err := filepath.Glob(target + ".pre-restore-*")
	if err != nil {
		t.Fatalf("glob safety copy: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one safety copy, got %d", len(matches))
	}
	for _, sidecar := range []string{target + "-wal", target + "-shm"} {
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("expected stale sidecar %s to be removed, got %v", sidecar, err)
		}
	}
}

func TestBackupSQLiteRefusesOverwriteWithoutForce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "stacyvm.db")
	backup := filepath.Join(dir, "backup.db")
	writeTestSQLite(t, source, "phase8")
	if err := os.WriteFile(backup, []byte("exists"), 0600); err != nil {
		t.Fatalf("write existing backup: %v", err)
	}

	err := backupSQLite(ctx, source, backup, false)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
	if err := backupSQLite(ctx, source, backup, true); err != nil {
		t.Fatalf("backup with force: %v", err)
	}
}

func writeTestSQLite(t *testing.T, path, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sanity (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM sanity`); err != nil {
		t.Fatalf("delete table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sanity (value) VALUES (?)`, value); err != nil {
		t.Fatalf("insert value: %v", err)
	}
}

func readTestSQLiteValue(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(`SELECT value FROM sanity LIMIT 1`).Scan(&value); err != nil {
		t.Fatalf("select value: %v", err)
	}
	return value
}
