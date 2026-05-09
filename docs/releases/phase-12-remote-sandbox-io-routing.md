# Phase 12 Remote Sandbox I/O Routing Release Notes

Date: 2026-05-09
Branch: `phase-12-remote-sandbox-io-routing`

## Summary

Phase 12 starts extending the Phase 11 remote worker runtime beyond lifecycle operations. This checkpoint adds non-streaming exec routing for remote-owned sandboxes, so the control plane can run commands through the owning worker instead of trying to execute against the local provider.

This is the first remote sandbox I/O slice. Streaming exec, file APIs, logs, previews, and production-grade drain handoff remain future Phase 12 work.

## What Changed

### Worker RPC Contract

- Added the `worker.exec` method to the worker RPC contract.
- Added `workerproto.ExecParams` for command, argv mode, environment, workdir, timeout, provider, sandbox ID, and provider runtime ID.
- Added `workerproto.ExecResult` for exit code, stdout, and stderr.
- Added the `worker:exec` scope constant for future token-scoped worker identity.

### Worker Runtime

- Implemented worker-side `worker.exec` handling in the inbound RPC server.
- Worker exec resolves the provider runtime ID, runs through the worker's provider registry, and returns a typed RPC result.
- Worker exec honors the provided timeout string as a worker-side context deadline when present.
- Added typed `RPCClient.Exec` support for control-plane calls.

### Control Plane Routing

- `Manager.Exec` now detects remote-owned sandboxes and routes non-streaming exec to the owning worker RPC endpoint.
- Remote exec uses persisted `worker_id` and provider `runtime_id` instead of local provider state.
- Remote exec keeps the existing event, audit, metrics, timeout, and exec-log behavior.
- Remote sandboxes no longer inherit the pool-mode default workdir just because their provider runtime ID is stored in `VMID`.

## Code Areas

- `internal/workerproto/protocol.go`: `worker.exec` contract and result types.
- `internal/worker/rpc.go`: worker-side exec RPC handler.
- `internal/worker/rpc_client.go`: typed exec RPC client method.
- `internal/orchestrator/manager.go`: remote-owned sandbox exec routing.
- `internal/worker/rpc_test.go`: worker exec RPC handler coverage.
- `internal/worker/rpc_client_test.go`: typed exec client coverage.
- `internal/orchestrator/manager_test.go`: control-plane remote exec routing coverage.

## Verification

- `go test ./internal/workerproto ./internal/worker ./internal/orchestrator`
- `go test ./...`
- `git diff --check`

## Remaining Phase 12 Direction

- Route streaming exec through the owning worker.
- Route file read/write/list/delete/move/chmod/stat/glob APIs through the owning worker.
- Route logs and live previews through remote workers.
- Add drain handoff/reassignment semantics for remote-owned sandboxes.
