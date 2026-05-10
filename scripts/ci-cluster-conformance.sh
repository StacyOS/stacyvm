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

echo "==> Running always-on SQLite store contract"
go test ./internal/store -run TestSQLiteStoreContract

echo "==> Running worker identity and worker route auth checks"
go test ./internal/api/middleware ./internal/api \
  -run 'TestWorkerAuth|TestWorkerHeartbeatUsesPerWorkerToken|TestWorkerRenewLeaseUsesWorkerToken'

echo "==> Running worker RPC mTLS conformance"
go test ./internal/worker -run TestRPCClientMTLSConformance -count=1

echo "==> Running signed worker RPC conformance"
go test ./internal/worker ./internal/orchestrator \
  -run 'TestRPCClientStatusWithSignedToken|TestRPCServerRejectsRevokedSignedToken|TestManager_RemoteSpawnUsesSignedWorkerRPC' -count=1

cluster_config="$tmpdir/stacyvm.cluster.yaml"
cat >"$cluster_config" <<YAML
server:
  host: "0.0.0.0"
  port: 7423
  preview_domain: "localhost"

worker:
  id: "worker-a"
  control_plane_url: "http://127.0.0.1:7423"
  listen_addr: "127.0.0.1:7430"
  heartbeat_interval: "30s"
  shutdown_timeout: "10s"

providers:
  default: "docker"
  docker:
    enabled: true
    socket: "unix:///var/run/docker.sock"
    runtime: "runc"
    default_image: "alpine:latest"
    network_mode: "stacyvm-network"
    seccomp_profile: "default"
    read_only_rootfs: false
    memory: "512m"
    cpus: "1"
    pids_limit: 256
    user: "1000:1000"
    dropped_caps: ["ALL"]
    added_caps: []
  firecracker:
    enabled: false
  proot:
    enabled: false

defaults:
  ttl: "30m"
  image: "alpine:latest"
  memory_mb: 1024
  vcpus: 1
  disk_size_mb: 1024
  max_ttl: "24h"
  default_exec_timeout: "30s"
  max_exec_timeout: "10m"
  max_sandboxes: 100
  max_sandboxes_per_owner: 10
  spawn_overflow: "queue"
  spawn_queue_timeout: "30s"
  max_spawn_queue: 100

auth:
  enabled: true
  api_key: "regular-api-key-with-at-least-32-bytes"
  admin_api_key: "admin-api-key-with-at-least-32-bytesxx"
  worker_tokens:
    worker-a: "worker-a-token-with-at-least-32-bytes"
    worker-b: "worker-b-token-with-at-least-32-bytes"
  admin_fallback_enabled: false
  admin_audit_retention: "2160h"

rate_limit:
  enabled: true
  requests_per_minute: 120
  burst: 60
  key_by: "api_key"
  bucket_ttl: "15m"
  cleanup_interval: "1m"

database:
  driver: "sqlite"
  path: "$tmpdir/stacyvm-cluster.db"

logging:
  level: "info"
  format: "json"
YAML

echo "==> Linting production-aligned cluster config"
go run ./cmd/stacyvm config lint --production --file "$cluster_config"

echo "==> Linting signed-token worker identity config"
signed_worker_config="$tmpdir/stacyvm.signed-worker.yaml"
sed \
  -e '/worker_tokens:/,/admin_fallback_enabled:/c\
  worker_signing_key: "worker-signing-key-with-at-least-32-bytes"\
  worker_signing_keys: ["old-worker-signing-key-with-at-least-32-bytes"]\
  admin_fallback_enabled: false' \
  "$cluster_config" >"$signed_worker_config"
go run ./cmd/stacyvm config lint --production --file "$signed_worker_config"

echo "==> Running worker identity certification smoke"
worker_identity_report="$tmpdir/worker-identity-certification.md"
scripts/certify-worker-identity.sh worker-a --format markdown --output "$worker_identity_report"
if [[ ! -s "$worker_identity_report" ]]; then
  echo "expected worker identity certification report to be written" >&2
  exit 1
fi
if [[ "$(cat "$worker_identity_report")" != *"Worker Identity Certification"* ]]; then
  cat "$worker_identity_report"
  echo "expected worker identity certification markdown report" >&2
  exit 1
fi
if grep -q "stacyvm-worker-v1" "$worker_identity_report"; then
  cat "$worker_identity_report"
  echo "worker identity certification report must not include token values" >&2
  exit 1
fi

echo "==> Linting signed-token worker identity migration warnings"
mixed_worker_config="$tmpdir/stacyvm.signed-worker-mixed.yaml"
sed \
  -e '/worker_tokens:/,/admin_fallback_enabled:/c\
  worker_token: "shared-worker-token-with-at-least-32-bytes"\
  worker_signing_key: "worker-signing-key-with-at-least-32-bytes"\
  admin_fallback_enabled: false' \
  "$cluster_config" >"$mixed_worker_config"
