# Phase 8 Single-Node Production Release Notes

Date: 2026-05-08
Branch: `phase-8-single-node-production`

## Summary

Phase 8 moves StacyVM from release-candidate hardening toward technical self-hosted production on a single node. The phase focuses on safe SQLite backup/restore workflows, deterministic production config linting, upgrade rehearsal, and redacted support bundles because single-node operators need reliable rollback points and clear support artifacts before upgrades, config changes, and provider certification.

## What Changed

### Database Backup And Restore

- Added `stacyvm db backup <output-path>`.
- Added `stacyvm db restore <backup-path> --yes`.
- Backup uses SQLite `VACUUM INTO` and verifies the backup with `PRAGMA integrity_check`.
- Restore verifies the backup before replacing the target database.
- Restore creates a timestamped `.pre-restore-*` safety copy of the existing database.
- Restore removes stale `-wal` and `-shm` sidecars before replacing the target database.
- Added tests for backup/restore, overwrite protection, integrity validation, safety copy creation, and sidecar cleanup.

### Production Config Linting

- Added `stacyvm config lint`.
- Added `stacyvm config lint --production` to treat production hardening issues as failures.
- Added `--file` support so operators can lint a target config file without relying on the default lookup path.
- Linting checks authentication posture, placeholder secrets, admin key separation, admin fallback, audit retention, rate limiting, database durability, runtime caps, exec timeouts, JSON logging, and Docker hardening.
- Added deterministic lint tests that do not require Docker, KVM, or a running StacyVM server.

### Upgrade Rehearsal

- Added `stacyvm upgrade rehearse`.
- Rehearsal runs production config lint checks and SQLite integrity checks.
- Rehearsal validates the intended backup directory and refuses an already-existing backup output path.
- Rehearsal prints the recommended upgrade and rollback flow for single-node operators.
- Added `--include-doctor` for live host checks when running on the target host.

### Redacted Support Bundle

- Added `stacyvm support bundle <output-path>`.
- Bundle includes version/runtime data, redacted config shape, production config lint output, optional doctor checks, and optional `/api/v1/diagnostics` output.
- Redaction covers secret-like keys, API keys, bearer tokens, token/password/secret assignments, and URLs with embedded credentials.
- Added tests to ensure final support JSON does not leak representative secrets.

### Runtime Certification Artifacts

- Upgraded `scripts/certify-runtime.sh` to emit host certification reports in `text`, `json`, or `markdown`.
- Added `--output` support so Docker, gVisor, Kata, Firecracker, and PRoot host checks can be attached to deployment records.
- Added stricter optional Firecracker and PRoot path checks through `STACYVM_FIRECRACKER_KERNEL`, `STACYVM_PROOT_ROOTFS`, and `STACYVM_PROOT_WORKSPACE_BASE`.
- Documented required Phase 8 signoff artifacts for target infrastructure.

### Documentation

- Updated deployment backup and upgrade guidance to prefer `stacyvm db backup`.
- Updated deployment and release guidance to run `stacyvm config lint --production` before staging, upgrades, and release tags.
- Added backup, config lint, upgrade rehearsal, and support bundle commands to the README command list.
- Updated runtime certification and conformance docs to require host-generated certification artifacts.

## Verification

```sh
go test ./cmd/stacyvm
go test ./...
stacyvm config lint --production --file deploy/stacyvm.production.yaml
```

## Next Phase 8 Direction

Phase 8 is now functionally complete for single-node technical production readiness. Remaining signoff is final cleanup: run the full build/test sweep, confirm GitHub CI, and keep platform-gated Docker/gVisor/Kata/Firecracker/PRoot conformance results attached to host-specific certification.
