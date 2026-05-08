# Threat Model

This threat model is the Phase 7 baseline. It focuses on StacyVM as a self-hosted sandbox control plane with local providers, live previews, SDK access, and an admin dashboard.

## Assets

- Host access to Docker, KVM, Firecracker, PRoot, and filesystem paths.
- Sandbox filesystem contents and user workspaces.
- API and admin API keys.
- SQLite database, audit logs, quotas, templates, environments, and registry metadata.
- Live-preview traffic and routing metadata.
- Registry credentials and build artifacts.

## Trust Boundaries

| Boundary | Risk |
|---|---|
| API client to StacyVM server | Unauthorized lifecycle, file, exec, or admin operations |
| StacyVM server to provider runtime | Container escape, VM misconfiguration, stale runtime ownership |
| Sandbox to host filesystem | Path traversal, workspace breakout, shared pool leakage |
| Live preview proxy to sandbox | Host header abuse, accidental exposure, cross-tenant preview routing |
| Admin dashboard to API | Key leakage from browser storage, overbroad operator access |
| Registry/environment builder | Secret leakage, malicious image build inputs, supply-chain drift |

## Primary Threats

| Threat | Current mitigation | Remaining work |
|---|---|---|
| Regular API key accesses admin routes | Admin key separation and `admin:*` scope enforcement | Add OIDC/RBAC and per-route policy tests |
| Missing operator attribution | `X-User-ID` and admin fallback attribution | OIDC actor claims |
| Sandbox file path traversal | Manager pool scoping rejects traversal; Docker/PRoot/provider tests cover traversal cases | Continue platform conformance on live runtimes |
| Shell command injection | Explicit shell/argv execution modes; argv mode avoids shell interpolation | Expand SDK examples and conformance tests for every provider |
| Docker container escape | Dropped caps/seccomp/resource config supported | Harden defaults and certify gVisor/Kata |
| Stale runtime after restart | Startup reconciliation | Distributed leases for multi-worker |
| Audit gaps | Admin audit and operation audit persisted for sandbox lifecycle, exec, and file operations | Extend operation audit to every env/registry mutation route |
| Live preview exposure | Traefik label routing and docs | Host allowlist and preview auth options |
| Secret leakage in diagnostics | Redaction in diagnostics | Support bundle redaction tests |
| Single-node database loss | SQLite backup docs | Backup/restore test automation |

## Phase 7 Security Objectives

- Make production misconfiguration visible through `stacyvm doctor --production`.
- Remove ambiguous command execution semantics before recommending public workloads.
- Increase file API path traversal coverage. Done for manager scoping and provider boundaries.
- Extend persisted audit beyond admin routes. Done for sandbox lifecycle, exec, and file operations.
- Convert runtime conformance docs into repeatable host checks. Done with `scripts/certify-runtime.sh`.

## Non-Goals For Phase 7

- Multi-worker scheduling.
- Full OIDC/SSO implementation.
- Postgres store.
- Enterprise RBAC.

Those belong to later production stages after the single-node release candidate is hardened.
