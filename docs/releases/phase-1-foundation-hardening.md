# Phase 1 Foundation Hardening Release Notes

Date: 2026-05-08
Branch: `feat/phase-1-foundation-hardening`
Commit: `194267e`

## Summary

Phase 1 focused on turning StacyVM's early provider/orchestrator foundation into a more production-ready base. The work improves error consistency, provider contracts, startup recovery, platform-aware conformance coverage, and local developer build reliability.

This phase does not introduce a new user-facing sandbox feature. Instead, it strengthens the foundation that future phases will build on: predictable errors, safer reconciliation after restarts, clearer provider expectations, and stronger regression coverage.

## What Changed

### Provider Contract And Conformance

- Added `docs/provider-contract.md` to document the required behavior for every sandbox provider.
- Added a reusable provider conformance test harness covering:
  - spawn, status, and destroy lifecycle
  - command execution success and non-zero exits
  - streaming command output
  - file write, read, stat, glob, move, chmod, and delete
- Wired conformance coverage for Mock, Docker, Custom, PRoot, and Firecracker providers.
- PRoot and Firecracker conformance tests are platform-gated so they skip locally unless the required runtime dependencies are available.

### Typed Error Taxonomy

- Added typed provider errors for:
  - sandbox not found
  - sandbox destroyed
  - provider not found
  - provider unavailable
  - exec timeout
  - resource limit
- Added typed store errors for:
  - not found
  - conflict
- Re-exported provider errors through the orchestrator package where API routes need stable domain-level matching.

### API Error Handling

- Added shared route error mapping in `internal/api/routes/errors.go`.
- Replaced string-matching error handling in sandbox, template, environment, and provider routes with typed error checks.
- Added response codes for timeout and resource-limit failures.
- API responses now map important failure classes consistently:
  - `404` for missing resources and sandbox lifecycle misses
  - `408` for exec timeout
  - `429` for resource limits
  - `503` for provider unavailability

### Startup Reconciliation

- Added `Manager.Reconcile(ctx)` to refresh persisted sandbox state from provider runtime state at server startup.
- Server startup now runs reconciliation before starting the manager reaper.
- Persisted sandboxes whose runtime no longer exists are marked `destroyed`.
- Persisted sandboxes whose provider is unavailable are marked `error`.
- Live provider runtimes can be restored into the manager's in-memory cache.

### Docker Runtime Adoption

- Docker sandboxes now include richer `stacyvm.*` labels for runtime discovery.
- Added Docker runtime inventory support through `ListRuntimeSandboxes`.
- Startup reconciliation can adopt StacyVM Docker containers that still exist but are missing from SQLite after a process restart.
- Docker missing-container cases now map to typed `ErrSandboxNotFound`.

### Streaming Timeout Semantics

- `Manager.ExecStream` now honors request-level timeout values.
- Streaming timeout paths emit an explicit stderr timeout chunk instead of silently closing.
- Docker, Custom, and Firecracker streaming paths now propagate timeout state more clearly.

### macOS Build Reliability

- The Linux-only `stacyvm-agent` entrypoint now has a Linux build tag.
- Added a non-Linux stub so `make build` and `make test` work on macOS while preserving the real Linux agent behavior.

## Code Changes By Area

### New Files

- `CHANGELOG.md`
- `cmd/stacyvm-agent/main_unsupported.go`
- `docs/provider-contract.md`
- `internal/api/routes/errors.go`
- `internal/api/routes/templates_test.go`
- `internal/orchestrator/errors.go`
- `internal/providers/custom_conformance_test.go`
- `internal/providers/errors.go`
- `internal/providers/provider_conformance_test.go`
- `internal/store/errors.go`

### Core Orchestrator

- `internal/orchestrator/manager.go`
  - Added startup reconciliation.
  - Added provider runtime adoption.
  - Added streaming timeout handling.
- `internal/orchestrator/manager_test.go`
  - Added tests for reconciliation, runtime adoption, and streaming timeout behavior.
- `cmd/stacyvm/cmd_serve.go`
  - Runs reconciliation during server startup.

### Providers

- `internal/providers/provider.go`
  - Documented provider contract expectations.
  - Added optional runtime inventory interfaces.
- `internal/providers/docker.go`
  - Added StacyVM labels.
  - Added runtime listing.
  - Added typed not-found handling.
  - Improved streaming timeout propagation.
- `internal/providers/custom.go`
  - Added typed HTTP error mapping.
  - Improved streaming timeout propagation.
- `internal/providers/firecracker.go`
  - Added typed lifecycle behavior and streaming read deadlines.
- `internal/providers/proot.go`
  - Added typed lifecycle, timeout, and resource-limit behavior.
- `internal/providers/mock.go`
  - Added typed lifecycle and timeout behavior for tests.
- `internal/providers/registry.go`
  - Uses typed provider-not-found errors.

### Store

- `internal/store/sqlite.go`
  - Maps missing rows and constraint conflicts to typed store errors.
- `internal/store/sqlite_test.go`
  - Adds coverage for typed store errors.

### API Routes

- `internal/api/routes/sandboxes.go`
- `internal/api/routes/templates.go`
- `internal/api/routes/environments.go`
- `internal/api/routes/providers.go`

These routes now use shared typed error handling instead of string comparisons.

## Verification

The following checks passed:

```sh
make test
make build
cd web && npm run build
```

Additional Docker provider conformance and runtime inventory checks passed with Docker daemon access.

## Platform Notes

- Firecracker conformance requires Linux, `/dev/kvm`, Firecracker, kernel, rootfs, and agent paths.
- PRoot conformance requires `proot` and a usable rootfs.
- The full Go integration suite uses `httptest`; local sandboxed runs need permission to bind local test sockets.

## Impact

Phase 1 leaves the codebase ready for Phase 2 work by making provider behavior explicit, testable, and recoverable. The next phase can focus on scalability and production operations without first untangling provider lifecycle ambiguity or inconsistent API failure behavior.
