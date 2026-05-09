# Phase 12 Remote Sandbox I/O Routing Release Notes

Date: 2026-05-09
Branch: `phase-12-remote-sandbox-io-routing`

## Summary

Phase 12 starts extending the Phase 11 remote worker runtime beyond lifecycle operations. This checkpoint adds exec routing for remote-owned sandboxes, so the control plane can run commands through the owning worker instead of trying to execute against the local provider.

This is the first remote sandbox I/O slice. Non-streaming exec, buffered exec-stream calls, and file APIs are routed. Logs, previews, true live worker stream transport, and production-grade drain handoff remain future Phase 12 work.

## What Changed

### Worker RPC Contract

- Added the `worker.exec` method to the worker RPC contract.
- Added the `worker.exec_stream` method to the worker RPC contract.
- Added `workerproto.ExecParams` for command, argv mode, environment, workdir, timeout, provider, sandbox ID, and provider runtime ID.
- Added `workerproto.ExecResult` for exit code, stdout, and stderr.
- Added `workerproto.ExecStreamResult` for stdout/stderr chunk delivery.
- Added worker file RPC methods for write, read, list, delete, move, chmod, stat, and glob.
- Added worker file result payloads for content, file listings, stat entries, and glob matches.
- Added the `worker:exec` scope constant for future token-scoped worker identity.
- Added the `worker:files` scope constant for future token-scoped worker identity.

### Worker Runtime

- Implemented worker-side `worker.exec` handling in the inbound RPC server.
- Implemented worker-side `worker.exec_stream` handling in the inbound RPC server.
- Worker exec resolves the provider runtime ID, runs through the worker's provider registry, and returns a typed RPC result.
- Worker exec and buffered exec stream honor the provided timeout string as a worker-side context deadline when present.
- Implemented worker-side file read/write/list/delete/move/chmod/stat/glob handling in the inbound RPC server.
- Added typed `RPCClient.Exec` support for control-plane calls.
- Added typed `RPCClient.ExecStream` support for control-plane calls.
- Added typed file RPC client helpers for control-plane calls.

### Control Plane Routing

- `Manager.Exec` now detects remote-owned sandboxes and routes non-streaming exec to the owning worker RPC endpoint.
- `Manager.ExecStream` now detects remote-owned sandboxes and routes buffered exec streams to the owning worker RPC endpoint.
- File APIs now detect remote-owned sandboxes and route through the owning worker RPC endpoint.
- Remote exec uses persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote file APIs use persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote exec keeps the existing event, audit, metrics, timeout, and exec-log behavior.
- Remote exec stream keeps the existing manager channel API, with chunks buffered by the worker RPC response.
- Remote sandboxes no longer inherit pool-mode default workdir or file path scoping just because their provider runtime ID is stored in `VMID`.

## Code Areas

- `internal/workerproto/protocol.go`: `worker.exec`, `worker.exec_stream`, and file RPC contract/result types.
- `internal/worker/rpc.go`: worker-side exec, exec-stream, and file RPC handlers.
- `internal/worker/rpc_client.go`: typed exec, exec-stream, and file RPC client methods.
- `internal/orchestrator/manager.go`: remote-owned sandbox exec, exec-stream, and file routing.
- `internal/worker/rpc_test.go`: worker exec and exec-stream RPC handler coverage.
- `internal/worker/rpc_client_test.go`: typed exec, exec-stream, and file client coverage.
- `internal/orchestrator/manager_test.go`: control-plane remote exec, exec-stream, and file routing coverage.

## Verification

- `go test ./internal/workerproto ./internal/worker ./internal/orchestrator`
- `go test ./...`
- `git diff --check`

## Remaining Phase 12 Direction

- Upgrade remote exec-stream from buffered RPC delivery to true live stream transport.
- Route logs and live previews through remote workers.
- Add drain handoff/reassignment semantics for remote-owned sandboxes.
