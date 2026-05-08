# Phase 9 Public Self-Serve Release Trust Release Notes

Date: 2026-05-08
Branch: `phase-9-public-self-serve-release-trust`

## Summary

Phase 9 starts the public self-serve readiness track. This first slice focuses on release trust: users installing StacyVM from GitHub or GHCR should be able to verify that binaries, checksums, and container images came from the StacyVM release workflow.

## What Changed

### Signed Release Artifacts

- Release binaries are signed with Sigstore keyless signing.
- `checksums.txt` is signed with Sigstore keyless signing.
- The release workflow publishes `.sig` and `.pem` files next to each signed artifact.
- The GHCR image digest is signed after the multi-arch image is published.

### Public Verification

- Added `scripts/verify-release.sh <version> [amd64|arm64]`.
- The verifier downloads release binaries, checksums, signatures, and certificates.
- The verifier checks the expected GitHub Actions OIDC issuer and StacyVM release workflow identity.
- The verifier runs SHA-256 checksum verification after signature verification succeeds.

### Installer Hardening

- `scripts/install.sh` verifies Sigstore signatures automatically when `cosign` is installed.
- `STACYVM_REQUIRE_SIGNATURES=true` makes the installer fail closed when `cosign` is unavailable.
- The installer still verifies SHA-256 checksums.

### Documentation

- Added release verification instructions to the README.
- Expanded release documentation with binary, checksum, and container image verification commands.
- Added Phase 9 acceptance criteria to the production readiness checklist.
- Added a public self-serve support and limitations matrix.

### Upgrade And Migration CI

- Added `scripts/ci-upgrade-migration.sh`.
- Added a CI job that runs focused config, upgrade rehearsal, and SQLite migration checks.
- Added coverage for migrating a legacy v1 SQLite database through the current schema.
- Made Docker integration tests opt-in with `STACYVM_DOCKER_INTEGRATION=1` so default CI is not coupled to Docker Hub availability or runner daemon state.

### Public Release Sanity

- Added `scripts/ci-public-release-sanity.sh`.
- CI now syntax-checks public install and verification scripts.
- CI builds release binaries for supported architectures and verifies `checksums.txt`.
- Real GitHub release asset verification remains a required post-tag drill for each published version.

### Diagnostics Remediation

- Added remediation links to `/api/v1/diagnostics`.
- Diagnostics now point operators to production readiness, deployment, runtime certification, runtime conformance, release verification, support bundle, and security governance docs.

### SDK Parity

- Added mock-based TypeScript and Python SDK parity smoke tests.
- TypeScript spawn options now include `template`, matching Python spawn behavior.
- Python SDK now exposes `templates` and `providers()` helpers for closer TypeScript parity.

### Support Intake

- Added GitHub issue forms for bug reports and production support requests.
- Issue templates ask for support bundle, config lint, upgrade rehearsal, runtime certification, release verification, environment, and logs.

## Verification

```sh
bash -n scripts/install.sh
bash -n scripts/verify-release.sh
bash -n scripts/ci-upgrade-migration.sh
scripts/ci-upgrade-migration.sh
scripts/ci-public-release-sanity.sh
bun test
python -m unittest sdk/python/tests/test_client_parity.py
git diff --check
go test ./...
```

## Remaining Phase 9 Direction

Phase 9 is now complete from a branch-readiness perspective. The only release-time follow-up is to run `scripts/verify-release.sh` and the installer against the actual GitHub assets after the next real version tag is published.
