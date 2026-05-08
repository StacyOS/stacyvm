# Phase 4 Production Deployment Release Notes

Date: 2026-05-08
Branch: `phase-4-production-deployment`

## Summary

Phase 4 starts turning the production control-plane work from earlier phases into repeatable shipping and deployment workflows. This checkpoint adds GitHub Actions CI coverage plus operator-facing deployment templates and runbooks for single-node production installations.

The goal of this phase is to make StacyVM easier to validate, release, and run outside a developer laptop while keeping host-specific runtime conformance explicit.

## What Changed

### Continuous Integration

- Added a GitHub Actions workflow for core project verification.
- CI now validates:
  - Go tests across the repository.
  - CLI build.
  - Swagger/OpenAPI drift.
  - Web dashboard production build.
  - TypeScript SDK build.
  - Python SDK package install, compile, and import.
- Stabilized the Swagger drift check for cold CI runners by downloading Go modules before invoking `swag`.

### Production Deployment Templates

- Added `deploy/docker-compose.yml` for a production-oriented Docker provider deployment with Traefik live-preview routing.
- Added `deploy/stacyvm.production.yaml` with production defaults for:
  - API auth.
  - API rate limiting.
  - sandbox caps and queue backpressure.
  - JSON logging.
  - persistent SQLite state.
  - Docker as the default provider.
- Added Compose and systemd environment templates:
  - `deploy/.env.example`
  - `deploy/stacyvm.env.example`
- Added `deploy/stacyvm.service` for binary-based Linux/systemd installs.

### Release Automation

- Added a release workflow for tag-driven and manually-dispatched releases.
- Release automation builds static Linux binary artifacts for `amd64` and `arm64`.
- Release automation publishes multi-arch container images to `ghcr.io/stacyos/stacyvm`.
- Docker image builds now accept an explicit `VERSION` build argument.
- Release artifacts now build into `dist/` instead of the repository root.
- Added `.dockerignore` to keep local build outputs and dependency directories out of release image contexts.

### Deployment Runbook

- Added `docs/deployment.md` covering:
  - host requirements.
  - Docker Compose deployment.
  - systemd deployment.
  - health, readiness, liveness, and Prometheus endpoints.
  - reverse proxy expectations.
  - SQLite backup and restore basics.
  - upgrade procedure.
  - Docker, Firecracker, and PRoot provider notes.
- Linked the deployment guide from the README.

## Code Changes By Area

### CI

- `.github/workflows/ci.yml`
  - Adds repository verification jobs for Go, Swagger, web, and SDKs.
  - Opts into Node 24-based JavaScript actions to address the GitHub Actions Node 20 deprecation warning.
- `.github/workflows/release.yml`
  - Adds binary and container image release automation.
- `scripts/check-swagger.sh`
  - Downloads modules before generating docs in a temporary workspace.

### Deployment

- `deploy/docker-compose.yml`
  - Adds a reusable production Compose template.
- `deploy/stacyvm.production.yaml`
  - Adds a production baseline config.
- `deploy/stacyvm.service`
  - Adds a systemd unit for running the StacyVM binary.
- `deploy/.env.example` and `deploy/stacyvm.env.example`
  - Add environment templates for Compose and systemd.
- `Dockerfile`
  - Adds BuildKit platform args and explicit version injection for release image publishing.
- `Makefile`
  - Moves release artifacts into `dist/` and keeps checksums with the artifacts.
- `.dockerignore`
  - Excludes build outputs, local dependency directories, and local state files from Docker build contexts.

### Docs

- `docs/deployment.md`
  - Adds the deployment guide and operator runbook.
- `docs/releasing.md`
  - Adds release workflow and GHCR publishing instructions.
- `README.md`
  - Links the deployment guide from navigation and configuration docs.
- `CHANGELOG.md`
  - Adds this Phase 4 checkpoint entry.

## Verification

The following checks passed during this checkpoint:

```sh
docker compose --env-file deploy/.env.example -f deploy/docker-compose.yml config
ruby -e 'require "yaml"; YAML.load_file("deploy/docker-compose.yml"); YAML.load_file("deploy/stacyvm.production.yaml")'
git diff --check
go test ./...
cd web && npm run build
scripts/check-swagger.sh
make release-build-all VERSION=phase-4-test
```

GitHub Actions has also passed for the initial Phase 4 CI workflow after the Swagger drift check stabilization.

## Platform Notes

- Docker Compose validation does not require daemon access, but runtime sandbox conformance still requires Docker daemon access on the host.
- Firecracker remains Linux/KVM-gated and should be rolled out only after host conformance checks pass.
- PRoot remains gated on a real `proot` binary and a rootfs with the expected sandbox tooling.

## Next Phase 4 Direction

Remaining Phase 4 work should focus on release automation, container publishing, deployment smoke tests, and a clearer production conformance matrix for Docker, gVisor/Kata, Firecracker, and PRoot hosts.
