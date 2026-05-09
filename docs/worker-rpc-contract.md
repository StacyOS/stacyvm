# Worker RPC Contract

Phase 10 defines the control-plane to worker contract without committing to a network transport. The current implementation keeps execution local, but the data model, placement, leases, and message shapes are now explicit enough for a future worker service.

## Contract Package

The Go contract lives in `internal/workerproto`.

Core envelope:

```json
{
  "id": "req-123",
  "method": "worker.spawn",
  "worker_id": "worker-a",
  "lease": {
    "resource_id": "sb-abc123",
    "holder_id": "worker-a",
    "generation": 4,
    "expires_at": "2026-05-09T10:31:00Z"
  },
  "params": {}
}
```

Supported methods:

| Method | Direction | Lease required | Purpose |
|---|---|---:|---|
| `worker.heartbeat` | worker to control plane | No | Report liveness, providers, capabilities, and capacity. |
| `worker.spawn` | control plane to worker | Yes | Assign sandbox creation to the selected worker. |
| `worker.destroy` | control plane to worker | Yes | Assign sandbox teardown to the owning worker. |
| `worker.status` | control plane to worker | No | Ask a worker for runtime state. |
| `worker.renew_lease` | control plane to worker | Yes | Confirm continued ownership and renew fencing. |
| `worker.shutdown` | control plane to worker | No | Drain or stop a worker process. |

## Lease Fencing

Every mutating lifecycle assignment must include a lease token:

- `resource_id` is the sandbox ID.
- `holder_id` must match the selected worker.
- `generation` is incremented whenever ownership is acquired or renewed.
- `expires_at` defines when another worker may take over.

Workers must reject mutating work if the lease holder does not match their own worker ID or if the lease is expired. The control plane must renew leases before long-running operations cross the expiry window.

## Worker Authentication

Worker identity must be separate from user and admin identity.

Recommended transport headers for the future network worker:

| Header | Purpose |
|---|---|
| `X-Worker-ID` | Stable worker ID that must match the token subject. |
| `Authorization: Bearer <token>` | Worker token signed by the control plane or trusted issuer. |
| `X-Request-ID` | Request correlation across control plane and worker logs. |

Validated worker tokens should produce `workerproto.AuthClaims`:

- `worker_id`
- `scopes`
- `expires`

Initial scopes:

- `worker:heartbeat`
- `worker:spawn`
- `worker:destroy`
- `worker:status`
- `worker:lease`

Workers must not accept user API keys or admin API keys for worker RPC. Control-plane admin access and worker execution access are separate trust boundaries.

## Cluster Store Semantics

SQLite remains suitable for single-node and local development. Enterprise multi-worker mode should use Postgres or another store with equivalent guarantees.

Required lease guarantees:

- Atomic acquire by `resource_id`.
- Acquire succeeds when no lease exists, the lease is expired, or the same holder renews ownership.
- Acquire fails when a different holder owns an unexpired lease.
- Renew succeeds only for the current holder and only before expiry.
- Release succeeds only for the current holder.
- Concurrent acquire attempts must serialize on the lease row.

In Postgres terms, lease acquire should be implemented with a unique key on `resource_id`, transactional upsert semantics, and row-level contention safety. Clock skew must be bounded because expiry is time-based.

## Phase 10 Limits

Phase 10 does not ship a network worker daemon. Remote placement intentionally returns `remote_worker_rpc_unavailable` until transport and execution are implemented. This prevents StacyVM from silently pretending a remote worker can run work before the trust boundary and lease enforcement are wired end to end.
