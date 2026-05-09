# Phase 13 Cluster Store And Worker Identity Release Notes

Date: 2026-05-09
Branch: `phase-13-cluster-store-and-worker-identity`

## Summary

Phase 13 starts the enterprise multi-worker production track after Phase 12 completed remote sandbox I/O routing. The first checkpoint makes persistence explicitly driver-based so the codebase has a clean seam for Postgres-backed cluster storage while keeping SQLite as the default supported store.

Postgres is intentionally not marked production-ready in this checkpoint. Config can express it, and the store factory fails clearly when this build does not link a Postgres implementation.

## What Changed

### Store Factory

- Added `store.Open` with explicit `sqlite` and `postgres` driver handling.
- SQLite remains the default and continues to use the existing `NewSQLiteStore` implementation.
- Postgres config now fails with `ErrUnsupportedDriver` until a Postgres store driver is linked.
- Added factory tests for default SQLite opening, missing SQLite path validation, and unsupported Postgres behavior.

### Configuration

- Added `database.driver`.
- Added `database.dsn`.
- Kept `database.path` for SQLite.
- Config validation now rejects unsupported database drivers.
- Config validation now requires `database.dsn` when `database.driver` is `postgres`.

### CLI And Diagnostics

- `stacyvm serve` now opens the store through the driver-based factory.
- `stacyvm config lint` and `stacyvm doctor` report clear Postgres-driver warnings for this build.

## Verification

- Pending for final Phase 13 cleanup.

## Next Phase 13 Direction

- Add a real Postgres store implementation with migration management.
- Add store contract tests that run against SQLite and Postgres.
- Add production-grade worker identity beyond shared worker tokens.
- Add cluster conformance docs and CI paths for Postgres-backed multi-worker mode.
