#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DIST_DIR="$(mktemp -d)"
trap 'rm -rf "$DIST_DIR"' EXIT

echo "==> Checking public install and verification scripts"
bash -n scripts/install.sh
bash -n scripts/verify-release.sh
bash -n scripts/post-release-validate.sh

echo "==> Building release artifacts"
make release-build-all VERSION=phase-9-ci DIST_DIR="$DIST_DIR"

echo "==> Verifying release checksums"
(
  cd "$DIST_DIR"
  sha256sum -c checksums.txt
)

echo "==> Public release sanity checks passed"
