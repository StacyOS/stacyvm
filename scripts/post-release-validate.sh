#!/usr/bin/env bash
set -euo pipefail

version="${1:-${STACYVM_VERSION:-}}"
repo="${STACYVM_REPO:-StacyOs/stacyvm}"

usage() {
  cat <<'USAGE'
usage: scripts/post-release-validate.sh <version>

Validates a published StacyVM GitHub release after a real version tag exists.
The release must include binaries, checksums, Sigstore signatures, and
certificates for amd64 and arm64.

Environment:
  STACYVM_REPO                 GitHub repo, default StacyOs/stacyvm
  STACYVM_VALIDATE_INSTALLER   Run install.sh in verify-only mode when on Linux
USAGE
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ -z "$version" ]; then
  printf 'version is required\n' >&2
  usage >&2
  exit 2
fi

if ! command -v gh >/dev/null 2>&1; then
  printf 'gh is required to inspect GitHub release assets\n' >&2
  exit 1
fi

required_assets=(
  checksums.txt
  checksums.txt.sig
  checksums.txt.pem
  stacyvm-linux-amd64
  stacyvm-linux-amd64.sig
  stacyvm-linux-amd64.pem
  stacyvm-agent-linux-amd64
  stacyvm-agent-linux-amd64.sig
  stacyvm-agent-linux-amd64.pem
  stacyvm-linux-arm64
  stacyvm-linux-arm64.sig
  stacyvm-linux-arm64.pem
  stacyvm-agent-linux-arm64
  stacyvm-agent-linux-arm64.sig
  stacyvm-agent-linux-arm64.pem
)

assets="$(gh release view "$version" --repo "$repo" --json assets --jq '.assets[].name')"
missing=0
for asset in "${required_assets[@]}"; do
  if ! grep -Fxq "$asset" <<<"$assets"; then
    printf '[FAIL] missing release asset: %s\n' "$asset" >&2
    missing=$((missing + 1))
  else
    printf '[PASS] release asset: %s\n' "$asset"
  fi
done
if [ "$missing" -gt 0 ]; then
  printf 'release %s is missing %d required asset(s)\n' "$version" "$missing" >&2
  exit 1
fi

scripts/verify-release.sh "$version" amd64
scripts/verify-release.sh "$version" arm64

if [ "${STACYVM_VALIDATE_INSTALLER:-false}" = "true" ]; then
  if [ "$(uname -s)" != "Linux" ]; then
    printf '[WARN] installer verify-only check skipped: install.sh is Linux-only\n' >&2
  else
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT
    STACYVM_VERSION="$version" \
      STACYVM_INSTALL_DIR="$tmpdir" \
      STACYVM_REQUIRE_SIGNATURES=true \
      STACYVM_VERIFY_ONLY=true \
      scripts/install.sh
  fi
fi

printf '[PASS] post-release validation complete for %s\n' "$version"
