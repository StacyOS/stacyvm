#!/usr/bin/env bash
set -euo pipefail

version="${STACYVM_VERSION:-}"
arch="${STACYVM_ARCH:-}"
repo="${STACYVM_REPO:-StacyOs/stacyvm}"
workdir="${STACYVM_VERIFY_DIR:-}"
identity_regexp="${STACYVM_COSIGN_IDENTITY_REGEXP:-https://github.com/StacyOS/stacyvm/.github/workflows/release.yml@refs/tags/v.*}"
issuer="${STACYVM_COSIGN_ISSUER:-https://token.actions.githubusercontent.com}"

usage() {
  cat <<'USAGE'
usage: scripts/verify-release.sh <version> [amd64|arm64]

Downloads StacyVM release checksums, binaries, Sigstore signatures, and
certificates, then verifies artifact authenticity and SHA-256 integrity.

Environment:
  STACYVM_REPO                         GitHub repo, default StacyOs/stacyvm
  STACYVM_VERIFY_DIR                   Reuse/download into this directory
  STACYVM_COSIGN_IDENTITY_REGEXP       Expected keyless signer identity regexp
  STACYVM_COSIGN_ISSUER                Expected certificate issuer
USAGE
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ -n "${1:-}" ]; then
  version="$1"
fi
if [ -n "${2:-}" ]; then
  arch="$2"
fi
if [ -z "$version" ]; then
  printf 'version is required\n' >&2
  usage >&2
  exit 2
fi
if [ -z "$arch" ]; then
  case "$(uname -m)" in
    x86_64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) printf 'unsupported architecture: %s\n' "$(uname -m)" >&2; exit 2 ;;
  esac
fi
case "$arch" in
  amd64|arm64) ;;
  *) printf 'unsupported arch: %s\n' "$arch" >&2; exit 2 ;;
esac

if ! command -v cosign >/dev/null 2>&1; then
  printf 'cosign is required for release verification: https://docs.sigstore.dev/cosign/installation/\n' >&2
  exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
  printf 'sha256sum is required for checksum verification\n' >&2
  exit 1
fi

if [ -z "$workdir" ]; then
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT
else
  mkdir -p "$workdir"
fi

release_url="https://github.com/${repo}/releases/download/${version}"
artifacts=(
  "checksums.txt"
  "stacyvm-linux-${arch}"
  "stacyvm-agent-linux-${arch}"
)

download() {
  local name="$1"
  curl -fsSL -o "${workdir}/${name}" "${release_url}/${name}"
}

for artifact in "${artifacts[@]}"; do
  download "$artifact"
  download "${artifact}.sig"
  download "${artifact}.pem"
done

for artifact in "${artifacts[@]}"; do
  cosign verify-blob "${workdir}/${artifact}" \
    --signature "${workdir}/${artifact}.sig" \
    --certificate "${workdir}/${artifact}.pem" \
    --certificate-identity-regexp "$identity_regexp" \
    --certificate-oidc-issuer "$issuer" >/dev/null
  printf '[PASS] signature: %s\n' "$artifact"
done

(
  cd "$workdir"
  sha256sum -c checksums.txt --ignore-missing
)

printf '[PASS] checksums verified for %s %s\n' "$version" "$arch"
printf 'verified artifacts in %s\n' "$workdir"
