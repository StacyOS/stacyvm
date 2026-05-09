# Remote Worker Staging

This guide runs StacyVM as two local processes: one control plane and one remote worker. Use the mock provider first so you can verify worker registration, remote spawn, remote status refresh, remote exec, and remote destroy before introducing Docker, Firecracker, or a real network boundary.

Phase 11 remote worker mode is for internal staging. It is not the enterprise production target yet because it still uses the configured shared worker token and SQLite store semantics.

## Prerequisites

- Built `stacyvm` binary.
- One terminal for `stacyvm serve`.
- One terminal for `stacyvm worker`.
- A random worker token shared by both processes.

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
  admin_fallback_enabled: false

database:
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
  worker_token: "dev-worker-token-dev-worker-token"
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
- Remote streaming exec, file APIs, logs, and previews are not routed to remote workers yet.
- Worker auth is a shared token suitable for staging; production enterprise mode should move to signed worker identity or mTLS.
- SQLite remains a staging/single-node store. Enterprise multi-worker mode still needs Postgres-grade lease semantics.
- Worker shutdown enters drain mode and rejects new spawns; full assignment handoff to another worker is still pending.
