# Phase 3 Quotas And Scheduling Release Notes

Date: 2026-05-08
Branch: `phase-3-quotas-and-scheduling`

## Summary

Phase 3 adds the first production multi-tenant control plane for StacyVM. The server now supports persisted owner quota policies, API rate limiting, spawn backpressure, scheduler visibility, quota audit events, admission preflight, and SDK helpers for the new quota and admission surfaces.

The goal of this phase is to make StacyVM safer under shared usage and load: operators can define per-owner limits, clients can understand whether work will run or queue, and dashboards can observe queue pressure and quota coverage.

## What Changed

### Persistent Owner Quotas

- Added persisted owner quota policies backed by SQLite.
- Quotas can override:
  - max active sandboxes per owner
  - max sandbox TTL
  - max exec timeout
- Added owner quota APIs:
  - `GET /api/v1/quotas`
  - `GET /api/v1/quotas/summary`
  - `GET /api/v1/quotas/{ownerID}`
  - `PUT /api/v1/quotas/{ownerID}`
  - `DELETE /api/v1/quotas/{ownerID}`
  - `GET /api/v1/quotas/{ownerID}/usage`
- Owner IDs are normalized and validated before quota use.
- Quota saves and deletes emit audit events.

### Spawn Admission And Backpressure

- Added serialized spawn admission checks to prevent concurrent over-admission.
- Added configurable spawn overflow behavior:
  - `reject`
  - `queue`
- Added configurable queue controls:
  - `defaults.spawn_queue_timeout`
  - `defaults.max_spawn_queue`
- Queued spawn requests resume when capacity opens or owner quota changes.
- Spawn queue timeouts return typed resource-limit errors.
- Added `POST /api/v1/sandboxes/admission` for preflight admission checks without creating provider resources.

### API Rate Limiting

- Added optional in-memory API rate limiting.
- Supported rate-limit keys:
  - owner
  - API key
  - IP address
- Rate-limit buckets use hashed keys so raw identifiers are not exposed in memory snapshots or metrics.
- Inactive rate-limit buckets are pruned on a configurable interval.

### Scheduler And Quota Observability

- Diagnostics and metrics now include scheduler state, queue depth, queue totals, queue timeouts, wait totals, wait max, and wait averages.
- Prometheus now exposes spawn queue gauges/counters and quota summary metrics.
- Added redacted quota summary counts for operators without exposing owner IDs.

### Streaming Timeout Semantics

- Streaming exec deadline expiry still emits `exec.timeout` and a timeout stderr chunk.
- Caller cancellation is no longer mislabeled as an exec timeout.
- Pre-stream exec limit errors now use the central API error mapper, so streaming and non-streaming exec return consistent status codes.

### SDK Support

- TypeScript SDK:
  - Added `client.admission(...)`.
  - Added `client.quotaSummary()`.
  - Added `SpawnAdmissionDecision` and `QuotaSummary` types.
  - Added `owner_id` on `SpawnOptions`.
- Python SDK:
  - Added `Client.admission(...)` and `AsyncClient.admission(...)`.
  - Added `Client.quota_summary()` and `AsyncClient.quota_summary()`.
  - Added `SpawnAdmissionDecision` and `QuotaSummary` models.
  - Added `owner_id` spawn parameter.

## Code Changes By Area

### API Routes

- `internal/api/routes/quotas.go`
  - Added owner quota CRUD, owner usage, and redacted summary routes.
- `internal/api/routes/sandboxes.go`
  - Added spawn admission preflight.
  - Aligned streaming exec preflight error mapping with non-streaming exec.
- `internal/api/routes/system.go`
  - Added scheduler, quota, and rate-limit data to diagnostics and metrics.
- `internal/api/routes/prometheus.go`
  - Added scheduler queue, quota summary, and rate-limit metrics.

### Orchestrator

- `internal/orchestrator/manager.go`
  - Added quota enforcement, quota summary, owner usage, spawn admission decisions, queue wait/resume behavior, and refined stream timeout handling.
- `internal/orchestrator/types.go`
  - Added quota, owner usage, scheduler status, and admission decision types.
- `internal/orchestrator/events.go`
  - Added spawn queue and quota audit events.

### Store And Config

- `internal/store/migrations.go`
  - Added `owner_quotas` persistence.
- `internal/store/sqlite.go`
  - Added owner quota CRUD.
- `internal/config/config.go`
  - Added spawn queue and API rate-limit configuration.

### SDKs And Docs

- `sdk/js/src/client.ts` and `sdk/js/src/types.ts`
  - Added quota summary and admission helpers/types.
- `sdk/python/stacyvm/client.py`, `sdk/python/stacyvm/async_client.py`, and `sdk/python/stacyvm/models.py`
  - Added quota summary and admission helpers/models.
- `docs/api.md`, `docs/swagger.yaml`, `docs/swagger.json`, and `docs/docs.go`
  - Documented and regenerated the Phase 3 API surface.

## Verification

The following checks passed during Phase 3 closeout:

```sh
go test ./internal/api/routes ./internal/orchestrator
make build
cd web && npm run build
make test
```

Full `make test` requires local socket-binding permission for `httptest` integration servers in this sandboxed environment.

## Platform Notes

- Docker daemon validation remains host-gated when the local sandbox cannot access Docker.
- Firecracker conformance remains Linux/KVM-gated.
- PRoot conformance remains gated on a real `proot` binary and usable rootfs.

## Impact

Phase 3 makes StacyVM meaningfully safer for shared usage:

- Operators can assign persistent per-owner policy.
- Clients can preflight work before spawning provider resources.
- Burst load can queue instead of failing immediately.
- Queue pressure is observable in JSON and Prometheus metrics.
- API rate limiting protects the control plane.
- SDKs expose the new control-plane helpers directly.

## Next Phase Direction

Phase 4 should focus on distributed production operation: durable distributed scheduling semantics, deployment/CI hardening, runtime conformance on real platform hosts, and deeper admin workflows for quota policy management.
