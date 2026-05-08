# Phase 7 Release Candidate Hardening Release Notes

Date: 2026-05-08
Branch: `phase-7-release-candidate-hardening`

## Summary

Phase 7 starts the release-candidate hardening track. The goal is to move StacyVM from production-oriented foundations toward a single-node release candidate that operators can validate before trusting it with real workloads.

## What Changed

### Doctor Command

- Added `stacyvm doctor`.
- Added `stacyvm doctor --production` for stricter production posture checks.
- Initial diagnostics cover:
  - config loading
  - API key posture
  - admin key and admin fallback posture
  - database path persistence
  - Docker CLI and daemon availability
  - Docker network and capability settings
  - Firecracker binary, `/dev/kvm`, kernel, and agent paths
  - PRoot binary, rootfs, and workspace base paths

### Production Readiness

- Added [production-readiness.md](../production-readiness.md).
- Documented readiness levels for internal staging, single-node production, public self-serve, and enterprise/multi-worker operation.
- Added Phase 7 acceptance criteria and release-candidate gates.

### Threat Model

- Added [threat-model.md](../threat-model.md).
- Documented assets, trust boundaries, primary threats, current mitigations, and Phase 7 security objectives.

## Verification

```sh
go test ./cmd/stacyvm
go test ./...
npm run build
```

## Next Phase 7 Direction

The next slices should harden Docker command execution semantics, expand file path traversal coverage, and add persisted audit coverage for sandbox lifecycle and file/exec operations.
