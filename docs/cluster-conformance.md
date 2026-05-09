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
- Worker lease renewal is guarded by `worker:lease`.
- A production-aligned cluster config with `auth.worker_tokens` passes `stacyvm config lint --production`.
- Postgres configuration with a valid DSN passes `stacyvm config lint --production`.
- Live Postgres passes the reusable store contract when `STACYVM_POSTGRES_TEST_DSN` is set.

## Store Matrix

| Store | Status | Required Checks |
|---|---|---|
| SQLite | Supported for single-node and internal staging | `TestSQLiteStoreContract`, migration tests, backup/restore tests |
| Postgres | Contract-backed cluster store path | `TestPostgresStoreContract`, Postgres migration alignment tests, migration upgrade/downgrade rehearsal, lease takeover race tests, startup reconciliation with multiple workers |

Postgres must not be marked production-ready until it runs the same store contract suite as SQLite and adds database-level lease semantics that are safe across multiple control-plane or worker processes.

## Worker Identity Matrix

| Mode | Status | Intended Use |
|---|---|---|
| `auth.worker_token` | Supported | Local development and internal staging with a shared worker token |
| `auth.worker_tokens.<worker_id>` | Supported | Production-aligned staging with individually rotatable worker credentials |
| Signed worker tokens or mTLS | Planned | Public multi-worker and enterprise deployments |

When `auth.worker_tokens` contains a worker ID, that worker must authenticate with its own token. The shared token is rejected for that worker ID.

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

Remaining cluster-storage work:

- Add Postgres lease race/concurrency tests.
- Add a real multi-worker conformance environment that runs control plane, worker, and Postgres together.
