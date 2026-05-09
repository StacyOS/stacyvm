package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDefaultsToSQLite(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(Config{Path: filepath.Join(dir, "test.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if closer, ok := st.(*SQLiteStore); ok {
		closer.Close()
	} else {
		t.Fatalf("store type = %T, want *SQLiteStore", st)
	}
}

func TestOpenPostgresRejectsMissingDSN(t *testing.T) {
	_, err := Open(Config{Driver: DriverPostgres})
	if err == nil || !strings.Contains(err.Error(), "postgres database dsn is required") {
		t.Fatalf("err = %v, want missing postgres dsn error", err)
	}
}

func TestOpenRejectsMissingSQLitePath(t *testing.T) {
	_, err := Open(Config{Driver: DriverSQLite})
	if err == nil {
		t.Fatal("expected missing sqlite path error")
	}
}
