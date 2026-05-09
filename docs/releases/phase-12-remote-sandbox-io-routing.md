# Phase 12 Remote Sandbox I/O Routing Release Notes

Date: 2026-05-09
Branch: `phase-12-remote-sandbox-io-routing`

## Summary

Phase 12 extends the Phase 11 remote worker runtime beyond lifecycle operations into sandbox I/O routing and conservative remote ownership handling.

Non-streaming exec, live exec-stream calls, file APIs, console logs, preview metadata, and drain/offline ownership policy are now routed through remote worker ownership.

## What Changed

### Worker RPC Contract

- Added the `worker.exec` method to the worker RPC contract.
- Added the `worker.exec_stream` method to the worker RPC contract.
- Added NDJSON stream transport for `worker.exec_stream`.
- Added `workerproto.ExecParams` for command, argv mode, environment, workdir, timeout, provider, sandbox ID, and provider runtime ID.
- Added `workerproto.ExecResult` for exit code, stdout, and stderr.
- Added `workerproto.ExecStreamResult` for stdout/stderr chunk delivery.
- Added worker file RPC methods for write, read, list, delete, move, chmod, stat, and glob.
- Added worker file result payloads for content, file listings, stat entries, and glob matches.
- Added `worker.logs` for remote console log retrieval.
- Added `worker.preview_domain` config and heartbeat capacity advertisement for worker-specific preview ingress.
- Added `unhealthy` and `expired` sandbox states for remote ownership reconciliation.
- Added the `worker:exec` scope constant for future token-scoped worker identity.
- Added the `worker:files` scope constant for future token-scoped worker identity.
- Added the `worker:logs` scope constant for future token-scoped worker identity.

### Worker Runtime

- Implemented worker-side `worker.exec` handling in the inbound RPC server.
- Implemented worker-side `worker.exec_stream` handling in the inbound RPC server.
- Worker exec resolves the provider runtime ID, runs through the worker's provider registry, and returns a typed RPC result.
- Worker exec and live exec stream honor the provided timeout string as a worker-side context deadline when present.
- Implemented worker-side file read/write/list/delete/move/chmod/stat/glob handling in the inbound RPC server.
- Implemented worker-side console log handling in the inbound RPC server.
- Added typed `RPCClient.Exec` support for control-plane calls.
- Added typed `RPCClient.ExecStream` support for control-plane calls.
- Added typed `RPCClient.ExecStreamLive` support for live NDJSON control-plane calls.
- Added typed file RPC client helpers for control-plane calls.
- Added typed logs RPC client support for control-plane calls.

### Control Plane Routing

- `Manager.Exec` now detects remote-owned sandboxes and routes non-streaming exec to the owning worker RPC endpoint.
- `Manager.ExecStream` now detects remote-owned sandboxes and routes live exec streams to the owning worker RPC endpoint.
- File APIs now detect remote-owned sandboxes and route through the owning worker RPC endpoint.
- Console logs now detect remote-owned sandboxes and route through the owning worker RPC endpoint.
- Remote-owned sandboxes now return the owning worker's advertised preview domain when present.
- Startup reconciliation now applies a remote worker ownership policy:
  - fresh draining workers keep existing sandbox ownership and remain unavailable for new placement.
  - stale, offline, or missing worker ownership marks non-expired sandboxes `unhealthy`.
  - expired remote-owned sandboxes become `expired` and release their durable lease.
- Remote exec uses persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote file APIs use persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote logs use persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote preview URLs use persisted worker ownership plus worker heartbeat capacity.
- Remote exec keeps the existing event, audit, metrics, timeout, and exec-log behavior.
- Remote exec stream keeps the existing manager channel API while forwarding worker NDJSON chunks as they arrive.
- Remote sandboxes no longer inherit pool-mode default workdir or file path scoping just because their provider runtime ID is stored in `VMID`.

## Code Areas

- `internal/workerproto/protocol.go`: `worker.exec`, `worker.exec_stream`, file RPC, and logs RPC contract/result types.
- `internal/worker/rpc.go`: worker-side exec, exec-stream, file, and logs RPC handlers.
- `internal/worker/rpc_client.go`: typed exec, live exec-stream, file, and logs RPC client methods.
- `internal/orchestrator/manager.go`: remote-owned sandbox exec, exec-stream, file, and logs routing.
- `internal/orchestrator/scheduler.go`: expired sandbox ownership exclusion from placement capacity.
- `cmd/stacyvm/cmd_worker.go`: worker preview domain capacity advertisement and Docker preview-domain wiring.
- `internal/config/config.go`: worker preview domain configuration.
- `internal/worker/rpc_test.go`: worker exec and exec-stream RPC handler coverage.
- `internal/worker/rpc_client_test.go`: typed exec, exec-stream, file, and logs client coverage.
- `internal/orchestrator/manager_test.go`: control-plane remote exec, exec-stream, file, and logs routing coverage.

## Verification

- `go test ./internal/workerproto ./internal/worker ./internal/orchestrator`
- `go test ./...`
- `git diff --check`

## Remaining Direction

- Real stateful runtime migration is still provider-dependent and should be implemented per provider through snapshot or migration capabilities rather than simulated by the control plane.
