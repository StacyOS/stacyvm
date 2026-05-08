<p align="center">
  <img src="assets/logo.png" alt="StacyVM" />
</p>

<h3 align="center"><b>Your AI agent just got its own computer.</b></h3>

<p align="center">
Like E2B but self-hosted. Like Docker but actually isolated. Like Daytona but one binary.
</p>

<p align="center">
One tool. Every isolation level. Every platform.<br><br>
On a Mac? Docker provider, no KVM needed.<br>
On bare metal? Firecracker microVMs in ~28ms.<br>
On Kubernetes? gVisor or Kata containers.<br>
Need 100 sandboxes but only have 20 VMs? Pool mode.<br>
Need to expose <code>localhost:3000</code> from inside the sandbox? Live preview, one method call.<br><br>
Self-hosted. Single binary. Python &amp; TypeScript SDKs. MIT licensed. No cloud required.
</p>

<p align="center">
  <a href="https://github.com/StacyOs/stacyvm/stargazers"><img src="https://img.shields.io/github/stars/StacyOs/stacyvm?style=flat-square" alt="Stars"/></a>
  <a href="https://github.com/StacyOs/stacyvm/network/members"><img src="https://img.shields.io/github/forks/StacyOs/stacyvm?style=flat-square" alt="Forks"/></a>
  <a href="https://github.com/StacyOs/stacyvm/issues"><img src="https://img.shields.io/github/issues/StacyOs/stacyvm?style=flat-square" alt="Issues"/></a>
  <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT"/>
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20mac%20%7C%20windows-blue?style=flat-square" alt="Platform"/>
</p>

<p align="center">
  <a href="#quick-start-30-seconds">Quick Start</a> •
  <a href="#why-stacyvm">Why StacyVM</a> •
  <a href="#pick-your-isolation-level">Providers</a> •
  <a href="#live-preview">Live Preview</a> •
  <a href="#pool-mode">Pool Mode</a> •
  <a href="docs/deployment.md">Deployment</a> •
  <a href="docs/api.md">API Reference</a> •
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

## Table of contents

