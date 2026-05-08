# Production Readiness Checklist

This checklist tracks the Phase 7 release-candidate hardening work needed before StacyVM is marketed as production-ready.

## Readiness Levels

| Level | Target user | Current gate |
|---|---|---|
| Internal staging | StacyOS team and trusted operators | `stacyvm doctor`, CI, mock deployment smoke, documented rollback |
| Single-node production | Technical self-hosters | Docker/gVisor or Firecracker conformance, hardened auth, backup/restore drill |
| Public self-serve | Users without handholding | Signed releases, upgrade tests, support bundle, clear failure remediation |
| Enterprise/multi-worker | Infrastructure teams | Postgres, workers, durable scheduler, leases, OIDC/RBAC |

## Phase 7 Acceptance Criteria

- `stacyvm doctor` reports actionable local and production diagnostics.
- Docker command execution has explicit shell and argv semantics. Done in Phase 7 slice 2.
- File APIs have path traversal tests across manager scoping and provider boundaries. Done in Phase 7 final cleanup.
- Sensitive operations are covered by persisted operation audit records. Done in Phase 7 final cleanup.
- Runtime certification scripts exist for Docker, gVisor, Kata, Firecracker, and PRoot host checks. Done in Phase 7 final cleanup.
- Threat model is documented for runtime, API, admin, live-preview, pool, and registry surfaces.
- Release notes describe verified CI and known platform caveats.

## Phase 8 Acceptance Criteria

- SQLite backup and restore are available through the CLI with integrity checks and restore safety copies. Done in Phase 8 slice 1.
- Production config linting is available through `stacyvm config lint --production` and can run against explicit config files without requiring Docker/KVM host access. Done in Phase 8 slice 2.
- Upgrade rehearsal checks document backup, config lint, service restart, readiness validation, and rollback. Done in Phase 8 slice 3.
- Support bundle export exists and redacts secrets before sharing with maintainers. Done in Phase 8 slice 3.

## Phase 9 Acceptance Criteria

- Release binaries and checksums are signed through the GitHub Actions release workflow. Done in Phase 9 slice 1.
- Published container image digests are signed through the GitHub Actions release workflow. Done in Phase 9 slice 1.
- A public verification script exists for release signatures and checksums. Done in Phase 9 slice 1.
- Installer supports Sigstore verification and a fail-closed mode. Done in Phase 9 slice 1.
- Upgrade and config migration tests run in CI. Done in Phase 9 slice 2.
- Public docs expose known limitations and exact remediation paths. Done in Phase 9 slice 2.

## Current Release-Candidate Gates

| Gate | Status | Notes |
|---|---|---|
| Full Go test suite | Passing | CI runs `make test`. |
| Web build | Passing | CI runs `npm run build`. |
| SDK checks | Partial | TypeScript builds and Python imports; full SDK behavioral parity tests still need expansion. |
| Deployment smoke | Passing | Mock-provider smoke is in CI. Docker live host certification remains external. |
| Runtime conformance | Partial | Harness and host certification script exist; Firecracker/PRoot remain platform-gated. |
| Security posture | Partial | Admin governance, operation audit, path traversal checks, and explicit exec modes are implemented; OIDC/JWT implementation remains. |
| Release automation | Passing | Release workflow signs binaries, checksums, and GHCR image digests; public verifier and installer verification exist. |

## Required Before Single-Node Production

- Production config uses distinct API and admin keys.
- `auth.admin_fallback_enabled` is `false`.
- `auth.admin_audit_retention` is set to a production window.
- Docker provider runs with explicit runtime, network mode, dropped caps, pid limit, memory, CPU, and seccomp settings.
- Firecracker hosts pass Linux/KVM conformance before being marked production.
- Backup and restore are tested against the SQLite database.
- `stacyvm config lint --production` passes with the same config and environment variables the service will use.
- `stacyvm upgrade rehearse` passes before binary/image replacement.
- Operators can generate `stacyvm support bundle` output without exposing API keys or provider secrets.
- Runtime certification artifacts are generated on the actual host with `scripts/certify-runtime.sh <runtime> --format markdown --output <runtime>-certification.md`.
- Operators run `stacyvm doctor --production` before go-live.

## Required Before Public Self-Serve

- Release artifacts are signed and checksummed.
- Upgrade and config migration tests run in CI.
- `stacyvm doctor` includes remediation links for every failure.
- Support bundle export exists and redacts secrets.
- Threat model is reviewed for each release candidate.
- Known limitations are visible in README, docs, and release notes.
- Public support expectations are documented in [public-support-matrix.md](public-support-matrix.md).

## Required Before Enterprise/Multi-Worker

- Postgres store implementation.
- Worker registration and heartbeat model.
- Scheduler abstraction with placement policy.
- Durable queue/pub-sub for lifecycle events.
- Distributed leases to prevent double ownership.
- OIDC/SSO and RBAC implemented, not only designed.
