#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${STACYVM_SMOKE_PORT:-17423}"
API_KEY="${STACYVM_API_KEY:-ci-smoke-key}"
DB_PATH="${STACYVM_DATABASE_PATH:-${TMPDIR:-/tmp}/stacyvm-ci-smoke.db}"
LOG_PATH="${STACYVM_SMOKE_LOG:-${TMPDIR:-/tmp}/stacyvm-ci-smoke.log}"

cd "$ROOT"

rm -f "$DB_PATH" "$DB_PATH-shm" "$DB_PATH-wal" "$LOG_PATH"

cleanup() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

STACYVM_SERVER_PORT="$PORT" \
STACYVM_PROVIDERS_DEFAULT=mock \
STACYVM_PROVIDERS_MOCK_ENABLED=true \
STACYVM_PROVIDERS_DOCKER_ENABLED=false \
STACYVM_PROVIDERS_FIRECRACKER_ENABLED=false \
STACYVM_AUTH_API_KEY="$API_KEY" \
STACYVM_DATABASE_PATH="$DB_PATH" \
  ./stacyvm serve >"$LOG_PATH" 2>&1 &
server_pid="$!"

for _ in $(seq 1 50); do
  if curl --silent --fail --max-time 1 -H "X-API-Key: $API_KEY" "http://127.0.0.1:$PORT/api/v1/live" >/dev/null; then
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "StacyVM server exited before becoming live. Logs:" >&2
    cat "$LOG_PATH" >&2
    exit 1
  fi
  sleep 0.2
done

scripts/smoke-deployment.sh "http://127.0.0.1:$PORT" "$API_KEY"
