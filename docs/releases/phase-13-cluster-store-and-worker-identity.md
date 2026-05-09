# Phase 13 Cluster Store And Worker Identity Release Notes

Date: 2026-05-09
Branch: `phase-13-cluster-store-and-worker-identity`

## Summary

Phase 13 starts the enterprise multi-worker production track after Phase 12 completed remote sandbox I/O routing. The checkpoints make persistence explicitly driver-based, add a reusable store contract harness, and link a Postgres-backed store path while keeping SQLite as the default supported store.

## What Changed

### Store Factory

- Added `store.Open` with explicit `sqlite` and `postgres` driver handling.
- SQLite remains the default and continues to use the existing `NewSQLiteStore` implementation.
- Postgres now opens through `NewPostgresStore` with the pgx stdlib driver.
- Added factory tests for default SQLite opening and missing database configuration.

### Postgres Migration Foundation

- Added Postgres-native migration definitions for the current store schema.
- Introduced shared migration metadata so SQLite and Postgres migration versions can be compared directly.
- Added tests that verify Postgres migrations track SQLite migration versions.
- Added tests that verify Postgres migrations cover all store tables and avoid SQLite-only dialect tokens.
- Added a Postgres store migrator that applies those migrations through `store.Open`.

### Store Contract Harness

- Added a reusable store contract test harness in `internal/store`.
- Wired the contract harness to SQLite as the first concrete driver.
- Wired the contract harness to Postgres when `STACYVM_POSTGRES_TEST_DSN` is set.
- Covered sandbox lifecycle semantics, including soft-delete behavior and active-list filtering.
- Covered worker registry behavior for save, update, list, get, and delete.
- Covered lease acquisition, renewal, conflict detection, release ownership, and expired lease takeover.
- Covered live Postgres lease acquisition and expired-takeover races across multiple store connections.
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
- `stacyvm config lint` accepts Postgres configs with a valid DSN.
- `stacyvm config lint --production` now distinguishes shared staging worker tokens from production per-worker credentials.

### Worker Identity

- Added `auth.worker_tokens` as a map of `worker_id: token`.
- Kept `auth.worker_token` for shared-token staging compatibility.
- Per-worker credentials override the shared worker token for that worker ID.
- Worker identities now receive explicit scopes for heartbeat, spawn, destroy, status, exec, files, logs, and leases.
- Worker lease renewal now requires the dedicated `worker:lease` scope.

### Cluster Conformance

- Added `scripts/ci-cluster-conformance.sh`.
- Added the `cluster-conformance` GitHub Actions job.
- Added the `remote-worker-postgres-smoke` GitHub Actions job.
- Added `docs/cluster-conformance.md` with store, worker identity, runtime, and promotion gates.
- CI now verifies the SQLite store contract, live Postgres store contract, live Postgres lease concurrency, worker identity tests, production-aligned cluster config linting, and a Postgres-backed remote worker smoke.

## Verification

- `go test ./internal/store`
- `go test ./internal/api/middleware ./internal/api ./internal/config ./cmd/stacyvm`
- `scripts/ci-cluster-conformance.sh`

## Next Phase 13 Direction

- Continue worker identity hardening toward signed tokens or mTLS transport enforcement.
- Extend multi-worker conformance beyond the mock-provider smoke into Docker/gVisor/Kata/Firecracker certified hosts.
