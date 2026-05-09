# Phase 11 Remote Worker Runtime Release Notes

Date: 2026-05-09
Branch: `phase-11-remote-worker-runtime`

## Summary

Phase 11 turns the Phase 10 worker RPC contract into a real remote-worker runtime path. The first slice adds a worker process, worker-specific authentication, and a heartbeat endpoint that is intentionally separate from regular API/admin keys.

This is still a staging foundation. Remote spawn, destroy, status routing, and lease-token lifecycle mutations remain follow-up slices inside Phase 11.

## What Changed

### Worker Runtime Command

- Added `stacyvm worker` as the entrypoint for remote worker processes.
- Added `--id`, `--control-plane`, `--worker-token`, `--heartbeat-interval`, `--listen`, and `--once` flags.
- The worker ID defaults to `worker.id`, then the host name.
- `--once` sends a single heartbeat and exits for smoke tests and staging probes.
- `--listen` starts the inbound worker RPC server for control-plane-to-worker calls.

### Worker Configuration

- Added `worker.id`.
- Added `worker.control_plane_url`.
- Added `worker.listen_addr`.
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
- Added a worker-only control-plane lease renewal endpoint:
  - `POST /api/v1/worker/{workerID}/leases/{resourceID}/renew`
- The endpoint persists worker host, status, provider list, capability list, capacity, and last heartbeat timestamp through the existing worker registry.

### Worker RPC Transport

- Added worker-side `/rpc` HTTP handling for `workerproto.Request` envelopes.
- Added worker RPC authentication with `X-Worker-ID` and `X-Worker-Token`.
- Implemented `worker.status` against the local worker provider registry.
- Implemented `worker.renew_lease` with resource, holder, and expiry validation before calling the control plane to renew the durable lease.
- Implemented worker-side `worker.spawn` with lease validation and provider-backed runtime creation.
- Added a typed worker RPC client for control-plane calls to `worker.spawn` and `worker.status`.
- Added control-plane remote spawn assignment when the scheduler selects a non-local worker with an advertised `rpc_url`.
- Remote spawn now acquires the durable sandbox lease for the selected worker before calling `worker.spawn`.
- Remote spawn persists the control-plane sandbox ID, selected `worker_id`, and provider `runtime_id`.
- Implemented a no-op `worker.shutdown` acknowledgement as a transport smoke path.
- `worker.spawn` returns the control-plane sandbox ID and provider runtime ID separately, and the control plane persists that mapping for later routing.
- Destroy returns explicit not-implemented responses until the destroy transport lands.

## Code Areas

- `cmd/stacyvm/cmd_worker.go`: remote worker command.
- `internal/worker`: worker heartbeat client, runtime loop, and inbound worker RPC handler.
- `internal/api/middleware/auth.go`: worker auth role, scope, and credential validation.
- `internal/api/server.go`: worker-only heartbeat route.
- `internal/api/routes/workers.go`: worker ID ownership check for heartbeat.
- `internal/config/config.go`: worker runtime and worker token configuration.

## Verification

- `go test ./internal/config ./internal/api ./internal/worker ./cmd/stacyvm`
- `go test ./internal/worker ./internal/config ./cmd/stacyvm`

## Remaining Phase 11 Direction

- Add worker RPC endpoints for destroy and real graceful shutdown.
- Route destroy/status calls to the owning remote worker.
- Extend lease-token enforcement from renewals to worker-side spawn/destroy lifecycle mutations.
- Add a two-process staging guide for `stacyvm serve` plus `stacyvm worker` using the mock provider first.
