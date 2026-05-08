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

### Exec Semantics

- Added explicit exec modes:
  - `shell` preserves the existing `/bin/sh -c` behavior.
  - `argv` runs direct process arguments without shell interpolation.
- Updated Docker, mock, PRoot, Firecracker agent protocol, custom provider passthrough, and CLI exec handling.
- Made `stacyvm exec` use argv mode by default, with `--shell` available for shell expressions.
- Added tests that verify argv payloads are treated literally.

### Final Hardening

- Added persisted operation audit records for sandbox lifecycle, exec, and file operations.
- Tightened pooled file path traversal behavior to reject workspace escapes instead of silently clamping paths.
- Expanded traversal tests across write, read, list, delete, move, chmod, stat, and glob operations.
- Added `scripts/certify-runtime.sh` and [runtime-certification.md](../runtime-certification.md) for Docker, gVisor/Kata, Firecracker, and PRoot host checks.
- Added remediation guidance to failing and warning `stacyvm doctor` checks.

## Verification

```sh
go test ./cmd/stacyvm
go test ./internal/providers
go test ./...
npm run build
scripts/check-swagger.sh
```

## Phase 7 Completion Status

Phase 7 is complete from a codebase and CI perspective. Remaining production signoff is host-gated: run `stacyvm doctor --production`, `scripts/certify-runtime.sh`, and live provider conformance on the actual Linux/Docker/KVM/PRoot hosts selected for release.
