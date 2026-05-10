---
title: "Production Deployment"
description: "Deploy StacyVM as a single-node service with Docker, systemd, health checks, metrics, and release validation."
---

# Production Deployment

This guide covers a single-node StacyVM deployment suitable for an internal service, staging, or a small production installation. The default production path uses the Docker provider because it works on the broadest set of hosts; Firecracker and PRoot require extra host setup and should be validated on the target platform before rollout.

## Requirements

- Linux host with Docker installed when using the Docker provider.
- A persistent data directory, normally `/var/lib/stacyvm`.
- A generated API key with at least 32 bytes of entropy.
- TLS and public ingress handled by a reverse proxy or load balancer in front of StacyVM.
- Explicit `server.cors_allowed_origins` for every browser origin that may call the API.
- Health checks wired to the API endpoints listed below.

StacyVM reads config from `./stacyvm.yaml`, then `~/.stacyvm/config.yaml`, then `STACYVM_` environment variables. In production, prefer a checked-in baseline config plus environment variables or secret files for secrets and environment-specific values. Worker credentials can be mounted through `auth.worker_token_file` and `auth.worker_signing_key_file`; the loader rejects configs that set both the inline secret and its file reference.

Before starting a single-node staging or production host, lint the final config with the same environment variables the service will use:

```bash
STACYVM_AUTH_API_KEY=sk-live \
STACYVM_AUTH_ADMIN_API_KEY=sk-admin \
stacyvm config lint --production --file deploy/stacyvm.production.yaml
```

The lint command is deterministic and does not require Docker or KVM access. In production mode it fails wildcard CORS, missing auth, weak rate limits, relative database paths, missing sandbox caps, unsafe Docker settings, and other public-exposure risks. Use `stacyvm doctor --production` after linting when you also want live host checks for Docker, Firecracker, PRoot, database directories, and installed binaries.

## Health and Metrics

