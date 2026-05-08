# Phase 8 Single-Node Production Release Notes

Date: 2026-05-08
Branch: `phase-8-single-node-production`

## Summary

Phase 8 moves StacyVM from release-candidate hardening toward technical self-hosted production on a single node. The first slices focus on safe SQLite backup/restore workflows and deterministic production config linting because single-node operators need a reliable rollback point and a clear preflight gate before upgrades, config changes, and provider certification.

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

### Documentation

- Updated deployment backup and upgrade guidance to prefer `stacyvm db backup`.
- Updated deployment and release guidance to run `stacyvm config lint --production` before staging, upgrades, and release tags.
- Added CLI backup command to the README command list.

## Verification

```sh
go test ./cmd/stacyvm
```

## Next Phase 8 Direction

The next Phase 8 slices should add upgrade rehearsal checks and a single-node support bundle with redaction so technical users can operate and debug StacyVM without handholding.
