# Worker RPC Contract

Phase 10 defined the control-plane to worker contract. Phase 11 wired that contract into a real worker runtime: `stacyvm worker` can authenticate to the control plane, submit heartbeat state through a worker-only HTTP endpoint, and expose an optional inbound RPC server with `--listen`.

Phase 12 starts remote sandbox I/O routing. Remote spawn, status, destroy, lease renewal, shutdown/drain, non-streaming exec, live exec-stream calls, file APIs, and console logs now use the worker RPC transport when the scheduler selects a non-local worker that advertises `rpc_url`.

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
| `worker.exec_stream` | control plane to worker | No | Run a command and stream stdout/stderr chunks. |
| `worker.file_write` | control plane to worker | No | Write file content in an owned runtime. |
| `worker.file_read` | control plane to worker | No | Read file content from an owned runtime. |
| `worker.file_list` | control plane to worker | No | List files in an owned runtime. |
| `worker.file_delete` | control plane to worker | No | Delete a file or directory in an owned runtime. |
| `worker.file_move` | control plane to worker | No | Move or rename a file in an owned runtime. |
| `worker.file_chmod` | control plane to worker | No | Change file mode in an owned runtime. |
| `worker.file_stat` | control plane to worker | No | Stat a file in an owned runtime. |
| `worker.file_glob` | control plane to worker | No | Evaluate a glob pattern in an owned runtime. |
| `worker.logs` | control plane to worker | No | Return console log lines from an owned runtime. |
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
- `worker:files`
- `worker:logs`
- `worker:lease`

Workers must not accept user API keys or admin API keys for worker RPC. Control-plane admin access and worker execution access are separate trust boundaries.

Current control-plane worker authentication accepts either:

- `auth.worker_token` as a shared staging token.
- `auth.worker_tokens.<worker_id>` as a per-worker token map for production-aligned staging.
- `auth.worker_signing_key` for HMAC-SHA256 signed worker tokens using the `stacyvm-worker-v1.<payload>.<signature>` format.
- `auth.worker_signing_keys` as additional verification keys accepted during signing-key rotation.

When a worker has an entry in `auth.worker_tokens`, that worker-specific token takes precedence and the shared token is rejected for that worker ID. This keeps legacy staging configs compatible while giving production deployments individually rotatable worker credentials.

Signed worker token payloads are base64url JSON claims with `worker_id`, `aud`, optional worker scopes, `iat`, and `exp`. The authenticated `X-Worker-ID` must match the signed `worker_id`, expired tokens are rejected, and any non-worker scopes are ignored. `stacyvm worker` can derive short-lived heartbeat and lease-renewal tokens with `aud=worker:control-plane` from `auth.worker_signing_key` when no static `--worker-token` or `auth.worker_token` is provided. Operators can also issue a token explicitly with `stacyvm worker token <worker-id> --ttl 5m`.

The same signed token format is accepted by worker RPC servers for control-plane-to-worker calls, but RPC tokens use `aud=worker:rpc`. When the control plane has `auth.worker_signing_key` and no shared `auth.worker_token`, it mints short-lived RPC-audience tokens for the target worker before calling `/rpc`. This lets remote spawn, status, exec, file, log, preview, and destroy routing avoid static shared worker RPC credentials.

No-downtime signing-key rotation uses a two-key window:

1. Set the new key as `auth.worker_signing_key`.
2. Move the previous key into `auth.worker_signing_keys`.
3. Restart or reload workers so they mint with the new key.
4. Wait until all old worker tokens have expired.
5. Remove the old key from `auth.worker_signing_keys`.

## Worker RPC mTLS

Signed worker tokens authenticate the worker identity at the application layer. Enterprise deployments should also protect worker RPC transport with mTLS when worker RPC crosses a host or network boundary.

Worker RPC TLS is opt-in through `worker.rpc_tls`:

```yaml
worker:
  listen_addr: "0.0.0.0:7430"
  rpc_tls:
    enabled: true
    server_cert_file: "/etc/stacyvm/tls/worker.crt"
    server_key_file: "/etc/stacyvm/tls/worker.key"
    client_ca_file: "/etc/stacyvm/tls/control-plane-ca.crt"
    ca_file: "/etc/stacyvm/tls/worker-ca.crt"
    client_cert_file: "/etc/stacyvm/tls/control-plane.crt"
    client_key_file: "/etc/stacyvm/tls/control-plane.key"
    server_name: "worker-a.internal"
    insecure_skip_verify: false
```

