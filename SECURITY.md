# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in StacyVM, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, [open a private security advisory](https://github.com/StacyOs/stacyvm/security/advisories/new) on GitHub.

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

## Security Model

### Sandbox isolation

- Each Firecracker sandbox runs in its own KVM virtual machine with a dedicated kernel
- No shared kernel between sandboxes or between sandbox and host
- Per-VM rootfs — destroyed on teardown
- Host-guest communication via virtio-vsock (no network exposure)
- Docker sandboxes use dropped capabilities (`CAP_ALL`), seccomp, pids limit, and memory/CPU limits
- TTL-based auto-expiry prevents resource leaks

### Authentication and authorisation

- **API key auth** — static API and admin keys; `stacyvm config lint --production` enforces minimum entropy and key separation
- **OIDC/JWT auth** — RS256 and ES256/ES384/ES512 Bearer token verification with configurable JWKS endpoint, audience, and issuer; supports Google Workspace, Okta, Cloudflare Access, Azure AD, and any RFC 7517-compliant IdP
- **RBAC** — `viewer` (read-only), `api`, `operator`, `admin`, `tenant_admin` roles with scope enforcement on every API route; OIDC group-to-role mapping is configurable
- **Multi-tenancy** — sandboxes, audit logs, and policies are scoped to tenants; cross-tenant access returns 404

### Worker security

- Remote workers authenticate with HMAC-SHA256 signed tokens (`stacyvm-worker-v1` format) or individually rotatable static tokens
- Worker RPC supports mutual TLS (mTLS) for network-level transport identity
- Signed tokens carry `worker_id`, `jti`, `aud`, `exp`, `iat`; emergency revocation is supported via `auth.worker_revoked_token_ids`
- Workers can receive short-lived signed tokens from the centralized issuer (`POST /api/v1/admin/worker-tokens`) without needing direct access to the signing key

### Audit

- All admin operations are persisted in `admin_audit_logs` with actor, method, path, status, and tenant
- All sandbox lifecycle, exec, and file operations are persisted in `operation_audit_logs`
- Per-tenant audit export is available at `GET /api/v1/admin/tenants/{id}/audit`
- Audit records redacted automatically in support bundles

## Scope

The following are in scope for security reports:

- VM or container escape vulnerabilities
- API authentication or authorisation bypass
- OIDC JWT validation bypass (algorithm confusion, `alg: none`, signature skip)
- Cross-tenant data access
- Unauthorized access to host filesystem or network
- Denial of service via resource exhaustion
- Command injection through the API or exec interface
- Worker impersonation or signed-token forgery

## Out of Scope

- The mock provider intentionally runs commands on the host and is for development only
- Self-hosted deployments where the operator has explicitly disabled auth (`auth.enabled: false`) — this is flagged as a production failure by `stacyvm doctor --production` and `stacyvm config lint --production`
- Vulnerabilities in third-party runtimes (Docker, Firecracker, gVisor, Kata) that are outside StacyVM's control
