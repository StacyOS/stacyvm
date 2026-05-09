# Remote Worker Staging

This guide runs StacyVM as two local processes: one control plane and one remote worker. Use the mock provider first so you can verify worker registration, remote spawn, remote status refresh, remote exec, and remote destroy before introducing Docker, Firecracker, or a real network boundary.

Remote worker mode can use a shared worker token for internal staging, per-worker static tokens for migration, or signed worker tokens for production-aligned staging. It is not the full enterprise production target yet because SQLite store semantics are still single-node oriented.

## Prerequisites

- Built `stacyvm` binary.
- One terminal for `stacyvm serve`.
- One terminal for `stacyvm worker`.
- A random worker token shared by both processes, a per-worker token configured under `auth.worker_tokens`, or a signed-token setup using `auth.worker_signing_key`.

## Control Plane Config

Create `control-plane.yaml`:

```yaml
server:
  host: "127.0.0.1"
  port: 7423

providers:
  default: "mock"
  mock:
    enabled: true
  docker:
    enabled: false
  firecracker:
    enabled: false

auth:
  api_key: "dev-api-key-dev-api-key-dev-api-key"
  admin_api_key: "dev-admin-key-dev-admin-key-dev"
  worker_token: "dev-worker-token-dev-worker-token"
  worker_tokens:
    worker-a: "dev-worker-a-token-dev-worker-a-token"
  admin_fallback_enabled: false

database:
  driver: "sqlite"
  path: "/tmp/stacyvm-remote-worker-staging.db"
```

Start the control plane:

```bash
STACYVM_CONFIG=control-plane.yaml stacyvm serve
```

## Worker Config

Create `worker.yaml`:

```yaml
worker:
  id: "worker-a"
  control_plane_url: "http://127.0.0.1:7423"
  listen_addr: "127.0.0.1:7430"
  preview_domain: "localhost"
  heartbeat_interval: "2s"

providers:
  default: "mock"
  mock:
    enabled: true
  docker:
    enabled: false
  firecracker:
    enabled: false

auth:
  worker_token: "dev-worker-a-token-dev-worker-a-token"
```

Start the worker:

```bash
STACYVM_CONFIG=worker.yaml stacyvm worker
```

## Smoke Flow

List workers:

```bash
curl -sS -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  http://127.0.0.1:7423/api/v1/workers
```

Spawn a sandbox. The scheduler should select `worker-a` once its heartbeat is fresh:

```bash
curl -sS -X DELETE \
  -H "X-Admin-API-Key: dev-admin-key-dev-admin-key-dev" \
  http://127.0.0.1:7423/api/v1/admin/workers/local

curl -sS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  -d '{"image":"alpine:latest","provider":"mock","ttl":"5m"}' \
  http://127.0.0.1:7423/api/v1/sandboxes
```

The delete call is only for this single-machine smoke flow. The control plane self-registers as `local`, and the scheduler prefers a fresh eligible local worker. Removing that transient registry record forces the next spawn to exercise `worker-a`.

Expected response fields:

```json
{
  "worker_id": "worker-a",
  "preview_domain": "localhost",
  "state": "running"
}
```

Get the sandbox. This refreshes status through `worker.status`:

```bash
curl -sS -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>
```

Run a command. This routes through `worker.exec` and records a normal control-plane exec log:

```bash
curl -sS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  -d '{"command":"echo remote worker ok"}' \
  http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>/exec
```

Run a streamed command. This routes through `worker.exec_stream` and forwards live stdout/stderr chunks through the normal API streaming response:

```bash
curl -sS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  -d '{"command":"printf \"line 1\\nline 2\\n\"","stream":true}' \
  http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>/exec
```

Write and read a file. These route through the worker file RPC methods:

```bash
curl -sS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  -d '{"path":"/workspace/remote.txt","content":"remote file"}' \
  http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>/files

curl -sS -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  "http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>/files?path=/workspace/remote.txt"
```

Fetch console logs. This routes through `worker.logs`:

```bash
curl -sS -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  "http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>/logs?lines=50"
```

Destroy the sandbox. This routes through `worker.destroy`, updates state, and releases the lease:

```bash
curl -sS -X DELETE \
  -H "X-API-Key: dev-api-key-dev-api-key-dev-api-key" \
  http://127.0.0.1:7423/api/v1/sandboxes/<sandbox-id>
```

## Automated Smoke Script

You can run the same flow with:

```bash
scripts/smoke-remote-worker.sh ./stacyvm
```

The script starts both processes, waits for worker registration, spawns a mock sandbox, verifies ownership by `worker-a`, and destroys it.

## Current Limits

- Remote non-streaming exec is routed to remote workers.
- Remote streaming exec is routed through live NDJSON worker RPC responses.
- Remote file APIs are routed to remote workers.
- Remote logs are routed to remote workers.
- Remote preview URL metadata is routed from worker heartbeat capacity. The actual preview ingress still depends on the worker/provider ingress setup, such as Docker plus Traefik on the worker host.
- Shared worker auth is suitable for local/internal staging only. Production-aligned workers should use signed worker identity, secret-file inputs, worker identity certification output, and mTLS when worker RPC crosses a host or network boundary.
- SQLite remains a staging/single-node store. Enterprise multi-worker mode still needs Postgres-grade lease semantics.
- Worker shutdown enters drain mode and rejects new spawns. Fresh draining workers keep existing sandbox ownership; stale/offline remote owners are marked `unhealthy`, and expired remote-owned sandboxes become `expired` with their lease released.