mixed_lint_output="$(go run ./cmd/stacyvm config lint --production --file "$mixed_worker_config")"
if [[ "$mixed_lint_output" != *"shared worker token still configured with signed worker tokens"* ]]; then
  echo "$mixed_lint_output"
  echo "expected signed-token migration warning for shared worker token" >&2
  exit 1
fi

invalid_rotation_config="$tmpdir/stacyvm.invalid-rotation.yaml"
sed \
  -e '/worker_tokens:/,/admin_fallback_enabled:/c\
  worker_signing_key: "worker-signing-key-with-at-least-32-bytes"\
  worker_signing_keys: ["worker-signing-key-with-at-least-32-bytes"]\
  admin_fallback_enabled: false' \
  "$cluster_config" >"$invalid_rotation_config"
rotation_lint_output="$(go run ./cmd/stacyvm config lint --production --file "$invalid_rotation_config")"
if [[ "$rotation_lint_output" != *"rotation keys include the active worker signing key"* ]]; then
  echo "$rotation_lint_output"
  echo "expected signed-token migration warning for invalid rotation keys" >&2
  exit 1
fi

echo "==> Linting production-aligned Postgres cluster config"
postgres_config="$tmpdir/stacyvm.postgres.yaml"
sed \
  -e 's/driver: "sqlite"/driver: "postgres"/' \
  -e 's|path: "'"$tmpdir"'/stacyvm-cluster.db"|dsn: "postgres://stacyvm:stacyvm@127.0.0.1:5432/stacyvm?sslmode=disable"|' \
  "$cluster_config" >"$postgres_config"
go run ./cmd/stacyvm config lint --production --file "$postgres_config"

if [[ -n "${STACYVM_POSTGRES_TEST_DSN:-}" ]]; then
  echo "==> Running live Postgres store contract"
  go test ./internal/store -run 'TestPostgresStoreContract|TestPostgresLeaseConcurrency|TestPostgresMigrationRehearsal' -count=1
else
  echo "==> Skipping live Postgres store contract; STACYVM_POSTGRES_TEST_DSN is not set"
fi

echo "==> Linting OIDC-enabled cluster config"
oidc_config="$tmpdir/stacyvm.oidc.yaml"
cat >"$oidc_config" <<YAML
server:
  host: "0.0.0.0"
  port: 7423
providers:
  default: "mock"
  mock:
    enabled: true
  docker:
    enabled: false
  firecracker:
    enabled: false
auth:
  enabled: true
  api_key: "api-key-with-at-least-32-bytes-longxx"
  admin_api_key: "admin-api-key-with-at-least-32-bytesxx"
  admin_fallback_enabled: false
  admin_audit_retention: "2160h"
  worker_signing_key: "worker-signing-key-with-at-least-32-bytes"
  oidc_enabled: true
  oidc_issuer: "https://accounts.example.com"
  oidc_audience: "stacyvm"
  oidc_jwks_url: "https://accounts.example.com/.well-known/jwks.json"
  oidc_admin_groups: ["stacyvm-admins"]
  oidc_operator_groups: ["stacyvm-operators"]
rate_limit:
  enabled: true
  requests_per_minute: 120
  burst: 60
  key_by: "api_key"
  bucket_ttl: "15m"
  cleanup_interval: "1m"
database:
  driver: "sqlite"
  path: "$tmpdir/stacyvm-oidc.db"
logging:
  level: "info"
  format: "json"
YAML
oidc_lint_output="$(go run ./cmd/stacyvm config lint --file "$oidc_config" 2>&1)"
if [[ "$oidc_lint_output" != *"auth.oidc_enabled: enabled"* ]]; then
  echo "$oidc_lint_output"
  echo "expected OIDC enabled lint pass" >&2
  exit 1
fi
if [[ "$oidc_lint_output" != *"auth.oidc_issuer"* ]]; then
  echo "$oidc_lint_output"
  echo "expected OIDC issuer lint check" >&2
  exit 1
fi
if [[ "$oidc_lint_output" != *"auth.oidc_groups"* ]]; then
  echo "$oidc_lint_output"
  echo "expected OIDC group mapping lint check" >&2
  exit 1
fi
echo "    OIDC config lint: OK"

echo "==> Running remote worker mTLS smoke with ephemeral certificates"
# Build the binary into the tmpdir if not already built.
SMOKE_BIN="$tmpdir/stacyvm"
if [[ ! -x "$SMOKE_BIN" ]]; then
  go build -o "$SMOKE_BIN" ./cmd/stacyvm
fi
scripts/smoke-remote-worker.sh "$SMOKE_BIN" --mtls

echo "==> Cluster conformance CI checks passed"
