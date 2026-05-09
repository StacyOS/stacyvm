# Worker RPC Contract

Phase 10 defined the control-plane to worker contract. Phase 11 wired that contract into a real worker runtime: `stacyvm worker` can authenticate to the control plane, submit heartbeat state through a worker-only HTTP endpoint, and expose an optional inbound RPC server with `--listen`.

Phase 12 starts remote sandbox I/O routing. Remote spawn, status, destroy, lease renewal, shutdown/drain, and non-streaming exec now use the worker RPC transport when the scheduler selects a non-local worker that advertises `rpc_url`.

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
| `worker.exec` | control plane to worker | No | Run a non-streaming command in an owned runtime. |
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
| `X-Worker-Token` or `Authorization: Bearer <token>` | Worker token signed by the control plane or trusted issuer. |
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
- `worker:exec`
- `worker:lease`

Workers must not accept user API keys or admin API keys for worker RPC. Control-plane admin access and worker execution access are separate trust boundaries.

Current Phase 11 heartbeat endpoint:

```text
POST /api/v1/worker/{workerID}/heartbeat
```

The endpoint requires `X-Worker-ID` plus `X-Worker-Token` and rejects requests where the authenticated worker ID differs from the `{workerID}` path.

Current Phase 11 worker RPC endpoint:

```text
POST /rpc
```

Run it with:

```bash
stacyvm worker --listen 127.0.0.1:7430
```

The endpoint accepts `workerproto.Request` envelopes, requires the same worker headers, and currently implements `worker.status`, `worker.exec`, `worker.renew_lease`, `worker.spawn`, `worker.destroy`, and `worker.shutdown`.

For `worker.spawn`, the request carries a control-plane `sandbox_id` and the response returns both that ID and the provider `runtime_id`. The control plane should persist that mapping before routing later status, exec, file, or destroy operations to the owning worker.

Remote workers advertise their control-plane callback endpoint through heartbeat capacity:

```json
{
  "capacity": {
    "max_sandboxes": 10,
    "rpc_url": "http://worker-a.internal:7430"
  }
}
```

When the scheduler selects a non-local worker with `rpc_url` and `auth.worker_token` is configured, the control plane acquires the sandbox lease for that worker, calls `worker.spawn`, persists the selected `worker_id`, and stores the returned provider runtime ID for later routing.

Sandbox reads use the persisted `worker_id` and provider `runtime_id` to call `worker.status` on the owning worker. If the worker reports a changed state, the control plane updates its stored sandbox state. If the worker is temporarily unreachable, the control plane keeps serving the cached record and logs the refresh failure at debug level.

Remote destroy uses the same persisted ownership tuple. The control plane fetches the durable sandbox lease, presents it to `worker.destroy`, updates sandbox state to `destroyed`, and releases the lease after the worker confirms teardown.

Remote non-streaming exec uses the same persisted ownership tuple without acquiring a new lifecycle lease. The control plane sends command, argv mode, environment, workdir, timeout, provider, sandbox ID, and provider runtime ID to `worker.exec`. The worker runs the command against its local provider registry and returns exit code, stdout, and stderr. The control plane still writes normal exec logs and emits the same audit, event, metric, and timeout behavior used by local exec.

`worker.shutdown` is a drain signal. After receiving it, the worker rejects new `worker.spawn` assignments and reports `draining` in future heartbeats, which keeps it out of scheduler placement. Existing sandboxes are not reassigned automatically in Phase 11.

Current Phase 11 control-plane lease renewal endpoint:

```text
POST /api/v1/worker/{workerID}/leases/{resourceID}/renew
```

The worker RPC handler validates the presented lease token before calling this endpoint. The control plane only renews unexpired leases held by the authenticated worker.

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

## Current Limits

Remote placement returns `remote_worker_rpc_unavailable` unless the selected worker advertises `rpc_url` and `auth.worker_token` is configured. Remote streaming exec, file APIs, logs, previews, Postgres-backed cluster storage, and production worker identity are still outside the current transport.
