# Phase 6 Security Governance Release Notes

Date: 2026-05-08
Branch: `phase-6-security-governance`

## Summary

Phase 6 starts the security and governance layer above the Phase 5 admin control plane. The first slice keeps current API-key deployments compatible while adding typed request identity metadata that future RBAC, SSO/OIDC, and policy enforcement can build on.

## What Changed

### Request Identity Foundation

- Added request-scoped authentication identities in the API middleware.
- Added explicit `api` and `admin` roles.
- Added initial scope metadata:
  - `api:*` for regular API identities.
  - `api:*` and `admin:*` for admin identities.
- Admin keys used through either supported key header are now represented as admin identities.
- Regular API keys remain regular API identities and still cannot access admin routes when `auth.admin_api_key` is configured.

### Route-Level Scope Enforcement

- Added a reusable `RequireScope` middleware.
- Wired authenticated admin routes through `RequireScope("admin:*")`.
- Kept unauthenticated development mode behavior unchanged when no API keys are configured.

### Audit Attribution

- Admin audit fallback attribution now reads the authenticated role from request context when no `X-User-ID` actor is supplied.
- Existing explicit actor behavior is preserved: `X-User-ID` still wins when supplied.

## Compatibility

- No deployment config changes are required.
- Existing `X-API-Key` and `X-Admin-API-Key` behavior is preserved.
- Admin route fallback to `auth.api_key` remains available when no separate `auth.admin_api_key` is configured.

## Verification

```sh
go test ./internal/api/middleware
go test ./internal/api
go test ./...
npm run build
```

## Next Phase 6 Direction

The next slices should continue the governance hardening: safer admin actor attribution, configurable admin compatibility modes, production security guidance, and then external identity integration planning for OIDC/SSO.
