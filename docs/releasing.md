# Releasing StacyVM

StacyVM releases publish two deliverables:

- Static Linux binaries for `stacyvm` and `stacyvm-agent` under the GitHub release.
- A multi-arch container image at `ghcr.io/stacyos/stacyvm`.

Release binaries, `checksums.txt`, and the published container image digest are
signed with Sigstore keyless signing from the GitHub Actions release workflow.

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

The release workflow also attaches:

- `<artifact>.sig`
- `<artifact>.pem`

for every binary and `checksums.txt`.

## Verifying A Release

Install `cosign`, then run:

```bash
scripts/verify-release.sh v0.4.0 amd64
scripts/verify-release.sh v0.4.0 arm64
```

The verifier checks:

- Sigstore certificate identity for the StacyVM release workflow.
- Sigstore certificate issuer from GitHub Actions OIDC.
- Binary and agent SHA-256 entries in `checksums.txt`.

Manual verification for one artifact:

```bash
cosign verify-blob stacyvm-linux-amd64 \
  --signature stacyvm-linux-amd64.sig \
  --certificate stacyvm-linux-amd64.pem \
  --certificate-identity-regexp 'https://github.com/StacyOS/stacyvm/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

## Container Image

The release workflow publishes:

- `ghcr.io/stacyos/stacyvm:<version>`
- `ghcr.io/stacyos/stacyvm:latest` for `v*` tag releases

The image digest is signed after publishing:

```bash
cosign verify ghcr.io/stacyos/stacyvm@sha256:<digest> \
  --certificate-identity-regexp 'https://github.com/StacyOS/stacyvm/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

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

After the release workflow publishes artifacts, verify both architectures:

```bash
scripts/verify-release.sh v0.4.0 amd64
scripts/verify-release.sh v0.4.0 arm64
```

Or run the full post-release gate:

```bash
scripts/post-release-validate.sh v0.4.0
STACYVM_VALIDATE_INSTALLER=true scripts/post-release-validate.sh v0.4.0
```

The full gate confirms that every binary, checksum, signature, and certificate
asset exists on the GitHub release, runs signature and checksum verification for
both architectures, and can exercise `scripts/install.sh` in verify-only mode on
Linux.

For a GitHub-hosted Linux evidence bundle, run the manual **Public Readiness
Certification** workflow against the published tag. It validates the release,
runs installer verify-only, certifies the selected runtime on the runner, and
uploads the generated Markdown reports. Treat this as CI-host evidence only;
production-host runtime claims still require `scripts/certify-runtime.sh` on
the actual host.

## Notes

- Do not store release secrets in `stacyvm.production.yaml`; pass them through environment variables.
- Keep release notes in `docs/releases/` up to date before creating a GitHub release.
- Do not publish public self-serve releases without Sigstore signatures and checksums.
- Platform conformance for Docker, gVisor/Kata, Firecracker, and PRoot remains host-gated and should be reported separately from generic build health.
