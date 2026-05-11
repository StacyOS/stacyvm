#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Running upgrade, config, and SQLite migration tests"
go test ./internal/config ./internal/store ./cmd/stacyvm \
  -run 'TestLoadAcceptsPhaseThreeConfig|TestSQLiteStoreMigratesLegacyDatabase|TestRunUpgradeRehearsal|TestLintConfigProductionBaselinePasses'

echo "==> Linting production deployment config with CI secrets"
STACYVM_AUTH_API_KEY="regular-api-key-with-at-least-32-bytes" \
STACYVM_AUTH_ADMIN_API_KEY="admin-api-key-with-at-least-32-bytesxx" \
go run ./cmd/stacyvm config lint --production --file deploy/stacyvm.production.yaml

echo "==> Upgrade and migration CI checks passed"
