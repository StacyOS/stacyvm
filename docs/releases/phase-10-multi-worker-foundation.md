# Phase 10 Multi-Worker Foundation Release Notes

Date: 2026-05-09
Branch: `phase-10-multi-worker-foundation`

## Summary

Phase 10 starts the enterprise and multi-worker production track. This slice adds the durable worker registry foundation StacyVM needs before scheduler placement, worker ownership, leases, and remote worker RPC can be made production-grade.

This is not a full distributed runtime yet. It is the first production-aligned control-plane layer for observing workers, recording heartbeats, and exposing that state through APIs, diagnostics, and metrics.

## What Changed

### Worker Registry Storage

- Added a SQLite migration for the `workers` table.
- Added durable worker fields for ID, hostname, status, providers, capabilities, capacity, heartbeat timestamp, and lifecycle timestamps.
- Added store methods for saving, fetching, listing, and deleting worker records.

### Local Worker Registration

- The API server now registers the current process as the `local` worker at startup.
- The local record includes configured providers, single-node capabilities, and manager capacity limits.
- Single-node deployments now appear in the same worker registry surface that future multi-worker deployments will use.

### Worker API

- Added read-only worker discovery:
  - `GET /api/v1/workers`
  - `GET /api/v1/workers/{workerID}`
- Added admin-only worker mutations:
  - `POST /api/v1/admin/workers/{workerID}/heartbeat`
  - `DELETE /api/v1/admin/workers/{workerID}`
- Worker responses include a computed `stale` flag when the last heartbeat is older than the freshness window.

### Sandbox Worker Ownership

- Added persisted `worker_id` ownership to sandbox records.
- New and adopted local sandboxes are stamped with the active worker ID.
- Scheduler status now reports the current worker ID.
- Sandbox API responses now include `worker_id` when ownership is known.

### Worker-Aware Scheduler Placement

- Spawn admission now evaluates worker placement using worker status, heartbeat freshness, provider support, declared capacity, and active sandbox counts.
- Scheduler status now reports the selected worker and number of eligible workers.
- Local execution remains honest: if the scheduler would place work on a remote worker, admission reports `remote_worker_rpc_unavailable` until the worker RPC slice lands.
- The current local worker remains eligible for local execution even if its registry heartbeat has gone stale between startup registration and the future heartbeat loop.

### Distributed Lease Foundation

- Added durable lease records for resource ownership fencing.
- Added store APIs to acquire, renew, release, get, and list leases.
- Lease acquisition is holder-aware and expiry-aware: a competing worker cannot acquire an unexpired lease held by another worker.
- Lease renewals require the current holder and an unexpired lease.
- Diagnostics and Prometheus now expose lease totals so operators can inspect active and expired lease state.

### Lease Enforcement

- Local spawns now acquire a sandbox lease before persisting the sandbox record.
- Runtime adoption during reconciliation now acquires a sandbox lease before adopting unknown provider runtimes.
- Pool VM and pooled logical sandbox creation now acquire leases.
- Destroy now acquires or renews the local worker lease before mutating provider/runtime/store state.
- Successful destroy releases the sandbox lease.
- Wrong-holder lease tests now prevent local destroy from mutating a sandbox owned by another worker.

### Diagnostics And Metrics

- Diagnostics now include worker totals, online count, stale count, unhealthy count, and worker items.
- Diagnostics now include lease totals, active count, expired count, and active leases by holder.
- Diagnostics sandbox summaries now include `by_worker` counts.
- Prometheus output now includes:
  - `stacyvm_workers_total{status="total"}`
  - `stacyvm_workers_total{status="online"}`
  - `stacyvm_workers_total{status="stale"}`
  - `stacyvm_workers_total{status="unhealthy"}`
  - `stacyvm_leases_total{status="active"}`
  - `stacyvm_sandboxes_by_worker_total{worker="local"}`

### Documentation

- Updated the changelog with Phase 10 changes.
- Updated the API reference with worker endpoints and metrics.
- Updated the README endpoint table with worker discovery.
- Updated the production readiness checklist with Phase 10 acceptance criteria.

## Code Areas

- `internal/store`: worker model, lease model, migrations, SQLite CRUD, and sandbox `worker_id` persistence.
- `internal/api/routes`: worker routes, diagnostics worker summary, and Prometheus worker metrics.
- `internal/api/server.go`: local worker startup registration and route mounting.
- `docs`: API, README, changelog, production readiness, and release notes.

## Verification

- `go test ./internal/store ./internal/api/routes ./internal/api`
- `scripts/check-swagger.sh`
- `go test ./...`
- `git diff --check`

## Remaining Phase 10 Direction

- Add worker RPC so selected remote workers can execute assigned spawns.
- Enforce leases across remote worker RPC once remote workers can execute lifecycle operations.
- Add remote worker authentication and RPC.
- Add Postgres-backed worker registry semantics for production clusters.
- Add CI coverage for remote worker RPC and distributed leases once those slices land.
