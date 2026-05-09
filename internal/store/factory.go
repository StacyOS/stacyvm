package store

import (
	"errors"
	"fmt"
	"strings"
)

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

var ErrUnsupportedDriver = errors.New("unsupported store driver")

type Config struct {
	Driver string
	Path   string
	DSN    string
}

func Open(cfg Config) (Store, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = DriverSQLite
	}
	switch driver {
	case DriverSQLite, "sqlite3":
		if strings.TrimSpace(cfg.Path) == "" {
			return nil, fmt.Errorf("sqlite database path is required")
		}
		return NewSQLiteStore(cfg.Path)
	case DriverPostgres, "postgresql":
		if strings.TrimSpace(cfg.DSN) == "" {
			return nil, fmt.Errorf("postgres database dsn is required")
		}
		return NewPostgresStore(cfg.DSN)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, driver)
	}
}