Use these endpoints for load balancers and monitors:

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/live` | Process liveness. Use this for simple restart checks. |
| `GET /api/v1/ready` | Readiness. Use this before routing traffic after deploys. |
| `GET /api/v1/health` | Dependency and provider health summary. |
| `GET /api/v1/metrics/prometheus` | Prometheus metrics scrape endpoint. |

Authenticated deployments should send `X-API-Key: <api-key>` to protected API endpoints. Keep health probes scoped to your private network if they bypass auth at an upstream proxy.

After a deploy, run the smoke script:

```bash
STACYVM_SMOKE_URL=https://stacyvm.example.com STACYVM_API_KEY=sk-live scripts/smoke-deployment.sh
```

## Docker Compose

The files in `deploy/` provide a production-oriented Compose starting point:

- `deploy/docker-compose.yml` starts StacyVM and Traefik for live previews.
- `deploy/stacyvm.production.yaml` enables auth, explicit CORS origins, rate limiting, sandbox caps, queueing, JSON logs, and persistent SQLite state.
- `deploy/.env.example` lists the environment variables expected by the Compose file.
- `deploy/stacyvm.env.example` is the systemd environment file template.

Use separate values for `STACYVM_API_KEY` and `STACYVM_ADMIN_API_KEY` in production. Admin routes live under `/api/v1/admin/*` and should be restricted to operator networks where possible. Replace the example `server.cors_allowed_origins` value with the exact public console/API origins for your deployment; do not expose browser clients with wildcard CORS.

See [admin-control-plane.md](admin-control-plane.md) for admin dashboard setup, quota operations, diagnostics, audit export, and audit retention notes. See [security-governance.md](security-governance.md) for the production admin hardening checklist and OIDC/SSO integration plan. The production config keeps 90 days of admin audit history with `auth.admin_audit_retention: "2160h"` and disables admin fallback with `auth.admin_fallback_enabled: false`.

```bash
cd deploy
cp .env.example .env
# Edit .env and replace STACYVM_API_KEY before starting.
docker compose up -d
docker compose logs -f stacyvm
```

For local image testing before a registry image exists:

```bash
docker build -t stacyvm:local ..
STACYVM_IMAGE=stacyvm:local docker compose up -d
```

For non-invasive smoke runs on a shared host, override the published ports:

```bash
STACYVM_IMAGE=stacyvm:local STACYVM_HOST_PORT=17426 STACYVM_TRAEFIK_HOST_PORT=18080 docker compose up -d
```

Then validate the API surface:

```bash
scripts/smoke-deployment.sh http://127.0.0.1:17426 "$STACYVM_API_KEY"
```

Live-preview routing can be checked by spawning a sandbox that serves port `3000` and requesting Traefik with `Host: 3000-<sandbox-id>.<preview-domain>`.

## systemd

Use `deploy/stacyvm.service` when running the binary directly on a Linux host.

```bash
sudo useradd --system --home /var/lib/stacyvm --shell /usr/sbin/nologin stacyvm
sudo usermod -aG docker stacyvm
sudo install -d -o stacyvm -g stacyvm /var/lib/stacyvm
sudo install -d -m 0750 /etc/stacyvm
sudo install -o root -g stacyvm -m 0640 deploy/stacyvm.production.yaml /etc/stacyvm/stacyvm.yaml
sudo install -o root -g stacyvm -m 0640 deploy/stacyvm.env.example /etc/stacyvm/stacyvm.env
sudo install -m 0755 bin/stacyvm /usr/local/bin/stacyvm
sudo install -m 0755 bin/stacyvm-agent /usr/local/bin/stacyvm-agent
sudo install -m 0644 deploy/stacyvm.service /etc/systemd/system/stacyvm.service
```

Edit `/etc/stacyvm/stacyvm.env` and set real `STACYVM_AUTH_API_KEY` and `STACYVM_AUTH_ADMIN_API_KEY` values. Then enable the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now stacyvm
sudo systemctl status stacyvm
```

The included unit uses `WorkingDirectory=/etc/stacyvm` so StacyVM can load `/etc/stacyvm/stacyvm.yaml` through its current `./stacyvm.yaml` lookup path while keeping persistent database state in `/var/lib/stacyvm`.

## Reverse Proxy

Terminate TLS before StacyVM. A typical proxy should:

- Forward API traffic to `http://127.0.0.1:7423`.
- Preserve `X-API-Key` headers.
- Route live preview hostnames such as `3000-sb-<id>.<preview-domain>` to Traefik when using Docker live previews.
- Restrict admin and metrics endpoints to trusted networks.

Set `server.preview_domain` or `STACYVM_SERVER_PREVIEW_DOMAIN` to the domain that resolves preview subdomains to your proxy.

## Backups

The default store is SQLite at `/var/lib/stacyvm/stacyvm.db`. Prefer the built-in backup command because it uses SQLite's online backup path and validates the output:

```bash
stacyvm db backup /backup/stacyvm-$(date +%Y%m%dT%H%M%SZ).db --database /var/lib/stacyvm/stacyvm.db
```

Restore requires the service to be stopped. The command validates the backup, creates a pre-restore safety copy of the current database, removes stale WAL/SHM sidecars, and replaces the target database:

```bash
sudo systemctl stop stacyvm
stacyvm db restore /backup/stacyvm-20260508T120000Z.db --database /var/lib/stacyvm/stacyvm.db --yes
sudo systemctl start stacyvm
```

For a manual fallback:

```bash
sudo systemctl stop stacyvm
sudo cp /var/lib/stacyvm/stacyvm.db /backup/stacyvm.db
sudo cp /var/lib/stacyvm/stacyvm.db-wal /backup/ 2>/dev/null || true
sudo cp /var/lib/stacyvm/stacyvm.db-shm /backup/ 2>/dev/null || true
sudo systemctl start stacyvm
```

If you run with Docker Compose, stop the service or snapshot the backing volume with your volume provider's backup tooling.

## Upgrades

Before changing binaries or images, rehearse the upgrade with the exact config and database path the service uses:

```bash
stacyvm upgrade rehearse \
  --config /etc/stacyvm/stacyvm.yaml \
  --database /var/lib/stacyvm/stacyvm.db \
  --backup-output /backup/stacyvm-pre-upgrade.db
```

Use `--include-doctor` on the target host when you also want live provider checks before the upgrade.

Upgrade flow:

1. Check the release notes for config or API changes.
2. Run `stacyvm upgrade rehearse` and resolve any failing checks.
3. Back up `/var/lib/stacyvm/stacyvm.db` with `stacyvm db backup`.
4. Replace the binary or update `STACYVM_IMAGE`.
5. Restart the service.
6. Confirm `GET /api/v1/ready` succeeds before routing traffic.
7. If the upgrade fails, stop StacyVM and restore the pre-upgrade backup with `stacyvm db restore --yes`.

## Support Bundles

For support requests, generate a redacted bundle instead of sharing raw config, logs, or environment output:

```bash
stacyvm support bundle /tmp/stacyvm-support.json \
  --config /etc/stacyvm/stacyvm.yaml \
  --include-doctor \
  --include-server
```

The bundle includes version/runtime data, redacted config shape, production config lint results, optional doctor checks, and optional `/api/v1/diagnostics` output. Secret-shaped keys, API keys, bearer tokens, and URLs with embedded credentials are redacted before the file is written.

## Provider Notes

Docker is the safest default for broad deployment compatibility. For stronger isolation, run Docker with gVisor (`runtime: "runsc"`) or Kata after validating the runtime on the host.

Firecracker requires Linux/KVM, a kernel image, rootfs images, networking setup, and the `stacyvm-agent` binary available to the runtime. Keep Firecracker disabled in shared templates until a host conformance check passes.

PRoot requires a real rootfs with the binaries your sandboxes need. Use it for restricted environments where Docker and KVM are unavailable, and validate memory/disk limits against the host because PRoot enforcement is not equivalent to VM isolation.

Use [runtime-conformance.md](runtime-conformance.md) as the signoff checklist for Docker, gVisor, Kata, Firecracker, PRoot, E2B, and custom providers.
