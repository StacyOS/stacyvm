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
- Public release sanity builds and checksum verification run in CI. Done in Phase 9 final polish.
- SDK parity smoke tests run in CI without requiring a live runtime. Done in Phase 9 final polish.
- GitHub issue templates request support bundle, config lint, upgrade rehearsal, runtime certification, and release verification evidence. Done in Phase 9 final polish.

## Phase 10 Acceptance Criteria

- Worker registration and heartbeat records are stored durably. Done in Phase 10 slice 1.
- Single-node servers self-register as the `local` worker with provider and capacity metadata. Done in Phase 10 slice 1.
- Single-node servers refresh the `local` worker heartbeat while running. Done in Phase 10 heartbeat slice.
- Read-only worker discovery is available through the normal API. Done in Phase 10 slice 1.
- Worker heartbeat and deletion are protected by the admin namespace. Done in Phase 10 slice 1.
- Diagnostics and Prometheus expose worker registry state. Done in Phase 10 slice 1.
- Sandbox records persist their owning worker ID and diagnostics expose sandbox counts by worker. Done in Phase 10 slice 2.
- Scheduler placement policy is worker-aware. Remote spawn, status, destroy, live exec streaming, files, logs, preview metadata, and conservative drain/offline ownership policy are available for workers that advertise `rpc_url`.
- Sandbox ownership is tied to worker IDs. Remote spawn/status/destroy ownership is enforced through worker RPC and persisted runtime IDs.
- Distributed leases prevent duplicate worker ownership. Remote spawn, renew, and destroy now carry lease tokens; persistence now has SQLite and Postgres store paths with Postgres lease race coverage.
- Remote worker authentication and RPC contract are implemented for heartbeat, lease renewal, spawn, status, destroy, exec, files, logs, preview metadata, and drain/offline ownership reconciliation. Shared worker tokens remain available for staging, and per-worker token mapping now supports individually rotatable worker credentials.

## Current Release-Candidate Gates

| Gate | Status | Notes |
|---|---|---|
| Full Go test suite | Passing | CI runs `make test`. |
| Web build | Passing | CI runs `npm run build`. |
| SDK checks | Passing | TypeScript builds, Python imports, and mock-based SDK parity smoke tests run in CI. |
| Deployment smoke | Passing | Mock-provider smoke is in CI. Docker live host certification remains external. |
| Cluster conformance | Partial | Always-on CI covers SQLite store contract, live Postgres store contract, Postgres migration rehearsal, Postgres lease concurrency, per-worker identity, production cluster config lint, and Postgres-backed remote worker smoke. See `docs/cluster-conformance.md`. |
| Runtime conformance | Partial | Harness and host certification script exist; Firecracker/PRoot remain platform-gated. |
| Security posture | Partial | Admin governance, operation audit, path traversal checks, and explicit exec modes are implemented; OIDC/JWT implementation remains. |
| Release automation | Passing | Release workflow signs binaries, checksums, and GHCR image digests; public verifier and installer verification exist. |
| Worker registry | Partial | Durable worker registration, heartbeat, diagnostics, metrics, placement, ownership, leases, per-worker token auth, and worker RPC contract exist; network worker transport remains. |

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
- Bug and production support issue templates ask for the same evidence required by the public support matrix.
- Public release sanity CI builds release binaries and validates checksums; real GitHub release asset verification must be repeated after each version tag is published.

## Required Before Enterprise/Multi-Worker

- Postgres store implementation. Driver, migrations, contract path, migration rehearsal, lease race coverage, and mock-provider remote worker smoke exist; production distributed mode still needs backup/restore-specific migration rehearsal.
- Worker registration and heartbeat model. Durable registry and per-worker token auth exist; production distributed mode still needs signed-token or mTLS hardening for public/enterprise deployments.
- Scheduler abstraction with placement policy.
- Durable queue/pub-sub for lifecycle events.
- Distributed leases to prevent double ownership.
- OIDC/SSO and RBAC implemented, not only designed.
- Worker RPC transport must enforce [worker-rpc-contract.md](worker-rpc-contract.md).
