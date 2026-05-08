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

## Verification

```sh
bash -n scripts/install.sh
bash -n scripts/verify-release.sh
git diff --check
go test ./...
```

## Next Phase 9 Direction

The next Phase 9 slices should add upgrade/config migration tests in CI, remediation links for diagnostics, and a public limitations page that clearly separates supported self-serve modes from host-gated runtime certification.
