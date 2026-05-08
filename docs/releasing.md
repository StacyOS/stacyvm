# Releasing StacyVM

StacyVM releases publish two deliverables:

- Static Linux binaries for `stacyvm` and `stacyvm-agent` under the GitHub release.
- A multi-arch container image at `ghcr.io/stacyos/stacyvm`.

## Release Workflow

The release workflow lives at `.github/workflows/release.yml`.

It runs automatically for tags that match `v*`:

```bash
git tag v0.4.0
git push origin v0.4.0
```

It can also be started manually from GitHub Actions with:

- `version`: release version or image tag, for example `v0.4.0`.
- `publish_image`: whether to publish the GHCR image.
- `create_release`: whether to create a GitHub release with binary artifacts.

Tag-triggered releases always build binaries, create the GitHub release, and publish the container image.

## Binary Artifacts

Local release artifacts can be built with:

```bash
make release-build-all VERSION=v0.4.0
```

The command writes artifacts to `dist/`:

- `stacyvm-linux-amd64`
- `stacyvm-agent-linux-amd64`
- `stacyvm-linux-arm64`
- `stacyvm-agent-linux-arm64`
- `checksums.txt`

## Container Image

The release workflow publishes:

- `ghcr.io/stacyos/stacyvm:<version>`
- `ghcr.io/stacyos/stacyvm:latest` for `v*` tag releases

The Dockerfile accepts a `VERSION` build argument and uses BuildKit target platform args so the release workflow can publish `linux/amd64` and `linux/arm64` images from one workflow.

To test the image locally before publishing:

```bash
docker build --build-arg VERSION=dev -t stacyvm:dev .
docker run --rm stacyvm:dev version
```

## Preflight Checklist

Before tagging:

```bash
make test
make build
cd web && npm run build
scripts/check-swagger.sh
stacyvm config lint --production --file deploy/stacyvm.production.yaml
make release-build-all VERSION=v0.4.0
```

When linting the production template, provide real `STACYVM_AUTH_API_KEY` and `STACYVM_AUTH_ADMIN_API_KEY` values through the environment so placeholder secrets do not pass the release gate.

For Phase 4, also confirm the production deployment templates still render:

```bash
docker compose --env-file deploy/.env.example -f deploy/docker-compose.yml config
```

## Notes

- Do not store release secrets in `stacyvm.production.yaml`; pass them through environment variables.
- Keep release notes in `docs/releases/` up to date before creating a GitHub release.
- Platform conformance for Docker, gVisor/Kata, Firecracker, and PRoot remains host-gated and should be reported separately from generic build health.
