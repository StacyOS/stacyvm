# StacyVM REST API Reference

This document is the source of truth for the StacyVM HTTP API. The Python and TypeScript SDKs are thin wrappers over these endpoints — anything they do, you can do with `curl`.

- **Base URL:** `http://localhost:7423/api/v1`
- **Content type:** `application/json` (request and response, except where noted)
- **OpenAPI spec:** [swagger.yaml](swagger.yaml) / [swagger.json](swagger.json)

---

## Table of contents

- [Authentication](#authentication)
- [Conventions](#conventions)
- [Errors](#errors)
- [Sandboxes](#sandboxes)
- [Files](#files)
- [Templates](#templates)
- [Providers](#providers)
- [Snapshots](#snapshots)
- [Pool](#pool)
- [System](#system)
- [Events stream](#events-stream)
- [WebSocket exec](#websocket-exec)

---

## Authentication

Two optional headers, both off by default:

| Header | Purpose | Required when |
|---|---|---|
| `X-API-Key` | API key authentication | `auth.enabled: true` in `stacyvm.yaml` |
| `X-User-ID` | Multi-tenant pool mode user identifier | `pool.enabled: true` |

```bash
curl -H 'X-API-Key: sk-xyz123' \
     -H 'X-User-ID: alice@example.com' \
     http://localhost:7423/api/v1/sandboxes
```

CORS is permissive by default (`*`). Lock it down via reverse proxy if you expose StacyVM to the open internet.

`X-User-ID` is trimmed when present. It must be 128 characters or fewer and cannot contain whitespace, control characters, or path separators.

---

## Rate limiting

API rate limiting is optional and disabled by default. When `rate_limit.enabled` is true, StacyVM applies an in-memory token bucket to API routes.

```yaml
rate_limit:
  enabled: true
  requests_per_minute: 120
  burst: 60
  key_by: owner # owner, api_key, or ip
  bucket_ttl: 15m
  cleanup_interval: 1m
```

The default `owner` mode uses `X-User-ID` when present, then falls back to `X-API-Key`, then client IP. Limited requests return `429 Too Many Requests` with `Retry-After`, `X-RateLimit-Limit`, and `X-RateLimit-Remaining` headers.

Rate-limit buckets store hashed identity keys internally; raw owner IDs, API keys, and IP addresses are not exposed in diagnostics or metrics.

---

## Conventions

- **IDs.** Sandbox IDs look like `sb-a1b2c3d4`. Templates are addressed by `name`.
- **Durations.** All `ttl` and `timeout` fields use Go duration strings: `30s`, `5m`, `1h30m`.
- **Timestamps.** ISO 8601 UTC, e.g. `2026-05-04T10:30:00Z`.
- **File modes.** Octal strings, e.g. `"755"`, `"644"`.
- **Streaming.** `POST /sandboxes/{id}/exec` switches to NDJSON (`application/x-ndjson`) when `stream: true`.

---

## Errors

Errors return a JSON body with HTTP status reflecting the failure class:

```json
{
  "code": "not_found",
  "message": "sandbox sb-a1b2c3d4 not found"
}
```

| Status | Code | When |
|---|---|---|
| `400` | `bad_request` | Invalid input — missing field, malformed JSON |
| `401` | `unauthorized` | Bad / missing API key |
| `404` | `not_found` | Sandbox / template / provider does not exist |
| `409` | `conflict` | Template name already exists |
| `429` | `resource_limit` | Quota, capacity, or API rate limit exceeded |
| `500` | `provider_error` | Provider failed (Docker, Firecracker, etc.) |
| `503` | `unavailable` | Pool full with `overflow: reject` |

---

## Sandboxes

### Spawn a sandbox

```
POST /api/v1/sandboxes
```

**Request body** (all fields optional, server defaults apply):
```json
{
  "image": "python:3.12",
  "provider": "docker",
  "memory_mb": 1024,
  "vcpus": 2,
  "ttl": "1h",
  "metadata": { "user": "alice" }
}
```

**Response** `201 Created`:
```json
{
  "id": "sb-a1b2c3d4",
  "state": "running",
  "provider": "docker",
  "image": "python:3.12",
  "memory_mb": 1024,
  "vcpus": 2,
  "created_at": "2026-05-04T10:30:00Z",
  "expires_at": "2026-05-04T11:30:00Z",
  "metadata": { "user": "alice" },
  "preview_domain": "localhost"
}
```

### Evaluate spawn admission

```
POST /api/v1/sandboxes/admission
```

Preflight a spawn request against current quota and scheduler limits without creating a sandbox. `X-User-ID` overrides `owner_id`, matching the spawn endpoint.

**Request body**: same shape as `POST /api/v1/sandboxes`.

**Response** `200 OK`:
```json
{
  "allowed": false,
  "queueable": true,
  "reason": "max_sandboxes",
  "active_sandboxes": 100,
  "max_sandboxes": 100,
  "active_owner_sandboxes": 2,
  "max_owner_sandboxes": 10,
  "max_ttl": "24h0m0s"
}
```

`queueable` reflects the configured spawn overflow mode. Capacity denials are queueable only when `defaults.spawn_overflow` is `queue`; TTL denials are never queueable.

### List sandboxes

```
GET /api/v1/sandboxes
```

**Response** `200 OK`: array of sandbox objects.

### Get a sandbox

```
GET /api/v1/sandboxes/{id}
```

**Response** `200 OK` or `404 Not Found`.

### Destroy a sandbox

```
DELETE /api/v1/sandboxes/{id}
```

**Response** `200 OK`:
```json
{ "status": "destroyed" }
```

### Prune expired sandboxes

```
DELETE /api/v1/sandboxes
```

**Response** `200 OK`:
```json
{ "pruned": 7 }
```

### Extend TTL

```
POST /api/v1/sandboxes/{id}/extend
```

**Request body**:
```json
{ "ttl": "1h" }
```

**Response** `200 OK`: full sandbox object with updated `expires_at`.

### Execute a command

```
POST /api/v1/sandboxes/{id}/exec
```

**Request body**:
```json
{
  "command": "python3 -c 'print(40+2)'",
  "args": ["--coverage"],
  "env": { "NODE_ENV": "test" },
  "workdir": "/app",
  "timeout": "30s",
  "stream": false
}
```

**Response** `200 OK` (non-streaming):
```json
{
  "exit_code": 0,
  "stdout": "42\n",
  "stderr": "",
  "duration": "127ms"
}
```

**Response** `200 OK` (streaming, `stream: true`): `application/x-ndjson` — one JSON object per line:
```
{"stream":"stdout","data":"installing pandas...\n"}
{"stream":"stdout","data":"done\n"}
{"stream":"stderr","data":"warning: deprecated flag\n"}
```

### Console logs

```
GET /api/v1/sandboxes/{id}/logs?lines=200
```

`lines` defaults to `100`.

**Response** `200 OK`:
```json
["[init] mounting /workspace", "[init] starting agent", "..."]
```

---

## Files

All file paths are absolute inside the sandbox. The endpoints below are scoped under `/sandboxes/{id}/files`.

### Write a file

```
POST /api/v1/sandboxes/{id}/files
```

```json
{ "path": "/app/main.py", "content": "print('hi')", "mode": "644" }
```

**Response** `200 OK`: `{ "status": "written" }`.

### Read a file

```
GET /api/v1/sandboxes/{id}/files?path=/app/main.py
```

**Response** `200 OK`: raw file contents (binary safe). The SDKs decode as UTF-8.

### Delete a file or directory

```
DELETE /api/v1/sandboxes/{id}/files?path=/app/cache&recursive=true
```

`recursive` defaults to `false`. **Response** `200 OK`: `{ "status": "deleted" }`.

### List a directory

```
GET /api/v1/sandboxes/{id}/files/list?path=/app
```

`path` defaults to `/`.

**Response** `200 OK`:
```json
[
  {
    "name": "main.py",
    "path": "/app/main.py",
    "size": 11,
    "is_dir": false,
    "mod_time": "2026-05-04T10:32:14Z",
    "mode": "0644"
  }
]
```

### Move / rename

```
POST /api/v1/sandboxes/{id}/files/move
```

```json
{ "old_path": "/app/main.py", "new_path": "/app/entry.py" }
```

**Response** `200 OK`: `{ "status": "moved" }`.

### Change permissions

```
POST /api/v1/sandboxes/{id}/files/chmod
```

```json
{ "path": "/app/run.sh", "mode": "755" }
```

**Response** `200 OK`: `{ "status": "chmod applied" }`.

### Stat

```
GET /api/v1/sandboxes/{id}/files/stat?path=/app/main.py
```

**Response** `200 OK`: a single `FileInfo` object (same shape as list).

### Glob

```
GET /api/v1/sandboxes/{id}/files/glob?pattern=/app/**/*.py
```

**Response** `200 OK`:
```json
["/app/main.py", "/app/utils/helpers.py"]
```

---

## Templates

### Create a template

```
POST /api/v1/templates
```

```json
{
  "name": "python-dev",
  "image": "python:3.12-slim",
  "memory_mb": 1024,
  "vcpus": 2,
  "ttl": "1h",
  "provider": "docker",
  "metadata": { "language": "python" }
}
```

**Response** `201 Created`: the template object. `409 Conflict` if name is taken.

### List templates

```
GET /api/v1/templates
```

**Response** `200 OK`: array of templates.

### Get a template

```
GET /api/v1/templates/{name}
```

**Response** `200 OK` or `404 Not Found`.

### Update a template

```
PUT /api/v1/templates/{name}
```

Same body as create (without `name`). **Response** `200 OK` or `404`.

### Delete a template

```
DELETE /api/v1/templates/{name}
```

**Response** `200 OK`: `{ "status": "deleted" }`.

### Spawn from a template

```
POST /api/v1/templates/{name}/spawn
```

Optional override body:
```json
{ "ttl": "30m", "provider": "firecracker" }
```

**Response** `201 Created`: full sandbox object.

---

## Quotas

Owner quotas are persisted overrides for per-owner sandbox and runtime limits. They apply when requests include an owner via `X-User-ID` or `owner_id`.

Owner IDs are trimmed and must be 128 characters or fewer. They cannot contain whitespace, control characters, or path separators. Quota durations must use whole-second Go duration strings; use `0s` or omit a duration to inherit the global default.

### List owner quotas

```
GET /api/v1/quotas
```

**Response** `200 OK`:
```json
[
  {
    "owner_id": "team-a",
    "max_sandboxes": 5,
    "max_ttl": "2h0m0s",
    "max_exec_timeout": "1m0s",
    "created_at": "2026-05-08T10:30:00Z",
    "updated_at": "2026-05-08T10:30:00Z"
  }
]
```

### Get quota summary

```
GET /api/v1/quotas/summary
```

Returns redacted policy coverage counts without exposing owner IDs.

**Response** `200 OK`:
```json
{
  "total": 2,
  "with_max_sandboxes": 1,
  "with_max_ttl": 1,
  "with_max_exec_timeout": 1
}
```

### Save owner quota

```
PUT /api/v1/quotas/{ownerID}
```

**Request**:
```json
{
  "max_sandboxes": 5,
  "max_ttl": "2h",
  "max_exec_timeout": "1m"
}
```

**Response** `200 OK`: full owner quota object.

Invalid owner IDs, negative sandbox counts, malformed durations, sub-second durations, and fractional-second durations return `400 Bad Request`.

### Get owner usage

```
GET /api/v1/quotas/{ownerID}/usage
```

**Response** `200 OK`:
```json
{
  "owner_id": "team-a",
  "active_sandboxes": 3,
  "max_sandboxes": 5,
  "max_ttl": "2h0m0s",
  "max_exec_timeout": "1m0s",
  "quota_configured": true
}
```

### Delete owner quota

```
DELETE /api/v1/quotas/{ownerID}
```

**Response** `200 OK`: `{ "status": "deleted" }`.

---

## Providers

### List providers

```
GET /api/v1/providers
```

**Response** `200 OK`:
```json
[
  {
    "name": "docker",
    "healthy": true,
    "default": true,
    "latency_ms": 3,
    "last_checked": "2026-05-08T10:30:00Z",
    "capabilities": ["spawn", "exec", "exec_stream", "files", "console", "health", "runtime_inventory", "container"],
    "runtime_count": 4
  },
  {
    "name": "firecracker",
    "healthy": false,
    "default": false,
    "latency_ms": 1,
    "last_checked": "2026-05-08T10:30:00Z",
    "error": "health check returned false",
    "capabilities": ["spawn", "exec", "exec_stream", "files", "console", "health", "snapshots", "microvm", "vsock_agent"]
  }
]
```

### Get a provider

```
GET /api/v1/providers/{name}
```

**Response** `200 OK`:
```json
{
  "name": "docker",
  "healthy": true,
  "default": true,
  "sandbox_count": 12,
  "health": {
    "name": "docker",
    "healthy": true,
    "default": true,
    "latency_ms": 3,
    "last_checked": "2026-05-08T10:30:00Z",
    "capabilities": ["spawn", "exec", "files", "runtime_inventory", "container"],
    "runtime_count": 4
  },
  "config": { "runtime": "runc", "network_mode": "stacyvm-network" }
}
```

### Health-check all providers

```
POST /api/v1/providers/test
```

**Response** `200 OK`:
```json
{ "docker": true, "firecracker": true, "mock": true }
```

---

## Snapshots

### List Firecracker snapshots

```
GET /api/v1/snapshots
```

**Response** `200 OK`: array of snapshot summaries (image name, kernel, size, created_at).

---

## Pool

### Pool status

```
GET /api/v1/pool/status
```

**Response** `200 OK` (pool enabled):
```json
{
  "enabled": true,
  "vms": 3,
  "max_vms": 20,
  "total_users": 14,
  "max_users_per_vm": 5
}
```

**Response** `200 OK` (pool disabled):
```json
{ "enabled": false }
```

---

## System

### Health

```
GET /api/v1/health
```

**Response** `200 OK`:
```json
{ "status": "ok", "version": "0.5.1", "uptime": "2h13m" }
```

### Liveness

```
GET /api/v1/live
```

**Response** `200 OK`:
```json
{ "status": "alive", "version": "0.5.1", "uptime": "2h13m" }
```

Use this endpoint for process liveness checks. It only confirms that the API process is responding.

### Readiness

```
GET /api/v1/ready
```

**Response** `200 OK`:
```json
{
  "status": "ready",
  "version": "0.5.1",
  "uptime": "2h13m",
  "ready_providers": 1,
  "total_providers": 2,
  "providers": [
    {
      "name": "docker",
      "healthy": true,
      "default": true,
      "latency_ms": 3,
      "last_checked": "2026-05-08T10:30:00Z",
      "capabilities": ["spawn", "exec", "files", "runtime_inventory", "container"],
      "runtime_count": 4
    },
    {
      "name": "firecracker",
      "healthy": false,
      "default": false,
      "latency_ms": 1,
      "last_checked": "2026-05-08T10:30:00Z",
      "error": "health check returned false",
      "capabilities": ["spawn", "exec", "files", "snapshots", "microvm", "vsock_agent"]
    }
  ]
}
```

**Response** `503 Service Unavailable` when no configured provider is healthy.

### Diagnostics

```
GET /api/v1/diagnostics
```

**Response** `200 OK`:
```json
{
  "generated_at": "2026-05-08T10:30:00Z",
  "build": {
    "version": "0.5.1",
    "goos": "linux",
    "goarch": "amd64"
  },
  "process": {
    "uptime": "2h13m",
    "goroutines": 42,
    "memory": {
      "alloc": 17825792,
      "sys": 71303168,
      "heap_alloc": 17825792,
      "gc_cycles": 8
    }
  },
  "store": {
    "healthy": true,
    "latency_ms": 1
  },
  "limits": {
    "max_sandboxes": 100,
    "max_sandboxes_per_owner": 10,
    "default_exec_timeout": "30s",
    "max_exec_timeout": "10m0s",
    "max_ttl": "24h0m0s",
    "spawn_overflow": "queue",
    "spawn_queue_timeout": "30s",
    "max_spawn_queue": 100
  },
  "scheduler": {
    "spawn_overflow": "queue",
    "spawn_queue_depth": 3,
    "max_spawn_queue": 100,
    "spawn_queue_timeout": "30s",
    "admission_control": "single_node",
    "spawn_queued_total": 18,
    "spawn_dequeued_total": 16,
    "spawn_queue_timeouts": 2,
    "spawn_queue_wait_count": 18,
    "spawn_queue_wait_total": "1m42s",
    "spawn_queue_wait_max": "12s",
    "spawn_queue_wait_avg": "5.666s",
    "spawn_queue_wait_total_ms": 102000,
    "spawn_queue_wait_max_ms": 12000,
    "spawn_queue_wait_avg_ms": 5666
  },
  "quotas": {
    "total": 8,
    "with_max_sandboxes": 6,
    "with_max_ttl": 4,
    "with_max_exec_timeout": 3
  },
  "rate_limit": {
    "enabled": true,
    "requests_per_minute": 120,
    "burst": 60,
    "key_by": "owner",
    "active_buckets": 14,
    "allowed_total": 9132,
    "limited_total": 27,
    "evicted_total": 4,
    "bucket_ttl": "15m0s",
    "cleanup_interval": "1m0s"
  },
  "providers": [
    {
      "name": "docker",
      "healthy": true,
      "default": true,
      "latency_ms": 3,
      "last_checked": "2026-05-08T10:30:00Z",
      "capabilities": ["spawn", "exec", "files", "runtime_inventory", "container"],
      "runtime_count": 4
    }
  ],
  "sandboxes": {
    "total": 138,
    "active": 12,
    "by_state": { "running": 12, "destroyed": 126 },
    "by_provider": { "docker": 90, "firecracker": 48 }
  },
  "events": {
    "subscribers": 2,
    "history_size": 1000,
    "events_total": 2401
  },
  "operations": [],
  "redactions": ["provider secrets", "registry credentials", "environment secrets", "API keys"]
}
```

Diagnostics are read-only and intentionally redacted. Use this endpoint for support bundles, incident debugging, and deployment sanity checks.

### Metrics

```
GET /api/v1/metrics
```

**Response** `200 OK`:
```json
{
  "uptime": "2h13m",
  "goroutines": 42,
  "memory_alloc": 17825792,
  "memory_sys": 71303168,
  "memory_heap_alloc": 17825792,
  "gc_cycles": 8,
  "sandboxes": {
    "total": 138,
    "active": 12,
    "by_state": { "running": 12, "destroyed": 126 },
    "by_provider": { "docker": 90, "firecracker": 48 }
  },
  "providers": {
    "total": 2,
    "healthy": 1,
    "items": [
      { "name": "docker", "healthy": true, "default": true },
      { "name": "firecracker", "healthy": false, "default": false }
    ]
  },
  "events": {
    "subscribers": 2,
    "history_size": 1000,
    "events_total": 2401
  },
  "scheduler": {
    "spawn_overflow": "queue",
    "spawn_queue_depth": 3,
    "max_spawn_queue": 100,
    "spawn_queue_timeout": "30s",
    "admission_control": "single_node",
    "spawn_queued_total": 18,
    "spawn_dequeued_total": 16,
    "spawn_queue_timeouts": 2,
    "spawn_queue_wait_count": 18,
    "spawn_queue_wait_total": "1m42s",
    "spawn_queue_wait_max": "12s",
    "spawn_queue_wait_avg": "5.666s",
    "spawn_queue_wait_total_ms": 102000,
    "spawn_queue_wait_max_ms": 12000,
    "spawn_queue_wait_avg_ms": 5666
  },
  "quotas": {
    "total": 8,
    "with_max_sandboxes": 6,
    "with_max_ttl": 4,
    "with_max_exec_timeout": 3
  },
  "rate_limit": {
    "enabled": true,
    "requests_per_minute": 120,
    "burst": 60,
    "key_by": "owner",
    "active_buckets": 14,
    "allowed_total": 9132,
    "limited_total": 27,
    "evicted_total": 4,
    "bucket_ttl": "15m0s",
    "cleanup_interval": "1m0s"
  },
  "operations": [
    {
      "operation": "exec",
      "provider": "docker",
      "success_total": 482,
      "failure_total": 7,
      "latency_count": 489,
      "latency_total_ms": 39120,
      "latency_min_ms": 3,
      "latency_max_ms": 2500,
      "latency_avg_ms": 80
    }
  ]
}
```

### Prometheus metrics

```
GET /api/v1/metrics/prometheus
```

**Response** `200 OK`:
```text
# HELP stacyvm_uptime_seconds StacyVM API process uptime in seconds.
# TYPE stacyvm_uptime_seconds gauge
stacyvm_uptime_seconds 7980
# HELP stacyvm_provider_healthy Provider health status where 1 is healthy and 0 is unhealthy.
# TYPE stacyvm_provider_healthy gauge
stacyvm_provider_healthy{provider="docker",default="true"} 1
stacyvm_spawn_queue_depth 3
stacyvm_spawn_queue_wait_milliseconds_count 18
stacyvm_owner_quotas_total 8
stacyvm_rate_limit_blocked_total 27
stacyvm_operation_success_total{operation="exec",provider="docker"} 482
stacyvm_operation_failure_total{operation="exec",provider="docker"} 7
```

Use this endpoint for Prometheus-compatible scraping of runtime, provider, sandbox, event, and operation metrics.

---

## Events stream

```
GET /api/v1/events
```

**Response** `200 OK` with `Content-Type: text/event-stream`. The server emits orchestrator events as Server-Sent Events:

```
data: {"id":"evt-1","type":"sandbox.created","sandbox_id":"sb-a1b2c3d4","timestamp":"2026-05-08T10:30:00Z"}

data: {"id":"evt-2","type":"exec.timeout","sandbox_id":"sb-a1b2c3d4","timestamp":"2026-05-08T10:31:00Z","data":{"operation":"exec","provider":"docker","error":"exec timeout: sb-a1b2c3d4"}}

data: {"id":"evt-3","type":"reconcile.action","sandbox_id":"sb-a1b2c3d4","timestamp":"2026-05-08T10:32:00Z","data":{"action":"adopted_runtime","provider":"docker","image":"python:3.12"}}
```

Common event types include:

- `sandbox.created`, `sandbox.running`, `sandbox.destroyed`, `sandbox.error`
- `exec.started`, `exec.completed`, `exec.failed`, `exec.timeout`
- `file.written`, `file.read`
- `operation.failed`, `resource.limit`, `provider.failed`, `reconcile.action`
- `spawn.queued`, `spawn.dequeued`, `spawn.queue_timeout`
- `quota.saved`, `quota.deleted`

Use any SSE client (`EventSource` in browsers, `httpx-sse` in Python, etc.) to consume.

---

## WebSocket exec

```
GET /api/v1/sandboxes/{id}/exec/ws
```

Upgrades the connection to a WebSocket for interactive command execution. Useful for terminals, REPLs, and any case where you need bi-directional I/O.

**Client → server messages:**
```json
{ "type": "start",  "command": "python3", "env": { "PYTHONUNBUFFERED": "1" } }
{ "type": "stdin",  "data": "print('hi')\n" }
{ "type": "resize", "cols": 80, "rows": 24 }
{ "type": "signal", "signal": "SIGINT" }
```

**Server → client messages:**
```json
{ "type": "stdout", "data": "hi\n" }
{ "type": "stderr", "data": "..." }
{ "type": "exit",   "exit_code": 0 }
```

The web dashboard uses this endpoint to power its live terminal — a concrete reference is at [`web/src/`](../web/src/).

---

## SDK mapping

If you'd rather write Python or TypeScript than `curl`, every endpoint above maps 1:1 to an SDK method:

| Endpoint | Python | TypeScript |
|---|---|---|
| `POST /sandboxes` | `client.spawn(...)` | `client.spawn(...)` |
| `GET /sandboxes/{id}` | `client.get(id)` | `client.get(id)` |
| `POST /sandboxes/{id}/exec` | `sb.exec(cmd)` / `sb.exec_stream(cmd)` | `sb.exec(cmd)` / `sb.execStream(cmd)` |
| `POST /sandboxes/{id}/files` | `sb.write_file(path, content)` | `sb.writeFile(path, content)` |
| `GET /sandboxes/{id}/files` | `sb.read_file(path)` | `sb.readFile(path)` |
| `POST /templates/{name}/spawn` | `client.spawn_template(name)` | `client.templates.spawn(name)` |
| `GET /pool/status` | `client.pool_status()` | `client.poolStatus()` |
| `GET /health` | `client.health()` | `client.health()` |

Full SDK docs: [Python](../sdk/python/README.md) · [TypeScript](../sdk/js/README.md).
