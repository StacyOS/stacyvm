# Changelog

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
