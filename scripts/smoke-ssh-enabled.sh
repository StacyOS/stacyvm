#!/usr/bin/env bash
# Proves `stacyvm serve` RUNS (does not crash) with ssh.enabled=true as a
# non-root user with no /var/lib/stacyvm. Uses the mock provider and isolated
# temp HOME/db/ports so it never touches a real install.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$(mktemp -d)/stacyvm"
go build -o "$BIN" "$ROOT/cmd/stacyvm"

TMP="$(mktemp -d)"
SSH_PORT=22422
LOG="$TMP/serve.log"

HOME="$TMP" \
STACYVM_SSH_ENABLED=true \
STACYVM_SSH_LISTEN_ADDR=":$SSH_PORT" \
STACYVM_PROVIDERS_DEFAULT=mock \
STACYVM_PROVIDERS_MOCK_ENABLED=true \
STACYVM_PROVIDERS_FIRECRACKER_ENABLED=false \
STACYVM_PROVIDERS_DOCKER_ENABLED=false \
STACYVM_DATABASE_PATH="$TMP/test.db" \
STACYVM_SERVER_PORT=27423 \
  "$BIN" serve >"$LOG" 2>&1 &
PID=$!

cleanup() { kill "$PID" 2>/dev/null || true; }
trap cleanup EXIT

# Give it a moment to boot and create keys; poll the SSH port.
for _ in $(seq 1 20); do
  kill -0 "$PID" 2>/dev/null || { echo "FAIL: serve exited at startup"; cat "$LOG"; exit 1; }
  if (exec 3<>/dev/tcp/127.0.0.1/"$SSH_PORT") 2>/dev/null; then exec 3>&- 3<&-; break; fi
  sleep 0.5
done

if ! kill -0 "$PID" 2>/dev/null; then
  echo "FAIL: serve is not running"; cat "$LOG"; exit 1
fi
if ! (exec 3<>/dev/tcp/127.0.0.1/"$SSH_PORT") 2>/dev/null; then
  echo "FAIL: SSH gateway not listening on :$SSH_PORT"; cat "$LOG"; exit 1
fi
exec 3>&- 3<&- 2>/dev/null || true
test -f "$TMP/.stacyvm/ssh_host_ed25519_key" || { echo "FAIL: host key not created under \$HOME/.stacyvm"; exit 1; }
test -f "$TMP/.stacyvm/ssh_user_ca_key" || { echo "FAIL: user CA not created under \$HOME/.stacyvm"; exit 1; }

echo "PASS: serve runs with ssh.enabled=true; gateway listening on :$SSH_PORT; keys created under \$HOME/.stacyvm"
