#!/usr/bin/env bash
set -euo pipefail

BIN="${1:-./stacyvm}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$BIN" != /* ]]; then
  BIN="$ROOT_DIR/${BIN#./}"
fi
WORK_DIR="$(mktemp -d)"
CONTROL_DIR="$WORK_DIR/control-plane"
WORKER_DIR="$WORK_DIR/worker"
CONTROL_CONFIG="$CONTROL_DIR/stacyvm.yaml"
WORKER_CONFIG="$WORKER_DIR/stacyvm.yaml"
SERVER_LOG="$WORK_DIR/server.log"
WORKER_LOG="$WORK_DIR/worker.log"
DB_PATH="$WORK_DIR/stacyvm.db"
CONTROL_PORT="${STACYVM_REMOTE_SMOKE_CONTROL_PORT:-17423}"
WORKER_PORT="${STACYVM_REMOTE_SMOKE_WORKER_PORT:-17430}"
API_KEY="dev-api-key-dev-api-key-dev-api-key"
ADMIN_KEY="dev-admin-key-dev-admin-key-dev"
WORKER_TOKEN="dev-worker-token-dev-worker-token"

cleanup() {
  if [[ -n "${WORKER_PID:-}" ]]; then
    kill "$WORKER_PID" 2>/dev/null || true
  fi
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  wait "${WORKER_PID:-}" 2>/dev/null || true
  wait "${SERVER_PID:-}" 2>/dev/null || true
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$CONTROL_DIR" "$WORKER_DIR"

cat >"$CONTROL_CONFIG" <<YAML
server:
  host: "127.0.0.1"
  port: $CONTROL_PORT
providers:
  default: "mock"
  mock:
    enabled: true
  docker:
    enabled: false
  firecracker:
    enabled: false
auth:
  api_key: "$API_KEY"
  admin_api_key: "$ADMIN_KEY"
  worker_token: "$WORKER_TOKEN"
  admin_fallback_enabled: false
database:
  path: "$DB_PATH"
logging:
  level: "warn"
  format: "json"
YAML

cat >"$WORKER_CONFIG" <<YAML
worker:
  id: "worker-a"
  control_plane_url: "http://127.0.0.1:$CONTROL_PORT"
  listen_addr: "127.0.0.1:$WORKER_PORT"
  heartbeat_interval: "1s"
providers:
  default: "mock"
  mock:
    enabled: true
  docker:
    enabled: false
  firecracker:
    enabled: false
auth:
  worker_token: "$WORKER_TOKEN"
logging:
  level: "warn"
  format: "json"
YAML

echo "==> Starting control plane"
(
  cd "$CONTROL_DIR"
  "$BIN" serve
) >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

for _ in {1..50}; do
  if curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/ready" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
if ! curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/ready" >/dev/null; then
  echo "control plane did not become ready; server log:" >&2
  cat "$SERVER_LOG" >&2
  exit 1
fi

echo "==> Starting remote worker"
(
  cd "$WORKER_DIR"
  "$BIN" worker
) >"$WORKER_LOG" 2>&1 &
WORKER_PID=$!

for _ in {1..50}; do
  if curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/workers" | grep -q '"id":"worker-a"'; then
    break
  fi
  sleep 0.2
done
if ! curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/workers" | grep -q '"id":"worker-a"'; then
  echo "worker did not register; worker log:" >&2
  cat "$WORKER_LOG" >&2
  exit 1
fi

echo "==> Preferring remote worker for this smoke"
curl -fsS -X DELETE -H "X-Admin-API-Key: $ADMIN_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/admin/workers/local" >/dev/null || true

echo "==> Spawning remote mock sandbox"
SPAWN_RESPONSE="$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"image":"alpine:latest","provider":"mock","ttl":"5m"}' \
  "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes")"
SANDBOX_ID="$(printf '%s' "$SPAWN_RESPONSE" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -z "$SANDBOX_ID" ]]; then
  echo "spawn response did not contain sandbox id: $SPAWN_RESPONSE" >&2
  exit 1
fi
if ! printf '%s' "$SPAWN_RESPONSE" | grep -q '"worker_id":"worker-a"'; then
  echo "spawn did not route to worker-a: $SPAWN_RESPONSE" >&2
  exit 1
fi

echo "==> Refreshing remote status"
curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes/$SANDBOX_ID" | grep -q '"state":"running"'

echo "==> Destroying remote sandbox"
curl -fsS -X DELETE -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes/$SANDBOX_ID" >/dev/null

echo "==> Remote worker smoke passed"
