# Phase 8 Single-Node Production Release Notes

Date: 2026-05-08
Branch: `phase-8-single-node-production`

## Summary

Phase 8 moves StacyVM from release-candidate hardening toward technical self-hosted production on a single node. The first slice focuses on safe SQLite backup and restore workflows because single-node operators need a reliable rollback point before upgrades, config changes, and provider certification.

## What Changed

### Database Backup And Restore

- Added `stacyvm db backup <output-path>`.
- Added `stacyvm db restore <backup-path> --yes`.
- Backup uses SQLite `VACUUM INTO` and verifies the backup with `PRAGMA integrity_check`.
- Restore verifies the backup before replacing the target database.
- Restore creates a timestamped `.pre-restore-*` safety copy of the existing database.
- Restore removes stale `-wal` and `-shm` sidecars before replacing the target database.
- Added tests for backup/restore, overwrite protection, integrity validation, safety copy creation, and sidecar cleanup.

### Documentation

- Updated deployment backup and upgrade guidance to prefer `stacyvm db backup`.
- Added CLI backup command to the README command list.

## Verification

```sh
go test ./cmd/stacyvm
```

## Next Phase 8 Direction

The next Phase 8 slices should add upgrade rehearsal checks, production config linting, and a single-node support bundle with redaction so technical users can operate and debug StacyVM without handholding.
