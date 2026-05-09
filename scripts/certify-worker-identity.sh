#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: scripts/certify-worker-identity.sh [worker-id] [--format text|json|markdown] [--output path]

Runs a signed worker token lifecycle smoke without writing token values to the report.

Environment:
  STACYVM_WORKER_SIGNING_KEY_FILE       Active worker signing key file
  STACYVM_OLD_WORKER_SIGNING_KEY_FILE   Previous worker signing key file
  STACYVM_WORKER_IDENTITY_AUDIENCE      Expected audience, default worker:control-plane
  STACYVM_WORKER_IDENTITY_TTL           Token lifetime, default 5m
  STACYVM_WORKER_IDENTITY_TOKEN_ID      Token ID, default certification-token-id
USAGE
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

worker_id="worker-a"
format="text"
output=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --format)
      format="${2:-}"
      shift 2
      ;;
    --output)
      output="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      worker_id="$1"
      shift
      ;;
  esac
done

case "$format" in
  text|json|markdown) ;;
  *)
    echo "format must be text, json, or markdown" >&2
    exit 2
    ;;
esac

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

export GOCACHE="${GOCACHE:-$tmpdir/go-build}"

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

echo "==> Issuing signed worker token" >&2
token="$(
  go run ./cmd/stacyvm worker token "$worker_id" \
    --signing-key-file "$signing_key_file" \
    --ttl "$ttl" \
    --audience "$audience" \
    --token-id "$token_id"
)"

echo "==> Inspecting signed worker token metadata" >&2
inspect_output="$(
  go run ./cmd/stacyvm worker token inspect "$token"
)"
if [[ "$inspect_output" != *"\"signature_verified\": false"* || "$inspect_output" != *"\"token_id\": \"$token_id\""* ]]; then
  echo "$inspect_output"
  echo "expected unverified inspect output with token_id $token_id" >&2
  exit 1
fi

echo "==> Verifying signed worker token" >&2
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

echo "==> Confirming revoked token IDs are rejected" >&2
if go run ./cmd/stacyvm worker token verify "$token" \
  --signing-key-file "$signing_key_file" \
  --worker-id "$worker_id" \
  --audience "$audience" \
  --revoked-token-id "$token_id" >"$tmpdir/revoked-token.out" 2>&1; then
  cat "$tmpdir/revoked-token.out"
  echo "expected revoked token verification to fail" >&2
  exit 1
fi

echo "==> Generating no-secret rotation plan" >&2
rotation_plan="$(
  go run ./cmd/stacyvm worker token rotation-plan \
  --new-key-ref "$signing_key_file" \
  --previous-key-ref "$old_signing_key_file" \
    --ttl "$ttl"
)"

echo "==> Worker identity certification passed for $worker_id" >&2

generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
report_file="$tmpdir/report"
case "$format" in
  text)
    cat >"$report_file" <<REPORT
Worker identity certification
status: pass
generated_at: $generated_at
worker_id: $worker_id
audience: $audience
ttl: $ttl
token_id: $token_id
checks:
- issued signed worker token from secret file
- inspected unverified metadata
- verified signature, worker ID, and audience
- confirmed revoked token ID rejection
- generated no-secret rotation plan
REPORT
    ;;
  markdown)
    cat >"$report_file" <<REPORT
# Worker Identity Certification

| Field | Value |
|---|---|
| Status | pass |
| Generated at | $generated_at |
| Worker ID | $worker_id |
| Audience | $audience |
| TTL | $ttl |
| Token ID | $token_id |

## Checks

- Issued signed worker token from secret file.
- Inspected unverified metadata.
- Verified signature, worker ID, and audience.
- Confirmed revoked token ID rejection.
- Generated no-secret rotation plan.

## Rotation Plan

\`\`\`text
$rotation_plan
\`\`\`
REPORT
    ;;
  json)
    cat >"$report_file" <<REPORT
{
  "status": "pass",
  "generated_at": "$generated_at",
  "worker_id": "$worker_id",
  "audience": "$audience",
  "ttl": "$ttl",
  "token_id": "$token_id",
  "checks": [
    "issued signed worker token from secret file",
    "inspected unverified metadata",
    "verified signature, worker ID, and audience",
    "confirmed revoked token ID rejection",
    "generated no-secret rotation plan"
  ]
}
REPORT
    ;;
esac

if [[ -n "$output" ]]; then
  cp "$report_file" "$output"
  echo "worker identity certification report written: $output"
else
  cat "$report_file"
fi
