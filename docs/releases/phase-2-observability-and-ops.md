# Phase 2 Observability And Ops Release Notes

Date: 2026-05-08
Branch: `phase-2-observability-and-ops`

## Summary

Phase 2 turns the Phase 1 foundation into an operable production surface. The API now exposes liveness, readiness, diagnostics, structured metrics, Prometheus scraping, richer provider health, operational audit events, and configurable runtime limits.

The goal of this phase is to make StacyVM easier to run, debug, monitor, and safely scale before deeper multi-tenant and production deployment work.

## What Changed

### Liveness And Readiness

- Added `/api/v1/live` for process liveness checks.
- Added `/api/v1/ready` for dependency readiness checks.
- Readiness now reports provider health instead of only returning a generic process status.

### Structured Runtime Metrics

- Added an in-process operation metrics recorder.
- Operations tracked include:
  - spawn
  - exec
  - exec stream
  - destroy
  - file write, read, list, delete, move, chmod, stat, and glob
- Each operation tracks:
  - success count
  - failure count
  - latency count
  - total latency
  - min, max, and average latency
  - last error
  - last observed timestamp
- `/api/v1/metrics` now includes sandbox, provider, event, process, runtime, and operation metrics.

### Prometheus Metrics

- Added `/api/v1/metrics/prometheus`.
- The Prometheus endpoint exposes:
  - process uptime
  - goroutines
  - memory and GC metrics
  - sandbox counts by state and provider
  - provider health
  - provider health latency
  - provider runtime inventory counts
  - event bus stats
  - operation success/failure and latency counters

### Operational Audit Events

- Added event IDs for published events.
- Added operational event types:
  - `exec.failed`
  - `exec.timeout`
  - `operation.failed`
  - `resource.limit`
  - `provider.failed`
  - `reconcile.action`
- Manager paths now publish audit events for:
  - exec failures and timeouts
  - stream exec timeouts
  - file operation failures
  - spawn/provider/resource failures
  - destroy provider failures
  - reconciliation actions and provider inventory failures

### Provider Health Detail

- Provider health now includes:
  - `latency_ms`
  - `last_checked`
  - `error`
  - `capabilities`
  - `runtime_count` when runtime inventory is supported
- Provider health detail is shared across:
  - `/api/v1/ready`
  - `/api/v1/metrics`
  - `/api/v1/metrics/prometheus`
  - `/api/v1/providers`
  - `/api/v1/providers/{name}`

### Redacted Diagnostics

- Added `/api/v1/diagnostics`.
- Diagnostics include:
  - generated timestamp
  - version/build info
  - GOOS/GOARCH
  - uptime, goroutines, memory, and GC cycles
  - store health and latency
  - active operational limits
  - detailed provider health
  - sandbox counts by state/provider
  - event bus stats
  - operation metrics
  - explicit redaction categories
- Diagnostics are read-only and intentionally avoid returning API keys, registry credentials, provider secrets, or environment secrets.

### Operational Limits

- Added configurable defaults:
  - `defaults.max_ttl`
  - `defaults.default_exec_timeout`
  - `defaults.max_exec_timeout`
  - `defaults.max_sandboxes`
  - `defaults.max_sandboxes_per_owner`
- Manager now centrally enforces:
  - max TTL
  - max total active sandboxes
  - max active sandboxes per owner
  - default exec timeout
  - max exec timeout
- Limit violations return typed resource-limit errors and publish `resource.limit` audit events.

## Code Changes By Area

### API Routes

- `internal/api/routes/system.go`
  - Added liveness, readiness, diagnostics, JSON metrics, and Prometheus metrics behavior.
- `internal/api/routes/provider_health.go`
  - Added shared provider health detail collection.
- `internal/api/routes/prometheus.go`
  - Added Prometheus text renderer.
- `internal/api/routes/providers.go`
  - Added detailed health to provider list/detail responses.
- `internal/api/routes/system_test.go`
  - Added coverage for readiness, diagnostics, metrics, and Prometheus output.

### Orchestrator

- `internal/orchestrator/metrics.go`
  - Added operation metrics recorder.
- `internal/orchestrator/manager.go`
  - Added metrics recording, audit event publishing, and operational limit enforcement.
- `internal/orchestrator/events.go`
  - Added event IDs and operational event types.
- `internal/orchestrator/types.go`
  - Added operational limit types.
- `internal/orchestrator/manager_test.go`
  - Added tests for operation metrics, audit events, TTL limits, sandbox limits, owner limits, and exec timeout limits.

### Config And Docs

- `internal/config/config.go`
  - Added default config fields for operational limits.
- `cmd/stacyvm/cmd_serve.go`
  - Wires configured operational limits into the manager.
- `README.md`
  - Documents new operational limit config.
- `docs/api.md`
  - Documents liveness, readiness, diagnostics, metrics, Prometheus metrics, provider health detail, and operational event shape.
- `CHANGELOG.md`
  - Adds this Phase 2 checkpoint entry.

## Verification

The following checks passed:

```sh
make test
make build
cd web && npm run build
```

## Impact

Phase 2 gives StacyVM the baseline visibility and guardrails needed to operate safely:

- Operators can distinguish liveness from readiness.
- Dashboards can consume JSON or Prometheus metrics.
- Support/debug flows can use a redacted diagnostics endpoint.
- Provider health is actionable rather than a single boolean.
- Resource pressure and failure modes are visible through events.
- Runtime limits can prevent accidental overload before full multi-tenant quota systems arrive.

## Next Phase Direction

Phase 3 should focus on production scaling and multi-tenant control planes: persistent quotas, per-owner policy, rate limits, queueing/backpressure, distributed scheduler boundaries, and deployment/CI hardening.
