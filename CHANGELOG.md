# Changelog

## Phase 4 Production Deployment - 2026-05-08

This checkpoint adds the first production deployment and verification surface for Phase 4: GitHub Actions CI, deployment templates, and an operator runbook.

### Added

- GitHub Actions workflow for Go tests/build, Swagger drift, web build, TypeScript SDK build, and Python SDK import checks.
- Production Docker Compose template with StacyVM and Traefik for live previews.
- Production baseline config with auth, rate limiting, sandbox caps, queueing, JSON logs, and persistent SQLite state.
- systemd unit and environment template for binary-based Linux installs.
- Deployment guide covering host requirements, health probes, Prometheus metrics, reverse proxy setup, backups, upgrades, and provider notes.
- Phase 4 release notes under `docs/releases/phase-4-production-deployment.md`.

### Changed

- Swagger drift checks now download Go modules before invoking `swag`, which makes cold CI runners reliable.
- README navigation now links to the production deployment guide.

### Verified

- `docker compose --env-file deploy/.env.example -f deploy/docker-compose.yml config`
- YAML parsing for deployment templates
- `git diff --check`
- `go test ./...`
- `cd web && npm run build`
- `scripts/check-swagger.sh`

## Phase 3 Quotas And Scheduling - 2026-05-08

This checkpoint adds the first production multi-tenant control plane: persisted owner quotas, API rate limiting, spawn backpressure, scheduler visibility, admission preflight, and SDK helpers.

### Added

- Persisted owner quota policies for max sandboxes, max TTL, and max exec timeout.
- Owner quota APIs, including usage and redacted summary endpoints.
- Spawn admission decisions and `POST /api/v1/sandboxes/admission`.
- Configurable spawn overflow queue with queue timeout and maximum queue depth.
- Optional API rate limiting by owner, API key, or IP address.
- Scheduler, quota, and rate-limit metrics in JSON diagnostics/metrics and Prometheus output.
- TypeScript and Python SDK helpers for admission preflight and quota summary.

### Changed

- Spawn admission is serialized to avoid concurrent over-admission.
- Queued spawns wake when capacity opens or owner quotas change.
- Rate-limit bucket keys are hashed before storage.
- Streaming exec cancellation is no longer reported as a timeout.
- Streaming exec preflight errors now use the same API error mapping as non-streaming exec.
- OpenAPI docs were regenerated for the Phase 3 API surface.

### Verified

- `go test ./internal/api/routes ./internal/orchestrator`
- `make build`
- `cd web && npm run build`
- `make test`

## Phase 2 Observability And Ops - 2026-05-08

This checkpoint adds production operations surfaces for health checks, diagnostics, metrics, audit events, and runtime limits.

### Added

- Liveness endpoint at `/api/v1/live`.
- Readiness endpoint at `/api/v1/ready` with detailed provider health.
- Redacted diagnostics endpoint at `/api/v1/diagnostics`.
- Structured JSON operation metrics on `/api/v1/metrics`.
- Prometheus-compatible metrics endpoint at `/api/v1/metrics/prometheus`.
- Provider health detail with latency, last checked time, capabilities, error reason, and runtime inventory count when supported.
- Operational audit events for exec failures, exec timeouts, provider failures, resource limits, and reconciliation actions.
- Configurable operational limits for max TTL, default/max exec timeout, max sandboxes, and max sandboxes per owner.

### Changed

- `/api/v1/metrics` now includes sandbox state/provider breakdown, provider health, event bus stats, and operation metrics.
- `/api/v1/providers` and `/api/v1/providers/{name}` now expose richer provider health details.
- Diagnostics include store health, build/runtime data, sandbox counts, provider health, event stats, operation metrics, and explicit redaction categories.
- Manager-level spawn and exec flows now enforce configured operational limits centrally.

### Verified

- `make test`
- `make build`
- `cd web && npm run build`

## Phase 1 Foundation Hardening - 2026-05-08

This checkpoint closes the Phase 1 reliability and production-readiness foundation.

### Added

- Provider contract documentation in `docs/provider-contract.md`.
- Typed provider errors for sandbox lifecycle, provider availability, exec timeout, and resource-limit failures.
- Typed store errors for not-found and conflict cases.
- Shared API route error mapping with explicit `404`, `408`, `429`, and `503` responses.
- Provider conformance harness covering lifecycle, exec, streaming exec, and file operations.
- Mock, Docker, Custom, PRoot, and Firecracker conformance coverage, with PRoot and Firecracker gated on platform dependencies.
- Startup reconciliation that refreshes persisted sandbox state from provider runtime state.
- Docker runtime inventory and adoption for StacyVM containers missing from SQLite after process restart.
- Streaming exec timeout handling that emits an explicit stderr timeout chunk.
- Non-Linux `stacyvm-agent` stub so repository builds work on macOS while the real agent remains Linux-only.

### Changed

- Sandbox, template, environment, and provider routes now use typed errors instead of string matching.
- Docker sandboxes now include richer `stacyvm.*` labels for reconciliation and metadata recovery.
- Docker missing-container paths now map to `ErrSandboxNotFound`.
- Manager `Exec` and `ExecStream` now consistently honor caller-supplied timeouts.
- Provider comments now point implementers to the documented contract and conformance tests.

### Verified

- `make test`
- `make build`
- `cd web && npm run build`
- Docker provider conformance and runtime inventory tests with Docker daemon access

### Platform Notes

- Firecracker conformance is available on Linux hosts with `/dev/kvm`, Firecracker, kernel, rootfs, and agent paths configured.
- PRoot conformance is available when `proot` and a usable rootfs are installed.
- Local sandboxed test runs still need permission to bind `httptest` sockets for the full integration suite.
