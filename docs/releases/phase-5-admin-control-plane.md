# Phase 5 Admin Control Plane Release Notes

Date: 2026-05-08
Branch: `phase-5-admin-control-plane`

## Summary

Phase 5 starts the operator/admin control-plane work for StacyVM. This phase builds on Phase 3 quotas and Phase 4 production deployment by separating admin access from regular API usage and preparing the API surface for safer dashboard-driven operations.

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

## Verification

```sh
go test ./internal/api/middleware ./internal/config ./cmd/stacyvm
```

## Next Phase 5 Direction

The next slice should move dashboard quota/provider/diagnostics workflows onto the admin namespace and add persisted audit history for admin operations.
