#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

export GOCACHE="${GOCACHE:-$tmpdir/go-build}"

worker_id="${1:-worker-a}"
audience="${STACYVM_WORKER_IDENTITY_AUDIENCE:-worker:control-plane}"
ttl="${STACYVM_WORKER_IDENTITY_TTL:-5m}"
token_id="${STACYVM_WORKER_IDENTITY_TOKEN_ID:-certification-token-id}"
signing_key_file="${STACYVM_WORKER_SIGNING_KEY_FILE:-$tmpdir/worker-signing-key}"
old_signing_key_file="${STACYVM_OLD_WORKER_SIGNING_KEY_FILE:-$tmpdir/worker-signing-key-old}"

if [[ ! -f "$signing_key_file" ]]; then
  printf '%s\n' "worker-signing-key-with-at-least-32-bytes" >"$signing_key_file"
fi
if [[ ! -f "$old_signing_key_file" ]]; then
  printf '%s\n' "old-worker-signing-key-with-at-least-32-bytes" >"$old_signing_key_file"
fi

echo "==> Issuing signed worker token"
token="$(
  go run ./cmd/stacyvm worker token "$worker_id" \
    --signing-key-file "$signing_key_file" \
    --ttl "$ttl" \
    --audience "$audience" \
    --token-id "$token_id"
)"

echo "==> Inspecting signed worker token metadata"
inspect_output="$(
  go run ./cmd/stacyvm worker token inspect "$token"
)"
if [[ "$inspect_output" != *"\"signature_verified\": false"* || "$inspect_output" != *"\"token_id\": \"$token_id\""* ]]; then
  echo "$inspect_output"
  echo "expected unverified inspect output with token_id $token_id" >&2
  exit 1
fi

echo "==> Verifying signed worker token"
verify_output="$(
  go run ./cmd/stacyvm worker token verify "$token" \
    --signing-key-file "$signing_key_file" \
    --worker-id "$worker_id" \
    --audience "$audience"
)"
if [[ "$verify_output" != *"\"signature_verified\": true"* || "$verify_output" != *"\"worker_id\": \"$worker_id\""* ]]; then
  echo "$verify_output"
  echo "expected verified token output for worker $worker_id" >&2
  exit 1
fi

echo "==> Confirming revoked token IDs are rejected"
if go run ./cmd/stacyvm worker token verify "$token" \
  --signing-key-file "$signing_key_file" \
  --worker-id "$worker_id" \
  --audience "$audience" \
  --revoked-token-id "$token_id" >"$tmpdir/revoked-token.out" 2>&1; then
  cat "$tmpdir/revoked-token.out"
  echo "expected revoked token verification to fail" >&2
  exit 1
fi

echo "==> Generating no-secret rotation plan"
go run ./cmd/stacyvm worker token rotation-plan \
  --new-key-ref "$signing_key_file" \
  --previous-key-ref "$old_signing_key_file" \
  --ttl "$ttl" >/dev/null

echo "==> Worker identity certification passed for $worker_id"
