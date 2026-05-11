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
| Regular API key accesses admin routes | Admin key separation, `admin:*` scope enforcement, OIDC/JWT RS256 Bearer auth with group-to-role mapping (viewer/operator/admin/tenant_admin), and per-resource policy enforcement for image/provider/network controls | Expand per-route policy tests for every provider type |
| Missing operator attribution | `X-User-ID`, admin fallback attribution, and OIDC `sub`/`email` claims injected into `AuthIdentity` and written to audit records | None; OIDC actor claims implemented |
| Sandbox file path traversal | Manager pool scoping rejects traversal; Docker/PRoot/provider tests cover traversal cases | Continue platform conformance on live runtimes |
| Shell command injection | Explicit shell/argv execution modes; argv mode avoids shell interpolation | Expand SDK examples and conformance tests for every provider |
| Docker container escape | Dropped caps/seccomp/resource config supported | Harden defaults and certify gVisor/Kata |
| Stale runtime after restart | Startup reconciliation | Distributed leases for multi-worker |
| Worker impersonation | Worker RPC contract separates worker identity from user/admin identity; signed worker tokens enforce worker ID, token ID, audience, expiry, revocation, and worker-only scopes; worker RPC mTLS is wired for transport identity; centralized token issuance via `/api/v1/admin/worker-tokens` removes the need for workers to hold the signing key directly | Target-network mTLS smoke with deployment-issued certificates |
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

Those belonged to later production stages after the single-node release candidate was hardened.

## Phase 14 Security Additions

- OIDC/JWT RS256 Bearer token auth with JWKS and configurable issuer, audience, and group-to-role mapping.
- RBAC roles: viewer, operator, admin, tenant_admin with scoped permission sets.
- Tenant/project model: resource isolation per tenant, per-tenant audit export.
- Policy controls: image, provider, and network allow-deny enforcement at spawn time.
- Centralized worker token issuance: workers obtain signed tokens from the control plane without holding the signing key.
- Sandbox tenant scoping: List and Get enforce tenant boundaries for OIDC-authenticated callers.
