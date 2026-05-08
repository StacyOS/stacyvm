#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT="$(mktemp -d)"
TMP_DIR="$TMP_ROOT/docs"
cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

cd "$ROOT"

go run github.com/swaggo/swag/cmd/swag@v1.16.4 init \
  -g internal/api/server.go \
  -o "$TMP_DIR" \
  --parseDependency

for file in docs.go swagger.json swagger.yaml; do
  if ! diff -u "docs/$file" "$TMP_DIR/$file"; then
    echo
    echo "Swagger docs are stale. Regenerate with:"
    echo "  go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g internal/api/server.go -o docs --parseDependency"
    exit 1
  fi
done

echo "Swagger docs are up to date."
