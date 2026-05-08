#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${STACYVM_SMOKE_URL:-${1:-http://127.0.0.1:7423}}"
API_KEY="${STACYVM_API_KEY:-${2:-}}"
TIMEOUT_SECONDS="${STACYVM_SMOKE_TIMEOUT:-5}"

BASE_URL="${BASE_URL%/}"

headers=()
if [[ -n "$API_KEY" ]]; then
  headers=(-H "X-API-Key: $API_KEY")
fi

curl_base=(curl --silent --show-error --fail --max-time "$TIMEOUT_SECONDS" "${headers[@]}")

probe_json() {
  local path="$1"
  local expected="$2"
  local url="$BASE_URL$path"

  printf 'Checking %s ... ' "$url"
  local body
  body="$("${curl_base[@]}" "$url")"
  if [[ "$body" != *"$expected"* ]]; then
    printf 'failed\n'
    printf 'Expected response to contain: %s\n' "$expected" >&2
    printf 'Response:\n%s\n' "$body" >&2
    return 1
  fi
  printf 'ok\n'
}

probe_metrics() {
  local path="/api/v1/metrics/prometheus"
  local url="$BASE_URL$path"

  printf 'Checking %s ... ' "$url"
  local body
  body="$("${curl_base[@]}" "$url")"
  if [[ "$body" != *"stacyvm_uptime_seconds"* ]]; then
    printf 'failed\n'
    printf 'Expected Prometheus metrics to contain stacyvm_uptime_seconds.\n' >&2
    printf 'Response:\n%s\n' "$body" >&2
    return 1
  fi
  printf 'ok\n'
}

probe_json "/api/v1/live" '"status":"alive"'
probe_json "/api/v1/health" '"status":"ok"'
probe_json "/api/v1/ready" '"status":"ready"'
probe_metrics

printf 'StacyVM deployment smoke checks passed for %s\n' "$BASE_URL"
