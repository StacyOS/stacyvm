# StacyVM TypeScript SDK

TypeScript / JavaScript client for [StacyVM](https://github.com/StacyOs/stacyvm) — self-hosted compute sandboxes for AI agents and code execution.

```bash
npm install stacyvm
```

- Works in Node.js 18+ (uses the built-in `fetch` and `ReadableStream` APIs).
- Zero runtime dependencies.
- Full TypeScript types shipped (`dist/index.d.ts`).
- ESM only.

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
- [Server introspection](#server-introspection)
- [Errors](#errors)
- [Type reference](#type-reference)
- [Examples](#examples)

---

## Quick start

```typescript
import { Client } from "stacyvm";

const client = new Client("http://localhost:7423");

const sandbox = await client.spawn({ image: "node:20" });
const result = await sandbox.exec("node -e 'console.log(1+1)'");
console.log(result.stdout);   // "2\n"

await sandbox.destroy();
```

Or use `withSandbox` for automatic cleanup:

```typescript
await client.withSandbox({ image: "node:20" }, async (sb) => {
  await sb.writeFile("/app/main.js", code);
  const result = await sb.exec("node /app/main.js");
  console.log(result.stdout);
});
// destroyed automatically, even if the callback throws
```

---

## Connecting

The `Client` constructor accepts either a URL string or an options object.

```typescript
// Simplest form
const client = new Client("http://localhost:7423");

// With options
const client = new Client({
  baseUrl: "http://localhost:7423",
  apiKey: process.env.STACYVM_API_KEY,
  userId: "alice@example.com",   // for pool mode
  timeout: 60_000,                // ms, applied to all HTTP requests
});

// Or split host/port
const client = new Client({ host: "10.0.0.5", port: 7423 });
```

| Option | Type | Default | Notes |
|---|---|---|---|
| `baseUrl` | `string` | `http://localhost:7423` | Full server URL |
| `host` | `string` | `localhost` | Used if `baseUrl` is omitted |
| `port` | `number` | `7423` | Used if `baseUrl` is omitted |
| `apiKey` | `string` | — | Sent as `X-API-Key` header |
| `userId` | `string` | — | Sent as `X-User-ID` header (pool mode) |
| `timeout` | `number` | `30_000` | Per-request timeout in ms |

---

## Spawning sandboxes

```typescript
const sandbox = await client.spawn({
  image: "python:3.12",
  provider: "docker",       // override server default
  memory_mb: 1024,
  vcpus: 2,
  ttl: "1h",                // "30s", "5m", "2h" — Go duration syntax
  metadata: { user: "alice", task: "data-analysis" },
});
```

All fields on `SpawnOptions` are optional. Server defaults apply when fields are omitted.

| Field | Type | Description |
|---|---|---|
| `image` | `string` | Container or VM image (e.g. `python:3.12`) |
| `provider` | `string` | `docker` \| `firecracker` \| `e2b` \| `custom` \| `proot` \| `mock` |
| `memory_mb` | `number` | RAM in MB |
| `vcpus` | `number` | Virtual CPUs |
| `ttl` | `string` | Auto-destroy after this duration |
| `metadata` | `Record<string, string>` | Free-form labels |

---

## Executing commands

```typescript
const result = await sandbox.exec("python3 -c 'print(40+2)'");
//             ^? ExecResult { exit_code, stdout, stderr, duration }

console.log(result.exit_code); // 0
console.log(result.stdout);    // "42\n"
console.log(result.duration);  // "127ms"
```

With options:

```typescript
const result = await sandbox.exec("npm test", {
  args: ["--coverage"],            // appended to command
  env: { NODE_ENV: "test" },       // additional env vars
  workdir: "/app",                 // cwd
  timeout: "30s",                  // server-side kill after this duration
});
```

| Field | Type | Description |
|---|---|---|
| `args` | `string[]` | Arguments appended to the command |
| `env` | `Record<string, string>` | Extra environment variables |
| `workdir` | `string` | Working directory inside the sandbox |
| `timeout` | `string` | Server-enforced timeout (Go duration string) |

---

## Streaming output

For long-running commands, stream stdout/stderr as it arrives. The server emits NDJSON; the SDK parses it into `StreamChunk` objects.

```typescript
for await (const chunk of sandbox.execStream("npm install")) {
  if (chunk.stream === "stdout") process.stdout.write(chunk.data);
  else                            process.stderr.write(chunk.data);
}
```

`execStream` accepts the same options as `exec` except `timeout` (streaming sessions live as long as the connection).

---

## File operations

The SDK exposes the full file API. Paths are absolute inside the sandbox.

```typescript
// Write
await sandbox.writeFile("/app/main.py", "print('hi')");
await sandbox.writeFile("/app/run.sh",  "#!/bin/sh\necho hi", "755");  // mode

// Read
const code = await sandbox.readFile("/app/main.py");

// List a directory
const entries = await sandbox.listFiles("/app");
//      ^? FileInfo[] { name, path, size, is_dir, mod_time, mode }

// Glob
const tests = await sandbox.globFiles("/app/**/*.test.js");

// Stat
const info = await sandbox.statFile("/app/main.py");

// Move / rename
await sandbox.moveFile("/app/main.py", "/app/entry.py");

// Permissions
await sandbox.chmodFile("/app/run.sh", "755");

// Delete
await sandbox.deleteFile("/app/temp.log");
await sandbox.deleteFile("/app/cache",   true);   // recursive
```

| Method | Returns | Notes |
|---|---|---|
| `writeFile(path, content, mode?)` | `void` | `mode` is an octal string like `"755"` |
| `readFile(path)` | `string` | UTF-8 only — for binary, use the REST API directly |
| `listFiles(path?)` | `FileInfo[]` | Defaults to `/` |
| `deleteFile(path, recursive?)` | `void` | `recursive: true` for directories |
| `moveFile(oldPath, newPath)` | `void` | Move or rename |
| `chmodFile(path, mode)` | `void` | Octal string |
| `statFile(path)` | `FileInfo` | Single-entry metadata |
| `globFiles(pattern)` | `string[]` | Returns matched paths |

---

## Live preview URLs

If your sandbox runs an HTTP server, get a public URL for any port. Backed by Traefik — see the [main README](../../README.md#live-preview).

```typescript
const sb = await client.spawn({ image: "node:20" });
await sb.writeFile("/app/server.js", "require('http').createServer((q,r)=>r.end('hi')).listen(3000)");
sb.exec("node /app/server.js &");

console.log(sb.getPreviewUrl(3000));
// http://3000-sb-a1b2c3d4.localhost  (local)
// https://3000-sb-a1b2c3d4.stacyide.xyz  (production)
```

`getPreviewUrl` is synchronous — it constructs the URL from the sandbox ID and the server's `preview_domain` setting.

---

## Templates

Templates are server-side blueprints. Define once, spawn many times.

```typescript
// Create or update
await client.templates.save({
  name: "python-dev",
  image: "python:3.12-slim",
  memory_mb: 1024,
  vcpus: 2,
  ttl: "1h",
  metadata: { language: "python" },
});

// Spawn from template
const sb = await client.templates.spawn("python-dev");

// Override on spawn
const sb2 = await client.templates.spawn("python-dev", { ttl: "5m", provider: "firecracker" });

// List / get / delete
const all = await client.templates.list();
const t = await client.templates.get("python-dev");
await client.templates.delete("python-dev");
```

---

## TTL and lifecycle

Sandboxes auto-destroy after their TTL expires. Extend mid-run if a job is taking longer than expected.

```typescript
await sandbox.extendTtl("1h");      // bump by 1 hour
await sandbox.extendTtl();          // default: 30 minutes

await sandbox.destroy();            // immediate teardown

// Refresh cached info from the server
await sandbox.refresh();
console.log(sandbox.state);         // up-to-date state
```

Reattach to an existing sandbox by ID:

```typescript
const sb = await client.get("sb-a1b2c3d4");
await sb.exec("ls /app");
```

---

## Pool mode (multi-user)

When the server runs in pool mode (multiple users sharing each VM), pass `userId` so the server knows whose workspace to scope to.

```typescript
const client = new Client({
  baseUrl: "https://stacy.example.com",
  userId: "alice@example.com",
});

const sb = await client.spawn({ image: "python:3.12" });
// sandbox now lives under /workspace/alice@example.com/ inside a shared VM

console.log(await client.poolStatus());
// { enabled: true, vms: 3, max_vms: 20, total_users: 14, max_users_per_vm: 5 }
```

---

## Server introspection

```typescript
await client.health();        // { status: "ok", version: "0.5.1", uptime: "2h13m" }
await client.list();          // SandboxInfo[] — all active sandboxes
await client.providers();     // [{ name: "docker", healthy: true, default: true }, ...]
await client.poolStatus();    // pool VM and user counts
await client.prune();         // returns count of expired sandboxes destroyed
```

---

## Errors

All errors extend `ForgevmError`. Catch the base class for general handling, or specific subclasses to react to particular failures.

```typescript
import {
  ForgevmError,
  SandboxNotFoundError,
  ProviderError,
  ConnectionError,
} from "stacyvm";

try {
  await sandbox.exec("python3 main.py");
} catch (err) {
  if (err instanceof SandboxNotFoundError) {
    // sandbox was destroyed or never existed — err.sandboxId is set
  } else if (err instanceof ProviderError) {
    // provider returned 5xx
  } else if (err instanceof ConnectionError) {
    // network issue
  } else if (err instanceof ForgevmError) {
    // any other API error — err.code, err.statusCode
  } else {
    throw err;
  }
}
```

| Error | When | Properties |
|---|---|---|
| `ForgevmError` | Base — any API error | `code`, `statusCode`, `message` |
| `SandboxNotFoundError` | 404 from server | `sandboxId` |
| `ProviderError` | 5xx from server | `code`, `statusCode` |
| `ConnectionError` | Network failure | `message` |

---

## Type reference

Everything is exported from the package root. Notable types:

```typescript
import {
  Client,
  Sandbox,
  TemplateManager,
  // Types
  SandboxState,        // "creating" | "running" | "stopped" | "destroyed" | "error"
  SandboxInfo,
  SpawnOptions,
  ExecOptions,
  ExecResult,
  StreamChunk,
  FileInfo,
  Template,
  TemplateConfig,
  TemplateSpawnOverrides,
  ProviderInfo,
  HealthInfo,
  VMPoolStatus,
  ForgevmClientOptions,
} from "stacyvm";
```

Selected shapes:

```typescript
interface ExecResult {
  exit_code: number;
  stdout: string;
  stderr: string;
  duration: string;        // e.g. "127ms"
}

interface FileInfo {
  name: string;
  path: string;
  size: number;
  is_dir: boolean;
  mod_time: string;        // ISO 8601
  mode: string;            // octal, e.g. "0644"
}

interface SandboxInfo {
  id: string;
  state: SandboxState;
  provider: string;
  image: string;
  memory_mb: number;
  vcpus: number;
  created_at: string;
  expires_at: string;
  metadata: Record<string, string>;
  preview_domain?: string;
}

interface StreamChunk {
  stream: "stdout" | "stderr";
  data: string;
}
```

---

## Examples

Working examples live in [`examples/js/`](https://github.com/StacyOs/stacyvm/tree/main/examples/js):

- Basic spawn → exec → destroy
- Streaming output
- Live preview (Vite/Next.js inside a sandbox)
- Pool mode with multiple users
- Template-driven workflows

---

## Building from source

```bash
git clone https://github.com/StacyOs/stacyvm
cd stacyvm/sdk/js
npm install
npm run build       # compile TypeScript → dist/
npm test            # run vitest
```

---

## License

MIT — see [LICENSE](https://github.com/StacyOs/stacyvm/blob/main/LICENSE).
