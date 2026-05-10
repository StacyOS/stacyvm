#!/usr/bin/env bash
# smoke-remote-worker.sh — end-to-end remote-worker smoke test.
#
# Plain mode (default): uses shared worker token over HTTP.
# mTLS mode (--mtls):   generates an ephemeral CA + server/client certs via
#                       openssl and runs the same smoke over HTTPS with mutual
#                       TLS authentication between control plane and worker RPC.
#
# Usage:
#   scripts/smoke-remote-worker.sh [binary] [--mtls] [--ca-cert f] [--ca-key f]
#                                  [--server-cert f] [--server-key f]
#                                  [--client-cert f] [--client-key f]
#
# Environment:
#   STACYVM_REMOTE_SMOKE_DATABASE_DRIVER   sqlite (default) or postgres
#   STACYVM_REMOTE_SMOKE_DATABASE_DSN      required when driver=postgres
#   STACYVM_REMOTE_SMOKE_CONTROL_PORT      default 17423
#   STACYVM_REMOTE_SMOKE_WORKER_PORT       default 17430
set -euo pipefail

# ── argument parsing ─────────────────────────────────────────────────────────
BIN="${1:-./stacyvm}"
shift || true

MTLS=false
CA_CERT=""   CA_KEY=""
SRV_CERT=""  SRV_KEY=""
CLI_CERT=""  CLI_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mtls)            MTLS=true;         shift ;;
    --ca-cert)         CA_CERT="$2";      shift 2 ;;
    --ca-key)          CA_KEY="$2";       shift 2 ;;
    --server-cert)     SRV_CERT="$2";     shift 2 ;;
    --server-key)      SRV_KEY="$2";      shift 2 ;;
    --client-cert)     CLI_CERT="$2";     shift 2 ;;
    --client-key)      CLI_KEY="$2";      shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$BIN" != /* ]]; then BIN="$ROOT_DIR/${BIN#./}"; fi

WORK_DIR="$(mktemp -d)"
CONTROL_DIR="$WORK_DIR/control-plane"
WORKER_DIR="$WORK_DIR/worker"
CERT_DIR="$WORK_DIR/certs"
CONTROL_CONFIG="$CONTROL_DIR/stacyvm.yaml"
WORKER_CONFIG="$WORKER_DIR/stacyvm.yaml"
SERVER_LOG="$WORK_DIR/server.log"
WORKER_LOG="$WORK_DIR/worker.log"
DB_PATH="$WORK_DIR/stacyvm.db"

DATABASE_DRIVER="${STACYVM_REMOTE_SMOKE_DATABASE_DRIVER:-sqlite}"
DATABASE_DSN="${STACYVM_REMOTE_SMOKE_DATABASE_DSN:-}"
# Default ports differ between plain and mTLS so sequential runs don't race for the same port.
if $MTLS; then
  CONTROL_PORT="${STACYVM_REMOTE_SMOKE_CONTROL_PORT:-17433}"
  WORKER_PORT="${STACYVM_REMOTE_SMOKE_WORKER_PORT:-17440}"
else
  CONTROL_PORT="${STACYVM_REMOTE_SMOKE_CONTROL_PORT:-17423}"
  WORKER_PORT="${STACYVM_REMOTE_SMOKE_WORKER_PORT:-17430}"
fi
API_KEY="dev-api-key-dev-api-key-dev-api-key"
ADMIN_KEY="dev-admin-key-dev-admin-key-dev"
WORKER_TOKEN="dev-worker-token-dev-worker-token"

cleanup() {
  kill "${WORKER_PID:-}" 2>/dev/null || true
  kill "${SERVER_PID:-}" 2>/dev/null || true
  wait "${WORKER_PID:-}" 2>/dev/null || true
  wait "${SERVER_PID:-}" 2>/dev/null || true
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$CONTROL_DIR" "$WORKER_DIR" "$CERT_DIR"

# ── mTLS cert generation ─────────────────────────────────────────────────────
if $MTLS; then
  if [[ -z "$CA_CERT" ]]; then
    # No external certs provided — generate an ephemeral PKI.
    if ! command -v openssl >/dev/null 2>&1; then
      echo "openssl is required for --mtls ephemeral cert generation" >&2
      exit 1
    fi

    echo "==> Generating ephemeral CA and mTLS certificates"

    # CA
    openssl genrsa -out "$CERT_DIR/ca.key" 2048 2>/dev/null
    openssl req -new -x509 -days 1 \
      -key "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" \
      -subj "/CN=stacyvm-smoke-ca" 2>/dev/null

    # Worker RPC server cert (SAN must include 127.0.0.1 for TLS hostname check).
    openssl genrsa -out "$CERT_DIR/server.key" 2048 2>/dev/null
    openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" \
      -subj "/CN=stacyvm-worker-rpc" 2>/dev/null
    openssl x509 -req -days 1 \
      -in "$CERT_DIR/server.csr" \
      -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
      -out "$CERT_DIR/server.crt" \
      -extfile <(printf 'subjectAltName=IP:127.0.0.1') 2>/dev/null

    # Control-plane client cert (authenticates to worker's mTLS server).
    openssl genrsa -out "$CERT_DIR/client.key" 2048 2>/dev/null
    openssl req -new -key "$CERT_DIR/client.key" -out "$CERT_DIR/client.csr" \
      -subj "/CN=stacyvm-control-plane" 2>/dev/null
    openssl x509 -req -days 1 \
      -in "$CERT_DIR/client.csr" \
      -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
      -out "$CERT_DIR/client.crt" 2>/dev/null

    CA_CERT="$CERT_DIR/ca.crt"
    SRV_CERT="$CERT_DIR/server.crt"
    SRV_KEY="$CERT_DIR/server.key"
    CLI_CERT="$CERT_DIR/client.crt"
    CLI_KEY="$CERT_DIR/client.key"
  else
    # Validate all cert files were supplied.
    for f in "$CA_CERT" "$SRV_CERT" "$SRV_KEY" "$CLI_CERT" "$CLI_KEY"; do
      if [[ ! -f "$f" ]]; then
        echo "cert file not found: $f" >&2
        exit 1
      fi
    done
  fi
fi

# ── control-plane config ─────────────────────────────────────────────────────
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
logging:
  level: "warn"
  format: "json"
YAML

if [[ "$DATABASE_DRIVER" == "postgres" || "$DATABASE_DRIVER" == "postgresql" ]]; then
  [[ -z "$DATABASE_DSN" ]] && { echo "STACYVM_REMOTE_SMOKE_DATABASE_DSN is required for postgres" >&2; exit 1; }
  cat >>"$CONTROL_CONFIG" <<YAML
database:
  driver: "postgres"
  dsn: "$DATABASE_DSN"
YAML
else
  cat >>"$CONTROL_CONFIG" <<YAML
database:
  driver: "sqlite"
  path: "$DB_PATH"
YAML
fi

# mTLS: add the control-plane-side RPC TLS client config (used when it calls worker RPC).
if $MTLS; then
  cat >>"$CONTROL_CONFIG" <<YAML
worker:
  rpc_tls:
    enabled: true
    ca_file: "$CA_CERT"
    client_cert_file: "$CLI_CERT"
    client_key_file: "$CLI_KEY"
YAML
fi

# ── worker config ─────────────────────────────────────────────────────────────
WORKER_LISTEN_SCHEME="http"
if $MTLS; then WORKER_LISTEN_SCHEME="https"; fi

# Build worker config — include rpc_tls inline so it is a proper child of worker:
{
  cat <<YAML
worker:
  id: "worker-a"
  control_plane_url: "http://127.0.0.1:$CONTROL_PORT"
  listen_addr: "127.0.0.1:$WORKER_PORT"
  heartbeat_interval: "1s"
YAML
  if $MTLS; then
    cat <<YAML
  rpc_tls:
    enabled: true
    server_cert_file: "$SRV_CERT"
    server_key_file: "$SRV_KEY"
    client_ca_file: "$CA_CERT"
YAML
  fi
  cat <<YAML
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
} >"$WORKER_CONFIG"

# ── smoke ─────────────────────────────────────────────────────────────────────
MODE="plain-HTTP"
if $MTLS; then MODE="mTLS"; fi
# Wait for ports to be free (guards against in-use ports from sequential runs).
# Uses curl --max-time so we don't hang if the port check itself blocks.
wait_port_free() {
  local port="$1"
  for _ in {1..25}; do
    if ! curl -fsS --max-time 0.3 "http://127.0.0.1:$port/" >/dev/null 2>&1; then return 0; fi
    sleep 0.3
  done
  # Port appears busy but proceed anyway; the server bind will fail loudly if so.
  return 0
}
wait_port_free "$CONTROL_PORT"
wait_port_free "$WORKER_PORT"

echo "==> Starting control plane [${MODE}]"
(cd "$CONTROL_DIR"; "$BIN" serve) >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

for _ in {1..50}; do
  if curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/ready" >/dev/null 2>&1; then break; fi
  sleep 0.2
done
if ! curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/ready" >/dev/null; then
  echo "control plane did not become ready; log:" >&2; cat "$SERVER_LOG" >&2; exit 1
fi

echo "==> Starting remote worker [${MODE}]"
(cd "$WORKER_DIR"; "$BIN" worker) >"$WORKER_LOG" 2>&1 &
WORKER_PID=$!

for _ in {1..50}; do
  if curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/workers" \
      | grep -q '"id":"worker-a"'; then break; fi
  sleep 0.2
done
if ! curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$CONTROL_PORT/api/v1/workers" \
    | grep -q '"id":"worker-a"'; then
  echo "worker did not register; log:" >&2; cat "$WORKER_LOG" >&2; exit 1
fi

if $MTLS; then
  echo "==> Verifying worker advertised rpc_url uses HTTPS"
  RPC_URL="$(curl -fsS -H "X-API-Key: $API_KEY" \
    "http://127.0.0.1:$CONTROL_PORT/api/v1/workers/worker-a" \
    | sed -n 's/.*"rpc_url":"\([^"]*\)".*/\1/p')"
  if [[ "$RPC_URL" != https://* ]]; then
    echo "worker rpc_url is not HTTPS: '${RPC_URL}'" >&2
    cat "$WORKER_LOG" >&2
    exit 1
  fi
  echo "    rpc_url=$RPC_URL [OK]"
fi

echo "==> Preferring remote worker for this smoke"
curl -fsS -X DELETE -H "X-Admin-API-Key: $ADMIN_KEY" \
  "http://127.0.0.1:$CONTROL_PORT/api/v1/admin/workers/local" >/dev/null || true

echo "==> Spawning remote mock sandbox"
SPAWN_RESPONSE="$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"image":"alpine:latest","provider":"mock","ttl":"5m"}' \
  "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes")"
SANDBOX_ID="$(printf '%s' "$SPAWN_RESPONSE" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -z "$SANDBOX_ID" ]]; then
  echo "spawn response had no sandbox id: $SPAWN_RESPONSE" >&2; exit 1
fi
if ! printf '%s' "$SPAWN_RESPONSE" | grep -q '"worker_id":"worker-a"'; then
  echo "spawn did not route to worker-a: $SPAWN_RESPONSE" >&2; exit 1
fi
echo "    sandbox=$SANDBOX_ID worker=worker-a [OK]"

echo "==> Verifying sandbox running"
STATUS="$(curl -fsS -H "X-API-Key: $API_KEY" \
  "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes/$SANDBOX_ID")"
if ! printf '%s' "$STATUS" | grep -q '"state":"running"'; then
  echo "sandbox not running: $STATUS" >&2; exit 1
fi

if $MTLS; then
  echo "==> Executing command over remote worker mTLS RPC"
  EXEC_RESULT="$(curl -fsS -X POST \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d '{"command":"echo stacyvm-mtls-ok","mode":"shell"}' \
    "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes/$SANDBOX_ID/exec")"
  if ! printf '%s' "$EXEC_RESULT" | grep -q '"exit_code":0'; then
    echo "exec over mTLS RPC failed: $EXEC_RESULT" >&2; exit 1
  fi
  echo "    exec exit_code=0 [OK]"
fi

echo "==> Destroying remote sandbox"
curl -fsS -X DELETE -H "X-API-Key: $API_KEY" \
  "http://127.0.0.1:$CONTROL_PORT/api/v1/sandboxes/$SANDBOX_ID" >/dev/null

echo ""
echo "==> Remote worker smoke PASSED [${MODE}]"
if $MTLS; then
  echo "    mTLS certs used:"
  echo "      CA:     ${CA_CERT}"
  echo "      server: ${SRV_CERT}"
  echo "      client: ${CLI_CERT}"
fi
