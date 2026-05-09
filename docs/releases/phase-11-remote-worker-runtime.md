# Phase 11 Remote Worker Runtime Release Notes

Date: 2026-05-09
Branch: `phase-11-remote-worker-runtime`

## Summary

Phase 11 turns the Phase 10 worker RPC contract into a real remote-worker runtime path. It adds a worker process, worker-specific authentication, worker heartbeat, inbound worker RPC, and control-plane routing for remote spawn, status, destroy, lease renewal, and drain-mode shutdown.

This is now suitable for internal two-process staging with the mock provider. It is not yet enterprise multi-worker production: remote exec/files/logs/previews, production-grade worker identity, Postgres-backed lease semantics, and assignment handoff remain future work.

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
- Sandbox reads now refresh remote-owned sandbox state through `worker.status` using the persisted provider `runtime_id`.
- Remote status refresh updates persisted sandbox state when the worker reports a state change.
- Implemented worker-side `worker.destroy` with lease validation and provider runtime teardown.
- Added control-plane remote destroy routing for remote-owned sandboxes using persisted `worker_id` and `runtime_id`.
- Remote destroy updates sandbox state, releases the durable lease, and publishes the normal destroyed event.
- Implemented `worker.shutdown` drain behavior: the worker marks itself draining, rejects new spawn assignments, and reports `draining` on subsequent heartbeats.
- `worker.spawn` returns the control-plane sandbox ID and provider runtime ID separately, and the control plane persists that mapping for later routing.
- Destroy now uses the worker RPC path for remote-owned sandboxes.
- Added a two-process remote worker staging guide in `docs/remote-worker-staging.md`.
- Added `scripts/smoke-remote-worker.sh` to exercise control plane plus worker with the mock provider.

## Code Areas

- `cmd/stacyvm/cmd_worker.go`: remote worker command.
- `internal/worker`: worker heartbeat client, runtime loop, and inbound worker RPC handler.
- `internal/api/middleware/auth.go`: worker auth role, scope, and credential validation.
- `internal/api/server.go`: worker-only heartbeat route.
- `internal/api/routes/workers.go`: worker ID ownership check for heartbeat.
- `internal/config/config.go`: worker runtime and worker token configuration.
- `docs/remote-worker-staging.md`: two-process staging guide.
- `scripts/smoke-remote-worker.sh`: local mock remote-worker smoke flow.

## Verification

- `go test ./internal/config ./internal/api ./internal/worker ./cmd/stacyvm`
- `go test ./internal/worker ./internal/config ./cmd/stacyvm`
- `go test ./...`
- `scripts/check-swagger.sh`
- `bash -n scripts/smoke-remote-worker.sh`
- `scripts/smoke-remote-worker.sh /private/tmp/stacyvm-phase11-smoke`
- `npm run build` in `web`
- `npm run build` in `sdk/js`
- `PYTHONPYCACHEPREFIX=/private/tmp/stacyvm-pycache python3 -m compileall stacyvm` in `sdk/python`
- `scripts/ci-upgrade-migration.sh`
- `scripts/ci-public-release-sanity.sh`

## Next Phase Direction

- Extend worker routing beyond spawn/status/destroy to exec, files, logs, and previews.
- Add production-grade drain handoff/reassignment across workers.
