# Phase 5 Admin Control Plane Release Notes

Date: 2026-05-08
Branch: `phase-5-admin-control-plane`

## Summary

Phase 5 delivers the first operator/admin control plane for StacyVM. This phase builds on Phase 3 quotas and Phase 4 production deployment by separating admin access from regular API usage and preparing the API surface for safer dashboard-driven operations.

## What Changed

### Admin Authentication

- Added optional `auth.admin_api_key` config.
- Added `STACYVM_AUTH_ADMIN_API_KEY` environment variable support through the existing config loader.
- Added `X-Admin-API-Key` support for admin requests.
- Admin keys can authenticate normal API requests, but normal API keys cannot access admin routes when an admin key is configured.
- When no admin key is configured, admin routes fall back to `auth.api_key` for backwards compatibility.

### Admin Route Namespace

- Added `/api/v1/admin/*` operator route aliases for:
  - providers
  - quotas
  - diagnostics
  - JSON metrics
  - Prometheus metrics
- Existing non-admin routes remain available for compatibility during this phase.

### Deployment And Docs

- Added admin key examples to production config, Compose env, systemd env, README, deployment docs, and API docs.
- Added an admin control-plane operator guide covering dashboard setup, quotas, diagnostics, audit export, and storage notes.

### Dashboard Admin Workflows

- Added dashboard settings for a regular API key and a separate admin API key.
- The shared web API client now sends `X-API-Key` and `X-Admin-API-Key` from browser settings.
- Provider list, provider detail, provider health tests, and JSON metrics now call `/api/v1/admin/*`.
- Provider cards now understand the backend `default`, latency, runtime count, capability, and error fields.
- Added an Operations dashboard page for owner quota management and diagnostics.
- Added dashboard workflows for quota list, save, delete, summary, owner usage checks, and redacted diagnostics.

### Admin Audit History

- Added persisted admin audit logs for admin route access.
- Added `/api/v1/admin/audit` to list recent redacted admin audit records.
- Records include actor, method, path, status, duration, request ID, client address, user agent, and timestamp.
- Added an Audit tab to the Operations dashboard.
- Added audit filters for actor, HTTP method, status, and path substring.
- Added CSV export for filtered audit history.
- Added `auth.admin_audit_retention` for native audit log pruning.
- Production templates keep 90 days of admin audit history with `2160h`.

## Verification

```sh
go test ./...
npm run build
```

GitHub CI passed on `phase-5-admin-control-plane` for:

- Go tests and CLI build
- Swagger drift check
- Python SDK import check
- TypeScript SDK build
- Deployment smoke test
- Web build

## Release Status

Phase 5 is complete and published as the `phase-5-admin-control-plane` GitHub release.
