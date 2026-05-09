# Cluster Conformance Matrix

This matrix defines the minimum checks StacyVM must pass before a branch is considered production-aligned for multi-worker operation. It is intentionally stricter than the single-node deployment smoke tests because cluster mode depends on durable ownership, worker identity, leases, and store behavior remaining consistent across processes.

## CI Coverage

The always-on CI entrypoint is:

```bash
scripts/ci-cluster-conformance.sh
```

It currently verifies:

- SQLite passes the reusable store contract harness.
- Worker route authentication accepts per-worker credentials.
- Worker-specific credentials override the shared staging token.
- Worker route authentication accepts short-lived signed worker tokens.
- Worker route authentication rejects signed tokens scoped to the worker RPC audience.
- Worker route authentication rejects revoked signed worker token IDs.
- Worker RPC accepts short-lived signed control-plane-to-worker tokens.
- Worker RPC rejects signed tokens scoped to the control-plane route audience.
- Worker RPC rejects revoked signed worker token IDs.
- Remote spawn can route through worker RPC using signed tokens without a shared worker token.
- Worker RPC mTLS completes a real client-authenticated request using generated certificates.
- Worker lease renewal is guarded by `worker:lease`.
- A production-aligned cluster config with `auth.worker_tokens` or `auth.worker_signing_key` passes `stacyvm config lint --production`.
- Postgres configuration with a valid DSN passes `stacyvm config lint --production`.
- Live Postgres passes the reusable store contract when `STACYVM_POSTGRES_TEST_DSN` is set.
- Live Postgres proves one active lease holder under concurrent acquire and expired takeover attempts.
- Live Postgres proves migrations apply idempotently and record every expected schema version.
- A Postgres-backed remote worker smoke runs control plane plus worker against the mock provider.

## Store Matrix

| Store | Status | Required Checks |
|---|---|---|
| SQLite | Supported for single-node and internal staging | `TestSQLiteStoreContract`, migration tests, backup/restore tests |
| Postgres | Contract-backed cluster store path | `TestPostgresStoreContract`, `TestPostgresMigrationRehearsal`, Postgres migration alignment tests, lease takeover race tests, remote worker smoke, startup reconciliation with multiple workers |

Postgres must not be marked production-ready for a deployment until it runs the same store contract suite as SQLite, passes lease race coverage, and passes the remote worker smoke in that deployment's target topology.

## Worker Identity Matrix

| Mode | Status | Intended Use |
|---|---|---|
| `auth.worker_token` | Supported | Local development and internal staging with a shared worker token |
| `auth.worker_tokens.<worker_id>` | Supported | Production-aligned staging with individually rotatable worker credentials |
| `auth.worker_signing_key` | Supported | Public or enterprise deployments that need short-lived signed worker credentials |
| `auth.worker_signing_keys` | Supported | No-downtime signing-key rotation window for old verification keys |
| Worker RPC mTLS | Supported | Enterprise deployments that require network-level worker transport identity |

When `auth.worker_tokens` contains a worker ID, that worker must authenticate with its own token. The shared token is rejected for that worker ID.

Signed worker tokens must use the `stacyvm-worker-v1` HMAC-SHA256 format. The signed subject must match `X-Worker-ID`, the `exp` claim must be in the future, and only worker scopes are granted.

## Runtime Matrix

| Runtime | Cluster Status | Notes |
|---|---|---|
| Mock | CI certified | Used for fast worker routing and control-plane smoke tests |
| Docker | Host-certified | Requires Docker daemon access and runtime certification outside the sandboxed CI path |
| gVisor/Kata | Host-certified | Requires configured Docker runtime and host-level certification |
| Firecracker | Platform-gated | Requires Linux/KVM, kernel, rootfs, and agent assets |
| PRoot | Platform-gated | Requires real rootfs/bin setup on the host |

## Promotion Gates

Before calling a multi-worker branch production-ready:

1. `scripts/ci-cluster-conformance.sh` passes in CI.
2. `scripts/smoke-remote-worker.sh` passes against a real control-plane plus worker pair.
3. Runtime certification passes for every runtime advertised by the deployment.
4. Postgres passes the store contract harness.
5. Postgres lease tests prove one active holder per sandbox under concurrent acquisition, renewal, expiry, and takeover.
6. Startup reconciliation is tested against persisted sandboxes whose owning worker is online, stale, draining, offline, and missing.
7. Worker credentials are per-worker, rotated, and not shared with user or admin API credentials.

## Current Phase 13 Position

Phase 13 has completed:

- Driver-based store selection.
- SQLite store contract coverage.
- Postgres-native migration definitions.
- Postgres store driver and live contract path.
- Per-worker token authentication.
- Cluster conformance CI scaffolding.

Phase 14 starts worker identity hardening on top of that foundation:

- HMAC-signed worker tokens.
- Signed-token config lint awareness.
- Worker runtime token derivation for heartbeat and lease renewal.
- Signed control-plane-to-worker RPC token derivation for remote worker calls.
- Worker RPC mTLS config, transport wiring, and production lint checks.
- Worker RPC mTLS conformance using generated CA, server, and client certificates.

Remaining cluster-storage and identity work after Phase 14:

- Extend multi-worker conformance beyond the mock provider into certified Docker, gVisor/Kata, and Firecracker hosts.
- Add backup/restore-specific Postgres migration rehearsal before enterprise production rollout.
- Add token issuer and rotation workflows so workers do not need direct access to the signing key in hardened deployments.
- Run worker RPC mTLS smoke tests with deployment-issued certificates in the target enterprise network.
