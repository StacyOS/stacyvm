package store

import (
	"os"
	"testing"
)

func TestPostgresMigrationRehearsal(t *testing.T) {
	dsn := os.Getenv("STACYVM_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set STACYVM_POSTGRES_TEST_DSN to run Postgres migration rehearsal")
	}

	first, err := NewPostgresStore(dsn)
	if err != nil {
		t.Fatalf("first open postgres store: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first postgres store: %v", err)
	}

	second, err := NewPostgresStore(dsn)
	if err != nil {
		t.Fatalf("second open postgres store: %v", err)
	}
	defer second.Close()

	var applied int
	if err := second.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("count postgres migrations: %v", err)
	}
	if applied != len(postgresMigrations) {
		t.Fatalf("applied postgres migrations = %d, want %d", applied, len(postgresMigrations))
	}

	for _, migration := range postgresMigrations {
		var exists int
		if err := second.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", migration.version).Scan(&exists); err != nil {
			t.Fatalf("check postgres migration %d: %v", migration.version, err)
		}
		if exists != 1 {
			t.Fatalf("postgres migration %d recorded %d times, want once", migration.version, exists)
		}
	}
}
