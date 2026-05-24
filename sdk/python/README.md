# StacyVM Python SDK

Python client for [StacyVM](https://github.com/StacyOs/stacyvm) — self-hosted compute sandboxes for AI agents and code execution.

```bash
pip install stacyvm
```

- Python 3.9+.
- Sync (`Client`) and async (`AsyncClient`) APIs with identical surface.
- Single dependency: `httpx`.
- Type hints everywhere, ships `py.typed`.

---

## Table of contents

- [Quick start](#quick-start)
- [Connecting](#connecting)
- [Spawning sandboxes](#spawning-sandboxes)
- [Executing commands](#executing-commands)
- [Streaming output](#streaming-output)
- [File operations](#file-operations)
- [Live preview URLs](#live-preview-urls)
- [Templates](#templates)
- [TTL and lifecycle](#ttl-and-lifecycle)
- [Pool mode (multi-user)](#pool-mode-multi-user)
- [Async usage](#async-usage)
- [Server introspection](#server-introspection)
- [Errors](#errors)
- [Type reference](#type-reference)
- [Examples](#examples)

---

## Quick start

```python
from stacyvm import Client

client = Client("http://localhost:7423")

sandbox = client.spawn(image="python:3.12")
result = sandbox.exec("python3 -c 'print(40 + 2)'")
print(result.stdout)        # "42\n"

sandbox.destroy()
```

Or use the context manager — auto-destroys on exit:

```python
with client.spawn(image="python:3.12") as sb:
    sb.write_file("/app/main.py", "print('hi')")
    print(sb.exec("python3 /app/main.py").stdout)
# sandbox destroyed here, even if an exception was raised
```

---

## Connecting

```python
from stacyvm import Client

# Simplest form
client = Client("http://localhost:7423")

# With options
client = Client(
    base_url="http://localhost:7423",
    api_key=os.environ["STACYVM_API_KEY"],
    user_id="alice@example.com",   # for pool mode
    timeout=60.0,                   # seconds, applied to all HTTP requests
)

# Use as a context manager to close the underlying httpx client
with Client("http://localhost:7423") as client:
    ...
```

| Argument | Type | Default | Notes |
|---|---|---|---|
| `base_url` | `str` | `http://localhost:7423` | Server URL |
| `api_key` | `str \| None` | `None` | Sent as `X-API-Key` header |
| `user_id` | `str \| None` | `None` | Sent as `X-User-ID` header (pool mode) |
| `timeout` | `float` | `30.0` | Per-request timeout in seconds |

---

## Spawning sandboxes

```python
sandbox = client.spawn(
    image="python:3.12",
    provider="docker",          # override server default
    memory_mb=1024,
    vcpus=2,
    ttl="1h",                   # "30s", "5m", "2h" — Go duration syntax
    owner_id="team-a",          # optional per-owner quota identity
    metadata={"user": "alice", "task": "data-analysis"},
)
```

All parameters are optional. Server defaults apply when omitted.

| Parameter | Type | Description |
|---|---|---|
| `image` | `str` | Container or VM image (default `alpine:latest`) |
| `provider` | `str \| None` | `docker`, `firecracker`, `e2b`, `custom`, `proot`, `mock` |
| `memory_mb` | `int \| None` | RAM in MB |
| `vcpus` | `int \| None` | Virtual CPUs |
| `ttl` | `str \| None` | Auto-destroy after this duration |
| `owner_id` | `str \| None` | Owner ID for per-owner quotas when no `user_id` header is set |
| `template` | `str \| None` | Spawn from a server-side template by name |
| `metadata` | `dict[str, str] \| None` | Free-form labels |

Spawn from a template directly:

```python
sandbox = client.spawn_template("python-dev")
```

Preflight quota and scheduler admission without creating a sandbox:

```python
decision = client.admission(image="python:3.12", ttl="1h")
if not decision.allowed and decision.queueable:
    print(f"Request would queue because {decision.reason}")
```

---

## Executing commands

```python
result = sandbox.exec("python3 -c 'print(40+2)'")
# ExecResult(exit_code=0, stdout='42\n', stderr='', duration='127ms')

print(result.exit_code)
print(result.stdout)
print(result.duration)
```

With options:

```python
result = sandbox.exec(
    "npm test",
    args=["--coverage"],            # appended to command
    env={"NODE_ENV": "test"},       # additional env vars
    workdir="/app",                 # cwd
    timeout="30s",                  # server-side kill after this duration
)
```

| Parameter | Type | Description |
|---|---|---|
| `command` | `str` | Command to run |
| `args` | `list[str] \| None` | Arguments appended to the command |
| `env` | `dict[str, str] \| None` | Extra environment variables |
| `workdir` | `str \| None` | Working directory inside the sandbox |
| `timeout` | `str \| None` | Server-enforced timeout (Go duration string) |

---

## Streaming output

For long-running commands, stream stdout/stderr as it arrives. The server emits NDJSON; the SDK parses it into `StreamChunk` objects.

```python
for chunk in sandbox.exec_stream("pip install pandas"):
    if chunk.stream == "stdout":
        print(chunk.data, end="")
    else:
        print(chunk.data, end="", file=sys.stderr)
```

`exec_stream` accepts the same arguments as `exec` except `timeout` (the connection itself bounds the session).

---

## File operations

The SDK exposes the full file API. Paths are absolute inside the sandbox.

```python
# Write
sandbox.write_file("/app/main.py", "print('hi')")
sandbox.write_file("/app/run.sh",  "#!/bin/sh\necho hi", mode="755")

# Read
code = sandbox.read_file("/app/main.py")

# List a directory
entries = sandbox.list_files("/app")
# [{"name": "main.py", "path": "/app/main.py", "size": 11, "is_dir": False, ...}, ...]

# Glob
tests = sandbox.glob_files("/app/**/*.test.py")

# Stat
info = sandbox.stat_file("/app/main.py")

# Move / rename
sandbox.move_file("/app/main.py", "/app/entry.py")

# Permissions
sandbox.chmod_file("/app/run.sh", "755")

# Delete
sandbox.delete_file("/app/temp.log")
sandbox.delete_file("/app/cache", recursive=True)
```

| Method | Returns | Notes |
|---|---|---|
| `write_file(path, content, mode=None)` | `None` | `mode` is an octal string like `"755"` |
| `read_file(path)` | `str` | UTF-8 only — for binary, hit the REST API directly |
| `list_files(path="/")` | `list[dict]` | Directory entries |
| `delete_file(path, recursive=False)` | `None` | Set `recursive=True` for directories |
| `move_file(old_path, new_path)` | `None` | Move or rename |
| `chmod_file(path, mode)` | `None` | Octal string |
| `stat_file(path)` | `dict` | Single-entry metadata |
| `glob_files(pattern)` | `list[str]` | Matched paths |

---

## Live preview URLs

If your sandbox runs an HTTP server, get a public URL for any port. Backed by Traefik — see the [main README](../../README.md#live-preview).

```python
sb = client.spawn(image="python:3.12")
sb.write_file("/app/server.py", """
from http.server import HTTPServer, BaseHTTPRequestHandler
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b'hi')
HTTPServer(('0.0.0.0', 3000), H).serve_forever()
""")
sb.exec("python3 /app/server.py &")

print(sb.get_preview_url(3000))
# http://3000-sb-a1b2c3d4.localhost                  (local)
# https://3000-sb-a1b2c3d4.stacyide.xyz              (production)
```

`get_preview_url` is a regular synchronous method — it builds the URL from the sandbox ID and the server's `preview_domain` setting. Available on both `Sandbox` and `AsyncSandbox`.

---

## Templates

Templates are server-side blueprints. Define once, spawn many times.

```python
from stacyvm import Template

# Save a template (create or update)
client.templates.save(Template(
    name="python-dev",
    image="python:3.12-slim",
    memory_mb=1024,
    vcpus=2,
    ttl="1h",
    metadata={"language": "python"},
))

# Spawn from template
sb = client.spawn(template="python-dev")
# or:
sb = client.spawn_template("python-dev")

# List / get / delete
for t in client.templates.list():
    print(t.name, t.image)

client.templates.get("python-dev")
client.templates.delete("python-dev")
```

---

## TTL and lifecycle

Sandboxes auto-destroy after their TTL expires. Extend mid-run for long jobs.

```python
sandbox.extend_ttl("1h")       # bump by 1 hour
sandbox.extend_ttl()           # default: "30m"

sandbox.destroy()              # immediate teardown

# Refresh cached info from the server
sandbox.refresh()
print(sandbox.state)           # up-to-date state
```

Reattach to an existing sandbox by ID:

```python
sb = client.get("sb-a1b2c3d4")
sb.exec("ls /app")
```

---

## Pool mode (multi-user)

When the server runs in pool mode (multiple users sharing each VM), pass `user_id` so the server scopes the workspace correctly.

```python
client = Client(
    base_url="https://stacy.example.com",
    user_id="alice@example.com",
)

sb = client.spawn(image="python:3.12")
# sandbox now lives under /workspace/alice@example.com/ inside a shared VM

print(client.pool_status())
# {"enabled": True, "vms": 3, "max_vms": 20, "total_users": 14, "max_users_per_vm": 5}
```

---

## Async usage

`AsyncClient` mirrors `Client` exactly — every method is `async`, every sandbox is an `AsyncSandbox`.

```python
import asyncio
from stacyvm import AsyncClient

async def main():
    async with AsyncClient("http://localhost:7423") as client:
        async with await client.spawn(image="python:3.12") as sb:
            result = await sb.exec("python3 -c 'print(\"hi\")'")
            print(result.stdout)

            # Streaming
            async for chunk in sb.exec_stream("pip install pandas"):
                print(chunk.data, end="")

asyncio.run(main())
```

Behavioural notes:
- `AsyncClient` and `AsyncSandbox` support `async with` for cleanup.
- `await sb.destroy()` is the async teardown.
- `sb.get_preview_url(port)` is sync even on `AsyncSandbox` — it does no I/O.

---

## Server introspection

```python
client.health()             # {"status": "ok", "version": "0.5.1", "uptime": "2h13m"}
client.list()               # list[SandboxInfo] — all active sandboxes
client.pool_status()        # pool VM and user counts
client.quota_summary()      # QuotaSummary — redacted owner quota policy counts
client.prune()              # int — count of expired sandboxes destroyed
```

---

## Errors

All exceptions inherit from `StacyVMError`. Catch the base class for general handling, or specific subclasses for granular control.

```python
from stacyvm import (
    StacyVMError,
    SandboxNotFound,
    ProviderError,
    ConnectionError,
)

try:
    sandbox.exec("python3 main.py")
except SandboxNotFound as e:
    print(f"Sandbox {e.sandbox_id} no longer exists")
except ProviderError as e:
    print(f"Provider error ({e.code}): {e}")
except ConnectionError as e:
    print(f"Network issue: {e}")
except StacyVMError as e:
    print(f"API error ({e.code}): {e}")
```

| Exception | When | Properties |
|---|---|---|
| `StacyVMError` | Base — any API error | `code`, `message` |
| `SandboxNotFound` | 404 from server | `sandbox_id`, `code="not_found"` |
| `ProviderError` | 5xx from server | `code` |
| `ConnectionError` | Network failure | `message` |

---

## Type reference

Public exports from `stacyvm`:

```python
from stacyvm import (
    Client,
    AsyncClient,
    Sandbox,
    AsyncSandbox,
    Template,
    TemplateManager,
    # Models
    ExecResult,
    QuotaSummary,
    SandboxInfo,
    SpawnAdmissionDecision,
    StreamChunk,
    # Exceptions
    StacyVMError,
    SandboxNotFound,
    ProviderError,
    ConnectionError,
)
```

Selected dataclasses:

```python
@dataclass
class ExecResult:
    exit_code: int
    stdout: str
    stderr: str
    duration: str = ""        # e.g. "127ms"

@dataclass
class SandboxInfo:
    id: str
    state: str                # "creating" | "running" | "stopped" | "destroyed" | "error"
    provider: str
    image: str
    memory_mb: int = 512
    vcpus: int = 1
    created_at: str = ""
    expires_at: str = ""
    metadata: dict = field(default_factory=dict)
    preview_domain: str = "localhost"

@dataclass
class StreamChunk:
    stream: str               # "stdout" | "stderr"
    data: str

@dataclass
class Template:
    name: str
    image: str
    memory_mb: int = 512
    vcpus: int = 1
    ttl: str = "30m"
    provider: str = ""
    metadata: dict = field(default_factory=dict)
```

---

## Examples

Working examples live in [`examples/python/`](https://github.com/StacyOs/stacyvm/tree/main/examples/python):

- Basic spawn → exec → destroy
- Streaming output
- Live preview (Flask/FastAPI inside a sandbox)
- Pool mode with multiple users
- Async patterns with `asyncio.gather`
- Template-driven workflows

---

## Building from source

```bash
git clone https://github.com/StacyOs/stacyvm
cd stacyvm/sdk/python
pip install -e ".[dev]"
pytest
```

---

## License

Apache 2.0 — see [LICENSE](https://github.com/StacyOs/stacyvm/blob/main/LICENSE).
