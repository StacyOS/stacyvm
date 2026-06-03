# StacyVM SSH/PTY Access — Architecture Proposal

## Context

StacyVM is a self-hosted system (Daytona-like, but broader) that runs user
workspaces as **sandboxes** behind a `Provider` abstraction (Docker, Firecracker
microVMs via a guest agent over vsock, PRoot, Custom HTTP, Mock). A control plane
(`stacyvm serve`) talks to **workers** (embedded in-process or remote via signed
worker tokens + mTLS JSON-RPC at `/rpc`), which talk to providers, which talk to
sandboxes. Identity is already modeled (API key / OIDC-JWT / HMAC worker tokens →
roles → scopes → tenant + owner). Observability (zerolog, Prometheus, health,
admin audit) and HA (Postgres LISTEN/NOTIFY, remote workers) already exist.

**The gap:** today's only way "into" a sandbox is the exec path. It is
one-directional, has **no stdin, no PTY/TTY, no window resize, and is not
binary-safe** — `StreamChunk{Stream, Data string}` ([provider.go:38](internal/providers/provider.go#L38))
marshals output as a JSON string, and the WebSocket handler
([sandboxes.go:689](internal/api/routes/sandboxes.go#L689)) only streams
stdout/stderr. An interactive SSH shell needs a **bidirectional, binary-safe,
multiplexed** path from the client all the way down to a real kernel PTY next to
the user's shell process.

**Why now / intended outcome:** give users production-grade SSH access to their
sandboxes (`ssh`, `scp`, `rsync`, VS Code Remote-SSH, port forwarding) that works
**fully standalone in the open-source self-hosted product today**, and is portable
to a future separate first-party managed product, **Stacy Cloud**, via one-click
migration of a running workspace. (Analogy: StacyVM is to Stacy Cloud as Next.js is
to Vercel — the OSS you self-host anywhere, plus an optional first-party cloud.)
The SSH subsystem is therefore **self-contained per deployment** (zero dependency
on Stacy Cloud), and client-side addressing hides host:port so the same
`stacy ssh <workspace>` keeps working even after a workspace migrates to a
different deployment.

## Decisions (confirmed with user)

- **Front door:** support **both** a real SSH server on a dedicated port *and* an
  SSH-over-WebSocket tunnel through the existing HTTPS+auth. One SSH server
  implementation, two transports.
- **Auth:** support **both** user-registered SSH public keys *and* short-lived
  CA-signed certificates minted by the `stacy ssh` CLI after normal API/OIDC
  login. Both resolve to the existing user+tenant identity.
- **Providers:** design one PTY contract covering **all** providers; implement
  Docker first (reference), then Firecracker, then PRoot.
- **Features:** interactive shell + resize (core), plus port forwarding, SFTP/SCP,
  and VS Code/JetBrains remote in scope; session recording optional/config-gated.
- **Build scope:** lean working slice first (Docker shell end-to-end), then layer.

---

## High-level architecture: the SSH Gateway

Introduce a stateless **SSH Gateway** that lives in the control-plane binary
(`stacyvm serve`, behind an `ssh.enabled` flag). It is a *protocol terminator and
byte relay only* — it never runs user commands on the host.

```
                        ┌─────────────────────── control-plane binary ───────────────────────┐
 ssh / scp / VS Code ──TCP:2222──▶ SSH Gateway ──┐                                            │
 (stock clients)                  (x/crypto/ssh) │  resolve sandbox→worker (shared store)     │
                                                 │  authz: identity owns/tenant-matches sb    │
 stacy ssh <sb>  ──WSS through ───▶ same SSH ─────┤  renew sandbox lease on activity          │
 (CLI, no port)   existing API     server over    │                                            │
                  + auth           a ws net.Conn   ▼                                            │
                                          PTY relay (binary frames, multiplexed by session)    │
                                                 │                                             │
                                                 ├── local worker (in-process) ──┐            │
                                                 └── remote worker (mTLS WSS /rpc/pty) ──┐     │
                                                                                  │     │      │
                                                                                  ▼     ▼      │
                                                                            Provider PTY layer │
                                                                  Docker exec(Tty) │ FC agent  │
                                                                  vsock pty │ PRoot creack/pty │
                                                                                  ▼            │
                                                              real kernel PTY next to user shell│
                                                              (inside container / microVM)      │
                                                              └────────────────────────────────┘
```

**Key principle — the real PTY lives next to the process, never on the gateway.**
The gateway forwards bytes + control events (window size, signals) to wherever the
shell runs. This keeps the gateway provider-agnostic and stateless-ish, preserves
the existing isolation boundary (microVM / container / proot), and means a session
is pinned to the *worker that hosts the sandbox* while *any* gateway replica can
serve *any* session (routing is a store lookup, so no LB affinity needed).

**Location transparency.** *Within* a deployment, clients address a sandbox by
logical ID (`ssh <sandboxID>@gateway` or `stacy ssh <sandboxID>`); the gateway
resolves `SandboxRecord.WorkerID` → current worker at connect time, so moving a
sandbox between workers of the **same** deployment changes only the routing target.
*Across* deployments (self-hosted → Stacy Cloud) the host genuinely changes;
portability there is handled by the `stacy` CLI's deployment **profile/context**
indirection (Section 7), not by one gateway spanning both products.

---

## 1. SSH ↔ workspace/session integration

- A new `internal/ssh` package hosts the gateway. The SSH **username** carries the
  target: `ssh <sandboxID>@host` (also accept `<sandboxID>.<tenant>` and a friendly
  workspace name resolved via the store).
- On channel open the gateway calls a new orchestrator entrypoint
  `Manager.OpenPTYSession(ctx, sandboxID, identity, ptyReq)` (added to
  [manager.go](internal/orchestrator/manager.go)), mirroring the existing
  `ExecStream` local-vs-remote routing (`isRemoteOwnedSandbox` →
  `execStreamRemote` pattern). This returns a bidirectional `PTYSession`
  (read/write/resize/signal/close).
- **Lease renewal:** SSH I/O activity renews the sandbox lease (reuse the
  `worker.renew_lease` path) so an active session keeps the sandbox alive, exactly
  like exec keeps it warm. Idle sessions stop renewing → normal TTL reclaim.
- **Auto-wake on connect (optional, Firecracker):** if the sandbox is stopped,
  the gateway can trigger snapshot resume (~28ms) before bridging — a strong
  differentiator. Behind `ssh.auto_resume`.

## 2. PTY bridging (where the real PTY lives, per provider)

Add an **optional** provider capability interface (don't force every provider; same
pattern as `StatsReporter`/`SnapshotLister` in [provider.go](internal/providers/provider.go)):

```go
type PTYProvider interface {
    OpenPTY(ctx, sandboxID string, opts PTYOptions) (PTYSession, error)
}
type PTYOptions struct { Cmd []string; Env map[string]string; WorkDir, Term string; Cols, Rows uint16 }
type PTYSession interface {
    io.ReadWriteCloser            // stdin in, stdout(+stderr merged) out — binary-safe
    Resize(cols, rows uint16) error
    Signal(sig string) error
    Wait() (exitCode int, err error)
}
```

- **Docker** ([docker.go](internal/providers/docker.go)): `ContainerExecCreate`
  with `Tty:true, AttachStdin:true, AttachStdout:true` → `ContainerExecAttach`
  gives a single bidirectional stream (no stdcopy demux under Tty). Resize via
  `ContainerExecResize`. Docker allocates the PTY *inside* the container. Lowest
  effort; the reference implementation.
- **Firecracker** ([firecracker.go](internal/providers/firecracker.go) +
  [cmd/stacyvm-agent/main.go](cmd/stacyvm-agent/main.go)): extend the guest agent
  with PTY methods. The agent (PID 1 inside the VM) opens a real PTY with
  `github.com/creack/pty`, starts the shell on the slave, and relays the master
  over vsock. Add methods to [agentproto/protocol.go](internal/agentproto/protocol.go):
  `pty_open` (params: cmd/env/term/cols/rows → returns session id), `pty_data`
  (**bidirectional, binary**), `pty_resize`, `pty_signal`, `pty_close`,
  `pty_exit`. Multiplex by `session_id` so one vsock conn carries many PTYs.
- **PRoot** ([proot.go](internal/providers/proot.go)): host-side `creack/pty`
  wrapping the proot-jailed shell; PTY lives in the proot namespace.
- **Custom/E2B/Mock:** Custom HTTP gets an optional PTY upgrade endpoint; Mock
  implements an in-memory echo PTY for tests; providers that don't implement
  `PTYProvider` return a clean "SSH not supported on this provider" error.

**Wire format for PTY data (hot path).** Do **not** reuse `StreamChunk` (JSON
string, binary-unsafe, 33% base64 bloat if patched). Instead add a small **binary
frame** alongside the existing 4-byte length-prefixed framing in
[agentproto/io.go](internal/agentproto/io.go): `[len][type][session_id][payload]`,
where `type ∈ {DATA, RESIZE, SIGNAL, CLOSE, EXIT}`. Control frames (open/resize/
signal) stay JSON; only DATA is raw bytes. Same frame vocabulary is reused on the
control-plane↔worker hop (below), so there is one mental model end-to-end.

## 3. Authentication & authorization

**Transport: one SSH server, two ways to reach it.**
- Native TCP listener on `ssh.listen_addr` (default `:2222`).
- SSH-over-WebSocket: a new authenticated endpoint
  (`GET /api/v1/ssh/connect`) upgrades to WS, wraps the WS as a `net.Conn`, and
  hands it to the *same* `x/crypto/ssh` server. The WS handshake is gated by the
  existing API-key/OIDC middleware, so the tunnel inherits all current auth +
  rate-limiting + tenant scoping for free. `stacy ssh` and ProxyCommand use this.

**SSH-level authentication (`PublicKeyCallback`):**
- **Registered keys:** new `SSHKeyRecord{ID, Subject/OwnerID, TenantID, Fingerprint,
  PublicKey, Label, CreatedAt}` in [store.go](internal/store/store.go) + migration.
  CRUD under `/api/v1/users/me/ssh-keys` and tenant-member keys. Callback looks up
  by SHA256 fingerprint → resolves `AuthIdentity` (subject, tenant, scopes).
- **CA-signed certs:** the control plane is an SSH **User CA**. The `stacy ssh` CLI
  authenticates with the existing API key/OIDC, calls `POST /api/v1/ssh/certs`
  (params: target sandbox), and the server mints a short-lived OpenSSH user
  certificate (principal = subject, ~10 min TTL, `critical-option`/extension
  binding it to the sandbox + tenant). The callback validates the cert against the
  CA and trusts its embedded principal. This mirrors the existing HMAC worker-token
  issuance ([auth.go:337](internal/api/middleware/auth.go#L337)) and gives
  revocation-by-expiry. Optional KRL for emergency revoke (same shape as the
  worker-token `RevokedTokenIDs` list).
- **Host key:** stable Ed25519 host key persisted in the state dir / DB / secret so
  restarts and HA replicas present the same identity (no `known_hosts` churn).

**Authorization** (reuses existing model):
- New scope `ssh:*` added in [auth.go](internal/api/middleware/auth.go) `scopesForRole`
  (granted to `api`+ roles). Cert/key must carry it.
- After SSH auth resolves identity, the gateway runs the existing ownership/tenant
  check (the `checkTenantAccess` logic in [sandboxes.go](internal/api/routes/sandboxes.go))
  against the target sandbox before bridging. Cross-tenant access is impossible by
  construction (username target + identity tenant must match the `SandboxRecord`).
- Per-sandbox policy (in `SandboxRecord.Metadata` or a new policy table):
  allow/deny SSH, allowed principals/groups, max concurrent sessions,
  port-forward allowed, recording required.

## 4. Session isolation & security boundaries

- **Gateway runs unprivileged**, executes nothing user-supplied on the host; it is
  a relay. systemd hardening already present (`NoNewPrivileges`, `ProtectSystem=
  strict`, `PrivateTmp`) applies.
- **One session → one sandbox.** All multiplexed channels stay within that
  sandbox's namespaces; the PTY runs as the sandbox's unprivileged user, bounded by
  the sandbox's existing CPU/memory caps and the provider isolation (Firecracker
  microVM > Docker dropped-caps+seccomp > PRoot).
- **SSH feature lockdown by default:** disable agent forwarding and X11; root login
  off by policy; `direct-tcpip` (port forwarding) and SFTP gated by policy flags;
  enforce login grace timeout, max-auth-tries, max channels/session, max packet
  size, keepalive. Cipher/kex/MAC allowlist (modern only).
- **Lifecycle caps:** idle timeout, max session duration, global + per-tenant +
  per-sandbox concurrent-session limits.

## 5. Scaling, reliability, observability

- **Horizontal scale:** gateway replicas are interchangeable behind an **L4 (TCP)**
  load balancer for the native port (SSH is not HTTP) and behind the existing
  reverse proxy for the WS path. No session affinity needed — routing is a store
  lookup, sessions pin to the worker, not the gateway.
- **Reliability:** SSH keepalives detect dead peers; bounded per-session buffers +
  flow control prevent a slow client from OOMing the gateway; graceful drain on
  deploy (stop accepting new, let existing idle out). Worker restart drops its
  sessions → client reconnects (server-side tmux-style persistence is a future
  add). Backpressure propagates through the relay (don't buffer unboundedly).
- **Observability:** new Prometheus metrics in
  [prometheus.go](internal/api/routes/prometheus.go): `stacyvm_ssh_sessions_active`,
  `_sessions_total`, `_auth_failures_total{method}`, `_bytes_total{dir}`,
  session-duration histogram, per-worker PTY-channel gauge. Per-session structured
  zerolog with session/request ID correlation. `SSHSessionRecord` (sandbox, owner,
  tenant, auth method, key fingerprint, client IP, start/end, bytes, exit) +
  admin-audit entries for open/close/auth. Readiness gate checks host key loaded +
  store reachable.

## 6. Networking & connection lifecycle

- **CP ↔ worker PTY transport:** the existing `/rpc` is request/response JSON over
  HTTP POST — unfit for a long-lived full-duplex PTY. Add a streaming endpoint
  `GET /rpc/pty` on the worker ([worker/rpc.go](internal/worker/rpc.go),
  [rpc_client.go](internal/worker/rpc_client.go)) upgraded to **WebSocket over the
  existing mTLS + worker-token auth**, carrying the same binary frame vocabulary as
  the agent hop. Add `pty_*` methods + scope `worker:pty` to
  [workerproto/protocol.go](internal/workerproto/protocol.go) and
  `scopesForRole(AuthRoleWorker)`.
- **Connection lifecycle:** TCP/WS accept → SSH handshake (host key, kex) → auth
  (login grace) → channel open → `pty-req` (term/cols/rows/modes) →
  `shell`/`exec`/`subsystem` → bridge established → data + `window-change` events →
  close on client exit / signal / sandbox death / timeout → teardown (close agent
  PTY, free worker stream, write audit + metrics, stop lease renewal).
- **Port forwarding / SFTP / IDE** ride the same SSH connection: `direct-tcpip`
  channels relay to `host:port` inside the sandbox (policy-gated); the `sftp`
  subsystem maps onto existing file ops (or a guest sftp binary); VS Code/JetBrains
  work once `exec` channels + SFTP + port-forward exist.

## 7. Self-hosted deployment & cloud portability

**Self-hosted (now):**
- New `ssh:` config block in [config.go](internal/config/config.go):
  `enabled`, `listen_addr`, `host_key_path` (or secret ref), `user_ca_path`,
  `auth_methods` (publickey/cert), `idle_timeout`, `max_session_duration`,
  `max_sessions_per_sandbox`, `allow_port_forward`, `allow_sftp`,
  `session_recording`, cipher/kex/mac allowlists. Add production-lint rules in
  [cmd_config.go](cmd/stacyvm/cmd_config.go) (host key set, ports bounded, recording
  retention sane).
- Deploy: bind `:2222` by default (no privilege); to use `:22` add
  `AmbientCapabilities=CAP_NET_BIND_SERVICE` in
  [deploy/stacyvm.service](deploy/stacyvm.service) or front with a proxy. Expose the
  port in [docker-compose.yml](docker-compose.yml) /
  [deploy/docker-compose.yml](deploy/docker-compose.yml). Persist host key + CA in
  `/var/lib/stacyvm/`.
- `stacy ssh <sandbox>` CLI (new command under [cmd/stacyvm/](cmd/stacyvm/)):
  fetches a cert (or uses a registered key), connects via native port or WS tunnel,
  and can write a `~/.ssh/config` `ProxyCommand`/`Host` block so plain `ssh`, `scp`,
  `rsync`, and VS Code Remote-SSH "just work." Cross-platform: the CLI runs on
  Linux/macOS/Windows; the gateway + PTY are Linux (Firecracker/agent are
  `//go:build linux`).

**Portability to Stacy Cloud (separate future product):**
StacyVM (this OSS) is fully self-contained — its SSH subsystem has **zero
dependency** on Stacy Cloud. Stacy Cloud is a distinct managed deployment with its
*own* control plane, gateway, host key, user CA, and identity/tenant store.
One-click migration moves a running workspace **across** deployments, so its SSH
endpoint legitimately changes host. We make that seamless on the **client** side,
not by spanning a single gateway across products:
- **CLI profile/context indirection:** `stacy ssh <workspace>` resolves through a
  named deployment profile in `~/.stacy/contexts` (kubeconfig-style). After
  migration the workspace's profile re-points to Stacy Cloud; the *same* command
  reaches its new home — fetching a fresh cert from the destination CA and
  rewriting `known_hosts`/`ssh-config` automatically.
- **Per-deployment trust roots:** each deployment owns its host key + user CA
  (clean isolation, no shared secrets across products). Optional CA cross-trust can
  smooth the cutover but is not required by core.
- **Stable workspace identity:** migration preserves the logical workspace
  name/ID, so connection strings and `ssh-config` Host blocks survive the move;
  only the resolved endpoint changes.

## 8. Production hardening & failure handling

- **Abuse defense:** login grace timeout, max-auth-tries, per-IP connection rate
  limiting (reuse the ratelimit middleware concept), fail2ban-friendly auth logs,
  constant-time credential comparison (existing pattern).
- **Resource safety:** global/per-tenant/per-sandbox session caps; bounded buffers;
  one goroutine-set per session with context cancellation; frame-size limits; fuzz
  the frame parser.
- **Failure modes:** worker unreachable → clean "sandbox unavailable" to client;
  sandbox stopped → auto-resume (FC) or reject per config; agent PTY crash → close
  channel with exit status; gateway crash → sessions drop, client reconnects;
  slow consumer → backpressure + timeout, never unbounded buffering.
- **Secrets/keys:** host-key rotation procedure documented; cert revocation via
  short TTL + optional KRL; no host filesystem exposure; sanitized env into the
  sandbox.

---

## Concrete change map (grounded in current code)

New:
- `internal/ssh/` — gateway, x/crypto/ssh server, auth callbacks, session relay,
  WS-as-net.Conn adapter, port-forward + sftp handlers.
- `internal/ssh/ca/` — user CA sign/verify (mirrors worker-token issuance).

Modify:
- [internal/providers/provider.go](internal/providers/provider.go) — add optional
  `PTYProvider` + `PTYSession` + `PTYOptions`.
- [internal/providers/docker.go](internal/providers/docker.go) — Tty exec +
  resize.
- [internal/providers/firecracker.go](internal/providers/firecracker.go),
  [cmd/stacyvm-agent/main.go](cmd/stacyvm-agent/main.go),
  [internal/agentproto/protocol.go](internal/agentproto/protocol.go),
  [internal/agentproto/io.go](internal/agentproto/io.go) — vsock PTY methods +
  binary DATA frame.
- [internal/providers/proot.go](internal/providers/proot.go) — creack/pty.
- [internal/workerproto/protocol.go](internal/workerproto/protocol.go),
  [internal/worker/rpc.go](internal/worker/rpc.go),
  [internal/worker/rpc_client.go](internal/worker/rpc_client.go) — `/rpc/pty`
  WS stream + `worker:pty` scope.
- [internal/orchestrator/manager.go](internal/orchestrator/manager.go) —
  `OpenPTYSession` local/remote routing + lease renewal on activity.
- [internal/api/middleware/auth.go](internal/api/middleware/auth.go) — `ssh:*`
  scope.
- [internal/api/routes/](internal/api/routes/) — SSH key CRUD, cert issuance,
  `/api/v1/ssh/connect` WS endpoint, SSH metrics in
  [prometheus.go](internal/api/routes/prometheus.go).
- [internal/store/store.go](internal/store/store.go) + migrations —
  `SSHKeyRecord`, `SSHSessionRecord`, host-key/CA storage.
- [internal/config/config.go](internal/config/config.go) +
  [cmd/stacyvm/cmd_config.go](cmd/stacyvm/cmd_config.go) — `ssh:` block + lint.
- [cmd/stacyvm/](cmd/stacyvm/) — `stacy ssh` command + ssh-config writer.
- [deploy/stacyvm.service](deploy/stacyvm.service),
  [docker-compose.yml](docker-compose.yml),
  [deploy/stacyvm.production.yaml](deploy/stacyvm.production.yaml) — port + key
  persistence.

Reuse: existing auth middleware + scopes, tenant/owner checks, worker token + mTLS,
lease renewal, zerolog/Prometheus/health, Postgres HA, snapshot resume.

## Phased roadmap (lean slice first)

1. **Vertical slice — Docker interactive shell.** `PTYProvider` + Docker impl;
   `OpenPTYSession`; `internal/ssh` gateway with native port; registered-key auth;
   ownership/tenant check; `ssh:*` scope; resize; lease renewal; basic metrics +
   audit. **Outcome:** `ssh sb@host` gives a real bash in a Docker sandbox.
2. **Firecracker + PRoot PTY.** agent vsock PTY methods + binary frame; `/rpc/pty`
   WS for remote workers; PRoot creack/pty. Provider conformance tests for PTY.
3. **Access UX + auth depth.** CA cert issuance + `stacy ssh` CLI + ssh-config
   writer; SSH-over-WebSocket tunnel; VS Code Remote-SSH validated.
4. **Power features.** Port forwarding (direct-tcpip), SFTP/SCP subsystem,
   per-sandbox policy, optional session recording.
5. **HA + hardening + Stacy Cloud portability.** Multi-replica gateway behind L4
   LB, graceful drain, KRL revocation, full hardening + fuzzing; `stacy` CLI
   context/profile indirection so a workspace migrated to Stacy Cloud stays
   reachable via the same command (+ optional CA cross-trust).

## Verification

- **Unit/integration:** Mock PTY provider; assert binary-safe round-trip (send
  random bytes incl. NUL/UTF-8 edge cases), resize propagation, exit-code
  fidelity, concurrent multiplexed sessions, idle/duration timeout, auth
  accept/reject (key + cert + wrong-tenant rejection).
- **Provider conformance:** extend `provider_conformance_test.go` with a PTY suite
  run against Docker (and Firecracker/PRoot when present).
- **End-to-end manual:** `ssh sb@localhost -p 2222` → interactive bash; `top`/`vim`
  render correctly; window resize works; `scp`/`rsync` a file; `ssh -L` to a dev
  server inside the sandbox; VS Code Remote-SSH attaches; `stacy ssh sb` via WS
  tunnel with no open port; kill the worker mid-session and confirm clean
  client-side error.
- **Security:** confirm gateway runs no host commands; cross-tenant `ssh` rejected;
  agent forwarding/X11 disabled; host key stable across restart; cert TTL expiry
  rejects reuse; metrics + audit records emitted.

## Open questions / future

- Server-side session persistence (reconnect to a live shell after disconnect)?
- Optional CA cross-trust between a self-hosted instance and Stacy Cloud to smooth
  the migration cutover (vs. always re-issuing certs from the destination CA).
- SFTP backed by guest binary vs. synthesized from existing file ops.
