# Phase 11 Remote Worker Runtime Release Notes

Date: 2026-05-09
Branch: `phase-11-remote-worker-runtime`

## Summary

Phase 11 turns the Phase 10 worker RPC contract into a real remote-worker runtime path. The first slice adds a worker process, worker-specific authentication, and a heartbeat endpoint that is intentionally separate from regular API/admin keys.

This is still a staging foundation. Remote spawn, destroy, status routing, and lease-token lifecycle mutations remain follow-up slices inside Phase 11.

## What Changed

### Worker Runtime Command

- Added `stacyvm worker` as the entrypoint for remote worker processes.
- Added `--id`, `--control-plane`, `--worker-token`, `--heartbeat-interval`, and `--once` flags.
- The worker ID defaults to `worker.id`, then the host name.
- `--once` sends a single heartbeat and exits for smoke tests and staging probes.

### Worker Configuration

- Added `worker.id`.
- Added `worker.control_plane_url`.
- Added `worker.heartbeat_interval`.
- Added `worker.shutdown_timeout`.
- Added `auth.worker_token` for worker-to-control-plane authentication.
- Worker durations are validated through the normal config validation path.

### Worker Authentication

- Added a dedicated worker auth role and `worker:heartbeat` scope.
- Added `X-Worker-ID` and `X-Worker-Token` validation for worker endpoints.
- Worker credentials cannot authenticate regular API/admin routes.
- Worker heartbeats are rejected when the authenticated worker ID does not match the requested worker path.

### Worker Heartbeat Transport

- Added `internal/worker` with a heartbeat client and runtime loop.
- Added a worker-only control-plane endpoint:
  - `POST /api/v1/worker/{workerID}/heartbeat`
- The endpoint persists worker host, status, provider list, capability list, capacity, and last heartbeat timestamp through the existing worker registry.

## Code Areas

- `cmd/stacyvm/cmd_worker.go`: remote worker command.
- `internal/worker`: worker heartbeat client and runtime loop.
- `internal/api/middleware/auth.go`: worker auth role, scope, and credential validation.
- `internal/api/server.go`: worker-only heartbeat route.
- `internal/api/routes/workers.go`: worker ID ownership check for heartbeat.
- `internal/config/config.go`: worker runtime and worker token configuration.

## Verification

- `go test ./internal/config ./internal/api ./internal/worker ./cmd/stacyvm`

## Remaining Phase 11 Direction

- Add worker RPC endpoints for status, lease renewal, spawn assignment, destroy, and shutdown.
- Add remote spawn assignment through the scheduler when a non-local worker is selected.
- Route destroy/status calls to the owning remote worker.
- Enforce lease tokens on worker-side lifecycle mutations.
- Add a two-process staging guide for `stacyvm serve` plus `stacyvm worker` using the mock provider first.
