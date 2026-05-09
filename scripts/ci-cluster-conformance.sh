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
  admin_fallback_enabled: false' \
  "$cluster_config" >"$signed_worker_config"
go run ./cmd/stacyvm config lint --production --file "$signed_worker_config"

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

echo "==> Cluster conformance CI checks passed"
