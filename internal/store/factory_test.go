package store

import (
	"errors"
	"path/filepath"
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

func TestOpenPostgresReportsUnsupportedDriver(t *testing.T) {
	_, err := Open(Config{Driver: DriverPostgres, DSN: "postgres://stacyvm@example/stacyvm"})
	if !errors.Is(err, ErrUnsupportedDriver) {
		t.Fatalf("err = %v, want ErrUnsupportedDriver", err)
	}
}

func TestOpenRejectsMissingSQLitePath(t *testing.T) {
	_, err := Open(Config{Driver: DriverSQLite})
	if err == nil {
		t.Fatal("expected missing sqlite path error")
	}
}