- [Table of contents](#table-of-contents)
- [Quick start (30 seconds)](#quick-start-30-seconds)
- [Why StacyVM?](#why-stacyvm)
  - [The math](#the-math)
- [Pick your isolation level](#pick-your-isolation-level)
- [Live Preview](#live-preview)
- [Pool mode — the feature nobody else has](#pool-mode--the-feature-nobody-else-has)
- [SDKs](#sdks)
- [REST API](#rest-api)
  - [Sandboxes](#sandboxes)
  - [Files (per sandbox)](#files-per-sandbox)
  - [Templates](#templates)
  - [Providers, pool, system](#providers-pool-system)
- [CLI](#cli)
- [Configuration](#configuration)
- [Production deployment](#production-deployment)
- [Templates](#templates-1)
- [Security defaults](#security-defaults)
- [Architecture](#architecture)
- [Web Dashboard](#web-dashboard)
- [Install options](#install-options)
- [Project layout](#project-layout)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Quick start (30 seconds)

```bash
git clone https://github.com/StacyOs/stacyvm && cd stacyvm
./scripts/setup.sh

# Start StacyVM and Traefik (Traefik powers Live Previews)
docker compose up -d

# Or run StacyVM locally without Docker:
# ./stacyvm serve
```

```bash
pip install stacyvm        # Python
npm install stacyvm        # TypeScript
```

```python
from stacyvm import Client

client = Client("http://localhost:7423")
sandbox = client.spawn(image="python:3.12")

result = sandbox.exec('python3 -c "print(\'hello from my own computer\')"')
print(result.stdout)  # hello from my own computer

sandbox.destroy()  # gone. forever.
```

7 lines. Your AI agent now has a real, isolated machine it can use and throw away.

---

## Why StacyVM?

You're building an AI agent. It generates code. That code needs to run somewhere safe.

**The problem:**

- **Docker** shares the host kernel. One container escape and your machine is owned. Multiple [runc CVEs in 2024-2025](https://github.com/opencontainers/runc/security/advisories) proved this isn't theoretical.
- **Cloud sandboxes** (E2B, Modal) send your code and data to someone else's servers. Adds latency, costs money, and you lose control of your data.
- **Daytona** is self-hostable but needs [12 services](https://www.daytona.io/docs/en/oss-deployment) (PostgreSQL, Redis, MinIO, Dex, registry...) just to get started.
- **Zeroboot** is blazing fast (~0.8ms) but strips everything — no networking, no filesystem, no multi-vCPU, serial-only I/O. Built for "run a function, get a result."

**StacyVM is one binary.** Self-hosted. Boots a sandbox in ~28ms. Your data never leaves your machine. And you choose the isolation level — Docker containers for dev, gVisor for cloud VMs, Firecracker microVMs for maximum hardware-level security.

| | StacyVM | E2B | Zeroboot | Daytona | Modal | Raw Docker |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| Self-hosted | ✅ | ❌ Cloud only | ✅ | ✅ (12 services) | ❌ Cloud only | ✅ |
| Isolation | KVM + gVisor + Docker | Container | KVM only | Container | Container | Shared kernel |
| Cold boot | **~28ms** (snapshot) | ~500ms | **~0.8ms** (CoW fork) | Seconds | Seconds | ~200ms |
| Networking | ✅ | ✅ | ❌ Serial only | ✅ | ✅ | ✅ |
| Filesystem / disk I/O | ✅ | ✅ | ❌ Memory only | ✅ | ✅ | ✅ |
| Multi-vCPU | ✅ | ✅ | ❌ Single vCPU | ✅ | ✅ | ✅ |
| Multiple providers | ✅ KVM/Docker/gVisor | ❌ | ❌ KVM only | ❌ | ❌ | N/A |
| Runs without KVM | ✅ Docker provider | N/A | ❌ | ✅ | N/A | ✅ |
| Multi-user pool mode | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Live preview URLs | ✅ Built-in (Traefik) | ✅ | ❌ | Partial | ✅ | ❌ |
| File API (read/write/glob) | ✅ 9 methods | ✅ | ❌ | ❌ | ❌ | ❌ |
| Python + TS SDKs | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
| Your data stays local | ✅ | ❌ | ✅ | ✅ | ❌ | ✅ |
| License | MIT | Partial | Apache 2.0 | Apache 2.0 | Proprietary | N/A |

> **On speed:** Zeroboot's 0.8ms is real — they bypass Firecracker's VMM entirely and `mmap(MAP_PRIVATE)` the snapshot memory as copy-on-write. But there's no disk, no network, and I/O is serial UART only. StacyVM's 28ms gives you a full sandbox with networking, file system, virtio, and multi-vCPU. Different tools for different jobs.

### The math

E2B charges per second. Default sandbox = 2 vCPU + 512 MiB RAM:

```
2 vCPU:    $0.000028/s
512 MiB:   $0.0000045/GiB/s × 0.5 GiB = $0.00000225/s
─────────────────────────────────────────
Total:     $0.00003025/s = $0.109/hour per sandbox
```

| Concurrent sandboxes | E2B / month | StacyVM pool mode |
|---|---|---|
| 10 | $261 compute + $150 plan = **$411** | **$0** |
| 50 | $1,307 compute + $150 plan = **$1,457** | **$0** |
| 100 | $2,614 compute + $150 plan = **$2,764** | **$0** |

_Assumes 8h/day active. StacyVM pool mode: 5 users per VM, your own infra._

---

## Pick your isolation level

StacyVM has a **provider interface**. One config change swaps the entire backend. Your application code doesn't change.

```yaml
# stacyvm.yaml — change one line
providers:
  default: "docker"  # or "firecracker", "e2b", "custom", "proot", "mock"
  docker:
    runtime: "runc"  # or "runsc" (gVisor) or "kata-runtime"
```

| Provider | What it does | KVM? | Boot | Use when |
|---|---|:---:|---|---|
| **Firecracker** | Real microVM. Own kernel, rootfs, network. ~28ms via snapshot restore. | Yes | ~28ms | Production. Maximum isolation. |
| **Docker** (runc) | OCI container with seccomp, cap_drop ALL, read-only rootfs option. | No | ~200ms | Dev, CI/CD, Mac, Windows. |
| **Docker** (gVisor) | Same as above, but syscalls hit a user-space kernel instead of host. | No | ~400ms | Cloud VMs. Stronger than containers. |
| **Docker** (Kata) | Lightweight VM per container. Hardware isolation without Firecracker setup. | Yes | ~1s | Kubernetes (AKS/GKE). |
| **E2B** | Forwards to E2B's hosted SaaS. Useful for hybrid deployments. | N/A | ~500ms | Bursting to cloud. |
| **Custom** | Pluggable HTTP backend. Bring your own runtime. | N/A | Varies | Special infra (HPC, Nomad, etc.). |
| **PRoot** | User-space chroot. No root, no KVM, no Docker. | No | Instant | Restricted hosts (Android, shared servers). |
| **Mock** | Temp directories on the host. Zero overhead. | No | Instant | Testing, development. |

Every provider implements the same interface. SDKs, REST API, CLI, pool mode, live preview — all work identically regardless of backend.

---

## Live Preview

Sandboxes can serve HTTP. StacyVM gives you a public URL for any port the sandbox exposes — no manual port forwarding, no SSH tunnels.

```python
from stacyvm import Client

client = Client("http://localhost:7423")
sandbox = client.spawn(image="node:20")

sandbox.write_file("/app/server.js", "require('http').createServer((req,res)=>res.end('hi')).listen(3000)")
sandbox.exec("node /app/server.js &")

print(sandbox.get_preview_url(3000))
# http://3000-sb-a1b2c3d4.localhost
```

```typescript
const sb = await client.spawn({ image: "node:20" });
await sb.writeFile("/app/index.js", code);
sb.exec("node /app/index.js &");

console.log(sb.getPreviewUrl(3000));
// http://3000-sb-a1b2c3d4.localhost
```

**How it works.** A bundled Traefik instance watches Docker labels. When you spawn a sandbox, StacyVM injects routing labels (`Host(\`3000-{id}.{domain}\`)`). Traefik picks them up instantly — no restarts, no config files. Open the URL in a browser, Traefik forwards the request to the sandbox's container.

**Local development:**
```yaml
# stacyvm.yaml
server:
  preview_domain: "localhost"   # browsers resolve *.localhost to 127.0.0.1
```
```bash
docker compose up -d
# visit http://3000-sb-xyz.localhost
```

**Production:**
```yaml
server:
  preview_domain: "stacyide.xyz"
```
Point a wildcard DNS record (`*.stacyide.xyz` → your server IP), give Traefik ports 80/443, and add an ACME resolver for Let's Encrypt. Users get HTTPS preview URLs automatically.

Full architecture write-up: [docs/live-preview-architecture.md](docs/live-preview-architecture.md).

> Live preview currently works with the Docker provider. Firecracker support is in progress (tracked on the roadmap).

---

## Pool mode — the feature nobody else has

Traditional sandbox tools: 1 user = 1 VM. 100 users = 100 VMs = massive bill.

StacyVM pool mode: **1 VM serves N users.** Each gets an isolated `/workspace/{id}/`. Path traversal blocked. Optional per-user UID + PID namespace hardening.

```yaml
pool:
  enabled: true
  max_vms: 20
  max_users_per_vm: 5
  image: "python:3.12-slim"
  memory_mb: 2048
  vcpus: 2
  overflow: "reject"   # or "queue"
```

Identify users with the `X-User-ID` header on every request:

```python
client = Client("http://localhost:7423", user_id="alice@example.com")
```
```typescript
const client = new Client({ baseUrl: "http://localhost:7423", userId: "alice@example.com" });
```

User IDs are trimmed by the server. They must be 128 characters or fewer and cannot contain whitespace, control characters, or path separators.

Hardening knobs (Docker provider):

```yaml
providers:
  docker:
    pool_security:
      per_user_uid: true              # each user gets a unique UID
      pid_namespace: true             # each user in a separate PID namespace
      workspace_permissions: true     # restrict file access between users
      hidepid: true                   # hide other users' processes from /proc
```

**100 users → 20 VMs instead of 100. 60% less infrastructure. Same isolation guarantees.**

Pool mode works with every provider — Docker containers, Firecracker microVMs, gVisor, Kata. The orchestrator handles user-to-VM assignment, workspace scoping, and cleanup automatically.

Check pool status from the SDK:
```python
print(client.pool_status())
# {"enabled": true, "vms": 3, "max_vms": 20, "total_users": 14, "max_users_per_vm": 5}
```

---

## SDKs

Both SDKs are thin wrappers over the REST API. Same method names, same return shapes (translated to native conventions per language).

<table>
<tr>
<td width="50%"><b>Python</b></td>
<td width="50%"><b>TypeScript</b></td>
</tr>
<tr>
<td>

```python
from stacyvm import Client

client = Client("http://localhost:7423")

# Context manager — auto-destroys on exit
with client.spawn(image="python:3.12") as sb:
    sb.exec("pip install pandas")
    sb.write_file("/app/analyze.py", code)
    result = sb.exec("python3 /app/analyze.py")
    print(result.stdout)

# Stream output
for chunk in sb.exec_stream("npm test"):
    print(chunk.data, end="")

# Async support
from stacyvm import AsyncClient
async with AsyncClient("http://localhost:7423") as client:
    sb = await client.spawn()
    result = await sb.exec("whoami")
    await sb.destroy()
```

</td>
<td>

```typescript
import { Client } from "stacyvm";

const client = new Client("http://localhost:7423");
const sb = await client.spawn({ image: "node:20" });

// Files + exec
await sb.writeFile("/app/index.js", code);
const result = await sb.exec("node /app/index.js");
console.log(result.stdout);

// Stream output in real-time
for await (const chunk of sb.execStream("npm test")) {
  process.stdout.write(chunk.data);
}

// Auto-destroy with withSandbox()
await client.withSandbox({ image: "node:20" }, async (sb) => {
  await sb.exec("npm test");
});

await sb.destroy();
```

</td>
</tr>
</table>

```bash
pip install stacyvm    # Python
npm install stacyvm    # TypeScript
```

Full SDK references:
- **Python:** [sdk/python/README.md](sdk/python/README.md)
- **TypeScript:** [sdk/js/README.md](sdk/js/README.md)

---

## REST API

Base URL: `http://localhost:7423/api/v1`

Auth: pass `X-API-Key: <your-key>` if `auth.enabled: true`. For pool mode, also send `X-User-ID: <user-id>`.

### Sandboxes

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/sandboxes` | Spawn a sandbox |
| `POST` | `/sandboxes/admission` | Preflight quota and scheduler admission |
| `GET` | `/sandboxes` | List active sandboxes |
| `DELETE` | `/sandboxes` | Prune expired sandboxes |
| `GET` | `/sandboxes/{id}` | Get sandbox details |
| `DELETE` | `/sandboxes/{id}` | Destroy sandbox |
| `POST` | `/sandboxes/{id}/extend` | Extend TTL |
| `POST` | `/sandboxes/{id}/exec` | Execute a command (sync or NDJSON stream) |
| `GET` | `/sandboxes/{id}/exec/ws` | Execute over WebSocket |
| `GET` | `/sandboxes/{id}/logs` | Console logs |

Exec requests default to backwards-compatible shell mode. Set `mode: "argv"` with `args` to run direct process arguments without shell interpolation.

### Files (per sandbox)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/sandboxes/{id}/files` | Write a file |
| `GET` | `/sandboxes/{id}/files?path=` | Read a file |
| `DELETE` | `/sandboxes/{id}/files?path=` | Delete a file (`recursive=true` for dirs) |
| `GET` | `/sandboxes/{id}/files/list?path=` | List a directory |
| `POST` | `/sandboxes/{id}/files/move` | Move/rename |
| `POST` | `/sandboxes/{id}/files/chmod` | Change permissions |
| `GET` | `/sandboxes/{id}/files/stat?path=` | File metadata |
| `GET` | `/sandboxes/{id}/files/glob?pattern=` | Glob pattern matching |

### Templates

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/templates` | Create a template |
| `GET` | `/templates` | List templates |
| `GET` | `/templates/{name}` | Get a template |
| `PUT` | `/templates/{name}` | Update a template |
| `DELETE` | `/templates/{name}` | Delete a template |
| `POST` | `/templates/{name}/spawn` | Spawn a sandbox from a template |

### Providers, pool, system

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/providers` | List configured providers |
| `GET` | `/providers/{name}` | Provider details + sandbox count |
| `POST` | `/providers/test` | Health-check all providers |
| `GET` | `/quotas` | List owner quota overrides |
| `GET` | `/quotas/summary` | Redacted owner quota policy counts |
| `PUT` | `/quotas/{ownerID}` | Create or update owner quota |
| `GET` | `/quotas/{ownerID}/usage` | Owner usage against effective quota |
| `GET` | `/pool/status` | Pool VM and user counts |
| `GET` | `/snapshots` | Available VM snapshots |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |
| `GET` | `/diagnostics` | Redacted operational diagnostics |
| `GET` | `/metrics` | Runtime metrics (goroutines, alloc, sandbox counts) |
| `GET` | `/metrics/prometheus` | Prometheus-compatible metrics |
| `GET` | `/events` | Server-sent events stream |

Admin aliases for providers, quotas, diagnostics, and metrics are available under `/admin/*` and can be protected with `auth.admin_api_key`.
For the operator dashboard, quota workflows, diagnostics, and persisted admin audit history, see [docs/admin-control-plane.md](docs/admin-control-plane.md).

Full schemas, request/response examples, and error codes: **[docs/api.md](docs/api.md)**.
OpenAPI spec: [docs/swagger.yaml](docs/swagger.yaml).

---

## CLI

```bash
stacyvm serve                                  # start the API server
stacyvm spawn --image python:3.12 --ttl 1h     # spawn
stacyvm exec sb-a1b2c3d4 -- python3 app.py     # run argv mode in a sandbox
stacyvm exec sb-a1b2c3d4 --shell -- "echo $HOME && pwd"
stacyvm list                                    # list active sandboxes
stacyvm kill sb-a1b2c3d4                        # destroy
stacyvm build-image python:3.12                 # pre-build rootfs (Firecracker)
stacyvm tui                                     # interactive dashboard
stacyvm version                                 # version info
```

Global flags:
- `--server` — server URL (default `http://localhost:7423`)
- `--api-key` — API key (or `STACYVM_API_KEY` env var)

---

## Configuration

```yaml
# stacyvm.yaml — sane defaults work without it
server:
  host: "0.0.0.0"
  port: 7423
  preview_domain: "localhost"   # used to build live-preview URLs

providers:
  default: "docker"

  docker:
    enabled: true
    socket: "unix:///var/run/docker.sock"
    runtime: "runc"             # or "runsc" (gVisor), "kata-runtime"
    network_mode: "bridge"
    read_only_rootfs: false
    seccomp_profile: "default"
    dropped_caps: ["ALL"]
    added_caps: []
    pids_limit: 256
    pool_security:
      per_user_uid: false
      pid_namespace: false
      workspace_permissions: true
      hidepid: false

  firecracker:
    enabled: true
    firecracker_path: "/usr/local/bin/firecracker"
    kernel_path: "/var/lib/stacyvm/vmlinux.bin"
    agent_path: "./bin/stacyvm-agent"
    data_dir: "/var/lib/stacyvm"

  e2b:
    enabled: false
    api_key: ""
    base_url: "https://api.e2b.dev"

  custom:
    enabled: false
    base_url: ""
    api_key: ""

  proot:
    enabled: false
    rootfs_path: "/var/lib/stacyvm/rootfs"

defaults:
  ttl: "30m"
  image: "alpine:latest"
  memory_mb: 1024
  vcpus: 1
  max_ttl: "24h"
  default_exec_timeout: "0s"    # disabled unless set
  max_exec_timeout: "10m"
  max_sandboxes: 0              # 0 = unlimited
  max_sandboxes_per_owner: 0    # 0 = unlimited
  spawn_overflow: "reject"       # reject or queue when sandbox capacity is full
  spawn_queue_timeout: "30s"
  max_spawn_queue: 100

auth:
  enabled: false
  api_key: ""
  admin_api_key: ""       # optional separate key for /api/v1/admin/*
  admin_fallback_enabled: true  # false requires admin_api_key for admin routes
  admin_audit_retention: "0s"  # 0s disables native audit pruning

rate_limit:
  enabled: false
  requests_per_minute: 120
  burst: 60
  key_by: "owner"        # owner, api_key, or ip
  bucket_ttl: "15m"
  cleanup_interval: "1m"

database:
  path: "stacyvm.db"

logging:
  level: "info"           # debug | info | warn | error
  format: "json"          # or "pretty"

pool:
  enabled: false
  max_vms: 10
  max_users_per_vm: 5
  image: "alpine:latest"
  memory_mb: 2048
  vcpus: 2
  overflow: "reject"      # or "queue"
```

**Config priority:** `./stacyvm.yaml` → `~/.stacyvm/config.yaml` → environment variables.

**Env vars:** prefix `STACYVM_`, dots become underscores. Examples:
```bash
STACYVM_SERVER_PORT=8080
STACYVM_PROVIDERS_DEFAULT=firecracker
STACYVM_AUTH_API_KEY=sk-xyz123
STACYVM_AUTH_ADMIN_API_KEY=sk-admin-xyz123
STACYVM_AUTH_ADMIN_FALLBACK_ENABLED=false
STACYVM_RATE_LIMIT_ENABLED=true
STACYVM_LOGGING_LEVEL=debug
```

---

## Production deployment

Use [docs/deployment.md](docs/deployment.md) for production setup guidance, including Docker Compose and systemd templates, auth and rate-limit defaults, health/readiness probes, Prometheus scraping, backup steps, and provider-specific rollout notes. Runtime signoff expectations live in [docs/runtime-conformance.md](docs/runtime-conformance.md), and release-candidate gates live in [docs/production-readiness.md](docs/production-readiness.md). The reusable templates live under [`deploy/`](deploy/).

Run `stacyvm doctor --production` on a target host before treating it as production-ready.

Release automation and GHCR publishing are documented in [docs/releasing.md](docs/releasing.md).

---

## Templates

Templates are pre-baked sandbox specs stored server-side. Define once, spawn many times.

```bash
curl -X POST http://localhost:7423/api/v1/templates \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "python-dev",
    "image": "python:3.12-slim",
    "memory_mb": 1024,
    "vcpus": 2,
    "ttl": "1h"
  }'
```

```python
sandbox = client.spawn(template="python-dev")           # spawn from template
client.templates.list()                                  # list all
client.templates.delete("python-dev")                    # delete
```

```typescript
const sb = await client.templates.spawn("python-dev");
const all = await client.templates.list();
await client.templates.delete("python-dev");
```

---

## Security defaults

Every sandbox ships locked down. You opt *in* to less restriction, not out.

| Layer | Default | What it does |
|---|---|---|
| Capabilities | `cap_drop: ALL` | Can't mount, ptrace, load modules, change networking |
| Syscalls | Seccomp default profile | Blocks ~44 dangerous syscalls |
| Filesystem | Read-only rootfs (Firecracker), opt-in (Docker) | Only `/tmp` and `/workspace` writable on Firecracker |
| Network | Bridge by default; `none` available | Switch to `network_mode: none` to block outbound |
| Processes | PID limit: 256 | Fork bombs die immediately |
| User | Non-root | No root inside the sandbox |
| Lifetime | TTL auto-expiry | Forgotten sandboxes clean themselves up |

With the Firecracker provider you also get: dedicated kernel per sandbox, vsock-only host-guest communication (no TCP between host and guest), and ephemeral rootfs destroyed on teardown.

Full security model and reporting policy: [SECURITY.md](SECURITY.md). Production admin hardening and identity-provider planning: [docs/security-governance.md](docs/security-governance.md). Release-candidate threat model: [docs/threat-model.md](docs/threat-model.md).

---

## Architecture

![StacyVM Architecture](assets/stacyvm_architecture_diagram.svg)

**Request flow:** SDK → REST API → Orchestrator (lifecycle, TTL, pool, templates) → Provider → Sandbox.

**Live preview flow:** Browser → Traefik → Docker label lookup → Sandbox container.

**Snapshot trick:** First Firecracker spawn cold-boots (~1s) and snapshots the VM state. Every spawn after that restores from snapshot in **~28ms** — faster than most HTTP requests. Details in [docs/snapshot-restore.md](docs/snapshot-restore.md).

---

## Web Dashboard

Built-in React dashboard for sandbox management, live terminal, file browser, and log viewer. Lives at `web/`.

```bash
make web                     # build the frontend (web/dist)
./stacyvm serve              # serves the dashboard at http://localhost:7423
```

The dashboard talks to the same REST API documented above — useful as a working reference.

---

## Install options

**One-command setup (recommended):**
```bash
git clone https://github.com/StacyOs/stacyvm && cd stacyvm
./scripts/setup.sh    # checks Go, Docker, KVM, downloads Firecracker + kernel, builds everything
./stacyvm serve
```

**Build from source:**
```bash
make build-all
sudo mkdir -p /var/lib/stacyvm && sudo chown $(whoami) /var/lib/stacyvm
./scripts/setup-kernel.sh
```

**Docker (with Traefik for live preview):**
```bash
docker compose up -d
# StacyVM:        http://localhost:7423
# Traefik admin:  http://localhost:8080
```

**Docker (StacyVM only):**
```bash
docker build -t stacyvm .
docker run -p 7423:7423 stacyvm
```

**Binary download** (when releases are available):
```bash
curl -fsSL https://github.com/StacyOs/stacyvm/releases/latest/download/stacyvm-linux-amd64 -o stacyvm
chmod +x stacyvm && sudo mv stacyvm /usr/local/bin/
```

---

## Project layout

```
stacyvm/
├── cmd/                 # CLI entrypoints (stacyvm, stacyvm-agent)
├── internal/            # Server, orchestrator, providers, API handlers
│   ├── api/             # HTTP handlers (chi router)
│   ├── orchestrator/    # Lifecycle, TTL, templates, pool, event bus
│   ├── providers/       # docker, firecracker, e2b, custom, proot, mock
│   └── config/          # Viper-based config loader
├── sdk/
│   ├── js/              # TypeScript SDK — see sdk/js/README.md
│   └── python/          # Python SDK — see sdk/python/README.md
├── web/                 # React dashboard
├── tui/                 # Terminal UI (bubbletea)
├── docs/                # Architecture docs, OpenAPI spec, API reference
├── scripts/             # setup.sh, build-rootfs.sh, install.sh, benchmarks
├── examples/            # Working code samples (js, python)
├── tests/               # Integration and provider tests
├── docker-compose.yml   # StacyVM + Traefik
└── Makefile             # build, test, web, release-build
```

---

## Roadmap

- [x] Firecracker provider (KVM microVMs, ~28ms snapshot restore)
- [x] Docker provider (OCI containers, seccomp, no KVM needed)
- [x] gVisor support (user-space kernel via runsc runtime)
- [x] Pool mode (N users per VM, workspace isolation)
- [x] Live Preview via Traefik (Docker provider)
- [x] Python SDK + TypeScript SDK
- [x] Web dashboard + TUI
- [x] Template system + warm pools
- [x] PRoot provider (root-less, KVM-less)
- [x] E2B + custom HTTP provider
- [ ] Live Preview for Firecracker
- [ ] Kata Containers provider (K8s-native)
- [ ] Persistent volumes across sandboxes
- [ ] MCP server mode
- [ ] GPU passthrough

---

## Contributing

PRs welcome — especially for new providers, SDK improvements, and documentation. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR. It covers the dev loop, where to put what, the test matrix, and the review process.

If you find a security issue, do **not** open a public issue — follow [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) — use it however you want.

---

<p align="center">
  <b>Built by <a href="https://github.com/StacyOs">StacyOs</a></b><br>
  If StacyVM helps you, drop a ⭐ — it helps others find it.
</p>
