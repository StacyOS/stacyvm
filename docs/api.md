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

## Providers

### List providers

```
GET /api/v1/providers
```

**Response** `200 OK`:
```json
[
  { "name": "docker",      "healthy": true, "default": true },
  { "name": "firecracker", "healthy": true, "default": false },
  { "name": "mock",        "healthy": true, "default": false }
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

### Metrics

```
GET /api/v1/metrics
```

**Response** `200 OK`:
```json
{
  "goroutines": 42,
  "memory_alloc": 17825792,
  "active_sandboxes": 12,
  "total_sandboxes": 138
}
```

For Prometheus-style metrics, scrape this endpoint and parse to your needs (a `/metrics` Prometheus exporter is on the roadmap).

---

## Events stream

```
GET /api/v1/events
```

**Response** `200 OK` with `Content-Type: text/event-stream`. The server emits orchestrator events as Server-Sent Events:

```
event: sandbox.spawned
data: {"id":"sb-a1b2c3d4","provider":"docker","image":"python:3.12"}

event: sandbox.destroyed
data: {"id":"sb-a1b2c3d4","reason":"ttl_expired"}

event: sandbox.exec
data: {"id":"sb-a1b2c3d4","command":"python3 main.py","exit_code":0}
```

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
