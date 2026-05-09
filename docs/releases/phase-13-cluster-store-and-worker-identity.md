# Phase 13 Cluster Store And Worker Identity Release Notes

Date: 2026-05-09
Branch: `phase-13-cluster-store-and-worker-identity`

## Summary

Phase 13 starts the enterprise multi-worker production track after Phase 12 completed remote sandbox I/O routing. The first checkpoints make persistence explicitly driver-based and add a reusable store contract harness so the codebase has a clean seam for Postgres-backed cluster storage while keeping SQLite as the default supported store.

Postgres is intentionally not marked production-ready in this checkpoint. Config can express it, and the store factory fails clearly when this build does not link a Postgres implementation.

## What Changed

### Store Factory

- Added `store.Open` with explicit `sqlite` and `postgres` driver handling.
- SQLite remains the default and continues to use the existing `NewSQLiteStore` implementation.
- Postgres config now fails with `ErrUnsupportedDriver` until a Postgres store driver is linked.
- Added factory tests for default SQLite opening, missing SQLite path validation, and unsupported Postgres behavior.

### Store Contract Harness

- Added a reusable store contract test harness in `internal/store`.
- Wired the contract harness to SQLite as the first concrete driver.
- Covered sandbox lifecycle semantics, including soft-delete behavior and active-list filtering.
- Covered worker registry behavior for save, update, list, get, and delete.
- Covered lease acquisition, renewal, conflict detection, release ownership, and expired lease takeover.
- Covered exec logs, admin audit logs, operation audit logs, owner quotas, and provider configs.
- Covered templates, environment specs, environment builds, build artifacts, and registry connections.

### Configuration

- Added `database.driver`.
- Added `database.dsn`.
- Kept `database.path` for SQLite.
- Config validation now rejects unsupported database drivers.
- Config validation now requires `database.dsn` when `database.driver` is `postgres`.

### CLI And Diagnostics

- `stacyvm serve` now opens the store through the driver-based factory.
- `stacyvm config lint` and `stacyvm doctor` report clear Postgres-driver warnings for this build.
- `stacyvm config lint --production` now distinguishes shared staging worker tokens from production per-worker credentials.

### Worker Identity

- Added `auth.worker_tokens` as a map of `worker_id: token`.
- Kept `auth.worker_token` for shared-token staging compatibility.
- Per-worker credentials override the shared worker token for that worker ID.
- Worker identities now receive explicit scopes for heartbeat, spawn, destroy, status, exec, files, logs, and leases.
- Worker lease renewal now requires the dedicated `worker:lease` scope.

## Verification

- `go test ./internal/store`
- `go test ./internal/api/middleware ./internal/api ./internal/config ./cmd/stacyvm`

## Next Phase 13 Direction

- Add a real Postgres store implementation with migration management.
- Run the existing store contract tests against Postgres once the Postgres driver is implemented.
- Continue worker identity hardening toward signed tokens or mTLS transport enforcement.
- Add cluster conformance docs and CI paths for Postgres-backed multi-worker mode.
