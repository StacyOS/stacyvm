# Admin Control Plane

This guide covers the operator-facing admin surface added in Phase 5: separate admin authentication, dashboard Operations workflows, owner quota management, diagnostics, and persisted admin audit history.

## Authentication

StacyVM supports a regular API key and an optional separate admin API key:

```yaml
auth:
  enabled: true
  api_key: "sk-client"
  admin_api_key: "sk-admin"
  admin_audit_retention: "2160h"
```

The same values can be supplied with environment variables:

```bash
STACYVM_AUTH_API_KEY=sk-client
STACYVM_AUTH_ADMIN_API_KEY=sk-admin
STACYVM_AUTH_ADMIN_AUDIT_RETENTION=2160h
```

Use `X-Admin-API-Key` for `/api/v1/admin/*` requests. If `auth.admin_api_key` is configured, the regular `auth.api_key` cannot access admin routes. If no admin key is configured, admin routes fall back to `auth.api_key` for compatibility.

Authenticated requests now carry a request-scoped identity with either the `api` or `admin` role. Admin identities receive both `api:*` and `admin:*` scopes; regular API identities receive `api:*`. This keeps the current API-key behavior stable while creating a typed authorization boundary for later RBAC and identity-provider integrations.

When API-key auth is enabled, admin routes also enforce the `admin:*` scope at the route layer. This is intentionally redundant with admin-key authentication today and gives future RBAC/OIDC integrations a single policy hook to satisfy.

## Dashboard Setup

Open Settings, enable API key sending, and set:

- API Key: sent as `X-API-Key` for regular API requests.
- Admin API Key: sent as `X-Admin-API-Key` for operator routes.

Settings are stored in browser local storage. Treat operator browsers as privileged clients and avoid sharing screenshots or profiles that may expose local configuration.

## Operations Dashboard

The Operations page has three tabs:

| Tab | Purpose |
|---|---|
| Quotas | List, create, update, delete, summarize, and inspect owner quota usage. |
| Diagnostics | View redacted process, scheduler, rate-limit, build, provider, store, sandbox, and redaction data. |
| Audit | Review, filter, and export persisted admin route access logs. |

## Owner Quotas

Owner quotas are persisted overrides keyed by owner ID. They apply to requests that carry an owner identity through `X-User-ID` or an `owner_id` field where supported.

Fields:

| Field | Meaning |
|---|---|
| `max_sandboxes` | Maximum active sandboxes for the owner. `0` means no override. |
| `max_ttl` | Maximum sandbox TTL for the owner, such as `30m` or `2h`. |
| `max_exec_timeout` | Maximum exec timeout for the owner, such as `30s`. |

Quota changes are admin operations and are recorded in the admin audit log.

## Diagnostics

Diagnostics are redacted by design. They include operational state useful for support and production checks while avoiding raw API keys, provider secrets, registry credentials, and environment secrets.

Use diagnostics during incidents to confirm:

- Store health and latency.
- Provider health and capabilities.
- Scheduler and spawn queue state.
- Rate-limit state.
- Sandbox count by state/provider.
- Quota coverage summary.

## Admin Audit

Admin route access is persisted in SQLite in the `admin_audit_logs` table. Records include:

- Actor from `X-User-ID`, or `admin` when no actor header is supplied.
- HTTP method and path.
- HTTP status and request duration.
- Request ID.
- Client address and user agent.
- Timestamp.

The Audit tab supports filters for actor, method, status, and path substring. It can export the current filtered view as CSV. The API equivalent is:

```bash
curl \
  -H "X-Admin-API-Key: $STACYVM_ADMIN_API_KEY" \
  "https://stacyvm.example.com/api/v1/admin/audit?actor=operator-a&method=PUT&format=csv"
```

## Storage And Retention

Audit logs live in the main SQLite database. They are included in normal database backups. Native retention is controlled by `auth.admin_audit_retention`.

Set the value to a Go duration such as `720h` for 30 days or `2160h` for 90 days. `0s` disables native pruning. Pruning runs after successful admin audit writes, so records older than the retention window are removed as operators use the admin control plane.