On worker nodes, `server_cert_file` and `server_key_file` serve the inbound `/rpc` endpoint. When `client_ca_file` is set, the worker requires and verifies a client certificate from the control plane.

On control-plane nodes, `ca_file` verifies worker server certificates, `client_cert_file` and `client_key_file` present the control-plane client identity, and `server_name` pins the expected worker certificate name when DNS or advertised `rpc_url` hostnames differ.

`insecure_skip_verify` exists only for throwaway local tests and should fail production config lint.

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

The endpoint accepts `workerproto.Request` envelopes, requires the same worker headers, and currently implements `worker.status`, `worker.exec`, `worker.exec_stream`, file operations, `worker.logs`, `worker.renew_lease`, `worker.spawn`, `worker.destroy`, and `worker.shutdown`.

For `worker.spawn`, the request carries a control-plane `sandbox_id` and the response returns both that ID and the provider `runtime_id`. The control plane should persist that mapping before routing later status, exec, file, or destroy operations to the owning worker.

Remote workers advertise their control-plane callback endpoint through heartbeat capacity:

```json
{
  "capacity": {
    "max_sandboxes": 10,
    "rpc_url": "http://worker-a.internal:7430",
    "preview_domain": "worker-a.preview.example.com"
  }
}
```

When the scheduler selects a non-local worker with `rpc_url` and `auth.worker_token` is configured, the control plane acquires the sandbox lease for that worker, calls `worker.spawn`, persists the selected `worker_id`, and stores the returned provider runtime ID for later routing.

Sandbox reads use the persisted `worker_id` and provider `runtime_id` to call `worker.status` on the owning worker. If the worker reports a changed state, the control plane updates its stored sandbox state. If the worker is temporarily unreachable, the control plane keeps serving the cached record and logs the refresh failure at debug level.

Remote destroy uses the same persisted ownership tuple. The control plane fetches the durable sandbox lease, presents it to `worker.destroy`, updates sandbox state to `destroyed`, and releases the lease after the worker confirms teardown.

Remote non-streaming exec uses the same persisted ownership tuple without acquiring a new lifecycle lease. The control plane sends command, argv mode, environment, workdir, timeout, provider, sandbox ID, and provider runtime ID to `worker.exec`. The worker runs the command against its local provider registry and returns exit code, stdout, and stderr. The control plane still writes normal exec logs and emits the same audit, event, metric, and timeout behavior used by local exec.

Remote exec stream uses `worker.exec_stream` with `X-Worker-Stream: ndjson`. The worker flushes each stdout/stderr chunk as an NDJSON `workerproto.Response`, and the control plane exposes those chunks through the manager's existing stream channel API. Clients that do not request NDJSON can still receive the buffered `ExecStreamResult` response shape.

Remote file APIs use the same ownership tuple. The control plane validates/scopes paths, then sends the provider runtime ID and requested file operation to the owning worker. Dedicated remote sandboxes are not treated as pool sandboxes just because `VMID` stores the provider runtime ID; pool workspace scoping remains local-pool only.

Remote console logs use `worker.logs` with the persisted provider runtime ID, so workers read logs from the runtime they actually own instead of the control-plane sandbox ID.

Remote preview metadata uses `capacity.preview_domain` from the owning worker. The control plane returns that domain on remote-owned sandboxes so SDKs and the dashboard build URLs for the worker or cluster ingress that can actually reach the runtime. If a worker does not advertise a preview domain, the control plane falls back to `server.preview_domain`.

`worker.shutdown` is a drain signal. After receiving it, the worker rejects new `worker.spawn` assignments and reports `draining` in future heartbeats, which keeps it out of scheduler placement. Existing sandboxes keep their worker ownership while the worker is fresh and draining.

Startup reconciliation applies a conservative remote ownership policy:

- Fresh draining workers keep existing sandbox ownership.
- Stale, offline, or missing workers cause non-expired remote-owned sandboxes to become `unhealthy`.
- Expired remote-owned sandboxes become `expired` and release their durable lease.
- The control plane does not pretend to migrate a stateful runtime to another worker. Real reassignment requires provider-level snapshot or migration support.

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

Remote placement returns `remote_worker_rpc_unavailable` unless the selected worker advertises `rpc_url` and `auth.worker_token` is configured. Postgres-backed cluster storage and production worker identity are still outside the current transport.
