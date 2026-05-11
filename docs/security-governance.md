# Security Governance

This guide captures the Phase 6 security posture for production StacyVM operators and the planned shape of future external identity integration.

## Production Admin Posture

Use separate credentials for regular API clients and admin operators:

```yaml
auth:
  enabled: true
  api_key: "sk-client"
  admin_api_key: "sk-admin"
  admin_fallback_enabled: false
  admin_audit_retention: "2160h"
```

Production deployments should also set the equivalent environment variables:

```bash
STACYVM_AUTH_API_KEY=sk-client
STACYVM_AUTH_ADMIN_API_KEY=sk-admin
STACYVM_AUTH_ADMIN_FALLBACK_ENABLED=false
STACYVM_AUTH_ADMIN_AUDIT_RETENTION=2160h
```

Keep admin routes under `/api/v1/admin/*` behind trusted networks or a reverse proxy allowlist. The admin dashboard should only be used from managed operator browsers because its settings are stored in browser local storage.

## Operator Attribution

Admin audit records use `X-User-ID` as the operator actor when it is supplied. Set it to a stable human or service identity such as `operator-a`, `sre-oncall`, or `ops-bot`.

When `X-User-ID` is missing, authenticated admin requests fall back to role and key-header attribution such as `admin:X-Admin-API-Key`. This is useful for debugging, but production operators should still send explicit actors for accountability.

## Key Handling

- Generate `auth.api_key` and `auth.admin_api_key` independently with at least 32 bytes of entropy.
- Keep keys in environment-specific secret storage rather than checked-in config.
- Rotate the admin key after operator offboarding, dashboard sharing incidents, or suspected local browser compromise.
- Prefer short-lived deployment access to the host and avoid copying admin keys into issue trackers, screenshots, or shared terminals.
- Keep `auth.admin_fallback_enabled: false` in production so regular API keys never become admin credentials.

## Audit Retention

Audit logs are stored in SQLite with the rest of StacyVM state. `auth.admin_audit_retention` controls native pruning after successful admin audit writes.

Recommended starting points:

| Environment | Retention |
|---|---|
| Local development | `0s` |
| Staging | `720h` |
| Production | `2160h` |

Back up the SQLite database before upgrades and before reducing retention windows.

## OIDC/SSO Groundwork

Phase 6 keeps API-key behavior as the implemented authentication mechanism. Future OIDC/SSO support should fit into the request identity model added in this phase instead of bypassing it.

Proposed config shape:

```yaml
auth:
  oidc:
    enabled: false
    issuer_url: "https://idp.example.com"
    client_id: "stacyvm"
    audience: "stacyvm-api"
    admin_groups:
      - "stacyvm-admins"
    api_groups:
      - "stacyvm-users"
    actor_claim: "email"
    groups_claim: "groups"
```

Expected claim mapping:

| Claim/Input | StacyVM identity |
|---|---|
| Admin group membership | `admin` role with `api:*` and `admin:*` scopes |
| API group membership | `api` role with `api:*` scope |
| Actor claim | Audit actor, replacing the need for user-supplied `X-User-ID` |
| Subject claim | Stable fallback identity when the actor claim is absent |

Implementation boundaries:

- Validate issuer, audience, expiry, and signature before creating an `AuthIdentity`.
- Reuse `RequireScope` for authorization decisions.
- Keep API-key auth available for service accounts and break-glass access.
- Make dashboard SSO optional and separate from API key support.
- Record the identity source in audit attribution so operators can distinguish API-key and OIDC-originated admin actions.

## Phase 6 Acceptance Checklist

- `auth.admin_api_key` is configured separately from `auth.api_key`.
- `auth.admin_fallback_enabled` is `false` in production.
- Admin ingress is restricted to trusted networks or authenticated upstreams.
- Operators send `X-User-ID` until OIDC supplies actor claims.
- `auth.admin_audit_retention` is set to a production retention window.
- Backups include the SQLite database before retention or upgrade changes.
