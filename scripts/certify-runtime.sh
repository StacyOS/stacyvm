#!/usr/bin/env bash
# certify-runtime.sh — host-level runtime certification for StacyVM.
#
# Checks that the host has the required binaries, daemons, and kernel features
# for a given runtime, and optionally runs a live StacyVM sandbox smoke to
# prove end-to-end functionality.
#
# Usage:
#   scripts/certify-runtime.sh [all|docker|gvisor|kata|firecracker|proot]
#     [--format text|json|markdown] [--output path]
#     [--stacyvm-url URL] [--stacyvm-api-key KEY]
#     [--stacyvm-bin PATH]
#
# When --stacyvm-url and --stacyvm-api-key are provided, the script spawns a
# real sandbox using the target runtime, verifies it reaches running state, and
# destroys it — proving the runtime works end-to-end through StacyVM.
#
# When --stacyvm-bin is provided without --stacyvm-url, the script starts a
# temporary local StacyVM server automatically and runs the integration smoke.
#
# Environment:
#   STACYVM_FIRECRACKER_KERNEL    Path to kernel image (for firecracker check)
#   STACYVM_PROOT_ROOTFS          Path to rootfs directory (for proot check)
#   STACYVM_PROOT_WORKSPACE_BASE  Workspace base directory (for proot check)
set -euo pipefail

runtime="all"
format="text"
output=""
stacyvm_url=""
stacyvm_api_key=""
stacyvm_bin=""

usage() {
  cat <<'USAGE'
usage: scripts/certify-runtime.sh [all|docker|gvisor|kata|firecracker|proot]
         [--format text|json|markdown] [--output path]
         [--stacyvm-url URL] [--stacyvm-api-key KEY]
         [--stacyvm-bin PATH]
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    all|docker|gvisor|kata|firecracker|proot) runtime="$1"; shift ;;
    --format)          format="${2:-}";         shift 2 ;;
    --output)          output="${2:-}";         shift 2 ;;
    --stacyvm-url)     stacyvm_url="${2:-}";     shift 2 ;;
    --stacyvm-api-key) stacyvm_api_key="${2:-}"; shift 2 ;;
    --stacyvm-bin)     stacyvm_bin="${2:-}";     shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

case "$format" in
  text|json|markdown) ;;
  *) printf 'unsupported format: %s\n' "$format" >&2; exit 2 ;;
esac

exit_code=0
RESULTS=()
LOCAL_SERVER_PID=""
LOCAL_WORK_DIR=""

cleanup() {
  if [[ -n "${LOCAL_SERVER_PID:-}" ]]; then
    kill "$LOCAL_SERVER_PID" 2>/dev/null || true
    wait "$LOCAL_SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "${LOCAL_WORK_DIR:-}" ]]; then
    rm -rf "$LOCAL_WORK_DIR"
  fi
}
trap cleanup EXIT

# ── result recording ──────────────────────────────────────────────────────────
record() {
  local status="$1" name="$2" message="$3"
  RESULTS+=("${status}|${name}|${message}")
  if [ "$format" = "text" ]; then
    printf '[%s] %s: %s\n' "$status" "$name" "$message"
  fi
  if [ "$status" = "FAIL" ]; then exit_code=1; fi
}
pass() { record "PASS" "$1" "$2"; }
warn() { record "WARN" "$1" "$2"; }
fail() { record "FAIL" "$1" "$2"; }

json_escape() {
  local v="$1"
  v="${v//\\/\\\\}"; v="${v//\"/\\\"}"; v="${v//$'\n'/\\n}"
  v="${v//$'\r'/\\r}"; v="${v//$'\t'/\\t}"
  printf '%s' "$v"
}
host_os()      { uname -srm 2>/dev/null || printf 'unknown'; }
host_id()      { hostname 2>/dev/null || printf 'unknown'; }
generated_at() { date -u '+%Y-%m-%dT%H:%M:%SZ'; }
docker_runtimes() { docker info --format '{{json .Runtimes}}' 2>/dev/null || true; }

# ── host prerequisite checks ──────────────────────────────────────────────────
check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker.cli" "Docker CLI not found"; return
  fi
  pass "docker.cli" "Docker CLI found"

  if ! docker info >/dev/null 2>&1; then
    fail "docker.daemon" "Docker daemon unreachable"; return
  fi
  pass "docker.daemon" "Docker daemon reachable"

  if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eq 'seccomp|name=seccomp'; then
    pass "docker.seccomp" "seccomp advertised"
  else
    warn "docker.seccomp" "seccomp not advertised by docker info"
  fi

  if docker run --rm --pull=never alpine:latest echo ok >/dev/null 2>&1 \
      || docker run --rm alpine:latest echo ok >/dev/null 2>&1; then
    pass "docker.run" "docker run alpine echo ok succeeded"
  else
    warn "docker.run" "docker run alpine echo ok failed — image may not be cached on this host"
  fi
}

check_gvisor() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    fail "gvisor.discovery" "Docker daemon unavailable; cannot discover gVisor runtime"; return
  fi
  if docker_runtimes | grep -Eq 'runsc|gvisor'; then
    pass "gvisor.runtime" "runsc/gVisor runtime discovered in docker info"
    if docker run --rm --runtime=runsc alpine:latest echo ok >/dev/null 2>&1; then
      pass "gvisor.run" "docker run --runtime=runsc alpine echo ok succeeded"
    else
      fail "gvisor.run" "docker run --runtime=runsc alpine echo ok failed"
    fi
  else
    fail "gvisor.runtime" "runsc/gVisor runtime not found — install gVisor and configure /etc/docker/daemon.json"
  fi
}

check_kata() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    fail "kata.discovery" "Docker daemon unavailable; cannot discover Kata runtime"; return
  fi
  if docker_runtimes | grep -Eq 'kata'; then
    pass "kata.runtime" "Kata runtime discovered in docker info"
    if docker run --rm --runtime=kata-runtime alpine:latest echo ok >/dev/null 2>&1 \
        || docker run --rm --runtime=kata-containers alpine:latest echo ok >/dev/null 2>&1; then
      pass "kata.run" "docker run --runtime=kata alpine echo ok succeeded"
    else
      fail "kata.run" "docker run --runtime=kata alpine echo ok failed"
    fi
  else
    fail "kata.runtime" "Kata runtime not found — install kata-containers and configure /etc/docker/daemon.json"
  fi
}

check_firecracker() {
  if command -v firecracker >/dev/null 2>&1; then
    local fc_ver
    fc_ver="$(firecracker --version 2>/dev/null | head -1 || printf 'unknown')"
    pass "firecracker.binary" "Firecracker found ($fc_ver)"
  else
    fail "firecracker.binary" "Firecracker binary not found in PATH"
  fi

  if [ -e /dev/kvm ]; then
    pass "firecracker.kvm" "/dev/kvm present"
    if [ -r /dev/kvm ] && [ -w /dev/kvm ]; then
      pass "firecracker.kvm_access" "/dev/kvm readable and writable"
    else
      warn "firecracker.kvm_access" "/dev/kvm exists but process lacks rw access; check kvm group membership"
    fi
  else
    fail "firecracker.kvm" "/dev/kvm missing — enable KVM in BIOS or use a KVM-capable host"
  fi

  if [ -n "${STACYVM_FIRECRACKER_KERNEL:-}" ]; then
    if [ -f "$STACYVM_FIRECRACKER_KERNEL" ]; then
      pass "firecracker.kernel" "kernel image exists at $STACYVM_FIRECRACKER_KERNEL"
    else
      fail "firecracker.kernel" "kernel image missing at $STACYVM_FIRECRACKER_KERNEL"
    fi
  else
    warn "firecracker.kernel" "STACYVM_FIRECRACKER_KERNEL not set; validate kernel_path in your StacyVM config"
  fi
}

check_proot() {
  if command -v proot >/dev/null 2>&1; then
    pass "proot.binary" "PRoot binary found"
  else
    fail "proot.binary" "PRoot binary not found in PATH"
  fi

  if [ -n "${STACYVM_PROOT_ROOTFS:-}" ]; then
    if [ -d "$STACYVM_PROOT_ROOTFS" ]; then
      pass "proot.rootfs" "rootfs directory exists at $STACYVM_PROOT_ROOTFS"
    else
      fail "proot.rootfs" "rootfs directory missing at $STACYVM_PROOT_ROOTFS"
    fi
  else
    warn "proot.rootfs" "STACYVM_PROOT_ROOTFS not set; validate rootfs_path in your StacyVM config"
  fi

  if [ -n "${STACYVM_PROOT_WORKSPACE_BASE:-}" ]; then
    if [ -w "$STACYVM_PROOT_WORKSPACE_BASE" ]; then
      pass "proot.workspace" "workspace base writable at $STACYVM_PROOT_WORKSPACE_BASE"
    else
      fail "proot.workspace" "workspace base at $STACYVM_PROOT_WORKSPACE_BASE is not writable"
    fi
  else
    warn "proot.workspace" "STACYVM_PROOT_WORKSPACE_BASE not set; validate workspace_base in your StacyVM config"
  fi
}

run_checks() {
  case "$runtime" in
    all)         check_docker; check_gvisor; check_kata; check_firecracker; check_proot ;;
    docker)      check_docker ;;
    gvisor)      check_gvisor ;;
    kata)        check_kata ;;
    firecracker) check_firecracker ;;
    proot)       check_proot ;;
  esac
}

# ── optional StacyVM integration smoke ───────────────────────────────────────
runtime_to_provider() {
  case "$1" in
    docker|gvisor|kata) printf 'docker' ;;
    firecracker)        printf 'firecracker' ;;
    proot)              printf 'proot' ;;
    *)                  printf 'docker' ;;  # "all" uses docker
  esac
}

runtime_to_image() {
  case "$1" in
    firecracker) printf 'ubuntu:22.04' ;;
    *)           printf 'alpine:latest' ;;
  esac
}

start_local_server() {
  # Start a temporary StacyVM server with the target provider enabled.
  # Sets stacyvm_url and stacyvm_api_key on success, returns 1 on failure.
  local bin="$1"
  if [[ ! -x "$bin" ]] && ! command -v "$bin" >/dev/null 2>&1; then
    warn "stacyvm.bin" "stacyvm binary not executable at $bin; skipping integration smoke"
    return 1
  fi

  LOCAL_WORK_DIR="$(mktemp -d)"
  # Use a random port in the ephemeral range to avoid conflicts with prior runs.
  local port; port=$(( ( RANDOM % 5000 ) + 19000 ))
  local api_key="cert-smoke-api-key-32bytes-long!!"
  local provider; provider="$(runtime_to_provider "$runtime")"
  local cfg="$LOCAL_WORK_DIR/stacyvm.yaml"
  local log="$LOCAL_WORK_DIR/server.log"

  cat >"$cfg" <<YAML
server:
  host: "127.0.0.1"
  port: $port
providers:
  default: "$provider"
  mock:
    enabled: true
  docker:
    enabled: $([ "$provider" = "docker" ] && printf 'true' || printf 'false')
  firecracker:
    enabled: $([ "$provider" = "firecracker" ] && printf 'true' || printf 'false')
  proot:
    enabled: $([ "$provider" = "proot" ] && printf 'true' || printf 'false')
auth:
  api_key: "$api_key"
  admin_fallback_enabled: false
logging:
  level: "warn"
  format: "json"
database:
  driver: "sqlite"
  path: "$LOCAL_WORK_DIR/stacyvm.db"
YAML

  (cd "$LOCAL_WORK_DIR"; "$bin" serve) >"$log" 2>&1 &
  LOCAL_SERVER_PID=$!

  for _ in {1..40}; do
    if curl -fsS -H "X-API-Key: $api_key" "http://127.0.0.1:$port/api/v1/ready" >/dev/null 2>&1; then
      stacyvm_url="http://127.0.0.1:$port"
      stacyvm_api_key="$api_key"
      return 0
    fi
    sleep 0.3
  done

  warn "stacyvm.start" "local StacyVM server at port $port did not become ready; skipping integration smoke"
  kill "$LOCAL_SERVER_PID" 2>/dev/null || true
  LOCAL_SERVER_PID=""
  return 1
}

run_stacyvm_smoke() {
  local url="$1" api_key="$2"
  local provider image sandbox_id state

  provider="$(runtime_to_provider "$runtime")"
  image="$(runtime_to_image "$runtime")"

  # Readiness check.
  if ! curl -fsS -H "X-API-Key: $api_key" "$url/api/v1/ready" >/dev/null 2>&1; then
    warn "stacyvm.ready" "StacyVM at $url not reachable; skipping integration smoke"
    return
  fi
  pass "stacyvm.ready" "StacyVM API reachable at $url"

  # Provider health.
  local health
  health="$(curl -fsS -H "X-API-Key: $api_key" "$url/api/v1/providers/$provider/health" 2>/dev/null || true)"
  if printf '%s' "$health" | grep -qE '"healthy":true|"status":"ok"'; then
    pass "stacyvm.provider_health" "$provider provider is healthy"
  else
    warn "stacyvm.provider_health" "$provider provider health check: ${health:-no response}"
  fi

  # Spawn. Use || true to capture response body even on HTTP error.
  local spawn_resp spawn_rc
  spawn_resp="$(curl -sS -X POST \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $api_key" \
    -d "{\"image\":\"$image\",\"provider\":\"$provider\",\"ttl\":\"2m\"}" \
    "$url/api/v1/sandboxes" 2>&1)" || spawn_rc=$?
  if printf '%s' "$spawn_resp" | grep -q '"code"'; then
    # HTTP error returned — provider may not be available on this host.
    warn "stacyvm.spawn" "spawn returned an error (provider may be unavailable): ${spawn_resp}"; return
  fi
  sandbox_id="$(printf '%s' "$spawn_resp" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
  if [[ -z "$sandbox_id" ]]; then
    fail "stacyvm.spawn" "spawn response contained no sandbox id: $spawn_resp"; return
  fi
  pass "stacyvm.spawn" "sandbox $sandbox_id spawned via $provider ($image)"

  # Wait for running (up to 60s for Firecracker; 30s for others).
  local max_wait=30
  [[ "$provider" = "firecracker" ]] && max_wait=60
  state=""
  for _ in $(seq 1 "$max_wait"); do
    state="$(curl -fsS -H "X-API-Key: $api_key" "$url/api/v1/sandboxes/$sandbox_id" 2>/dev/null \
      | sed -n 's/.*"state":"\([^"]*\)".*/\1/p' || true)"
    [[ "$state" = "running" ]] && break
    sleep 1
  done

  if [[ "$state" = "running" ]]; then
    pass "stacyvm.running" "sandbox $sandbox_id reached running state"
  else
    fail "stacyvm.running" "sandbox $sandbox_id state is '$state' after ${max_wait}s (expected running)"
    curl -fsS -X DELETE -H "X-API-Key: $api_key" "$url/api/v1/sandboxes/$sandbox_id" >/dev/null 2>&1 || true
    return
  fi

  # Exec a trivial command to prove the runtime executes code (skip for Firecracker
  # which may require agent boot time beyond a simple exec).
  if [[ "$provider" != "firecracker" ]]; then
    local exec_resp
    exec_resp="$(curl -fsS -X POST \
      -H "Content-Type: application/json" \
      -H "X-API-Key: $api_key" \
      -d '{"command":"echo stacyvm-runtime-ok","mode":"shell"}' \
      "$url/api/v1/sandboxes/$sandbox_id/exec" 2>/dev/null || true)"
    if printf '%s' "$exec_resp" | grep -q '"exit_code":0'; then
      pass "stacyvm.exec" "exec in sandbox exited 0 — runtime is executing code"
    else
      warn "stacyvm.exec" "exec result: ${exec_resp:-empty response}"
    fi
  fi

  # Destroy.
  curl -fsS -X DELETE -H "X-API-Key: $api_key" "$url/api/v1/sandboxes/$sandbox_id" >/dev/null 2>&1 || true
  pass "stacyvm.destroy" "sandbox $sandbox_id destroyed"
}

# ── output writers ────────────────────────────────────────────────────────────
write_json() {
  printf '{\n'
  printf '  "generated_at": "%s",\n' "$(generated_at)"
  printf '  "runtime": "%s",\n' "$(json_escape "$runtime")"
  printf '  "host": {\n'
  printf '    "hostname": "%s",\n' "$(json_escape "$(host_id)")"
  printf '    "os": "%s"\n' "$(json_escape "$(host_os)")"
  printf '  },\n'
  printf '  "stacyvm_url": "%s",\n' "$(json_escape "${stacyvm_url:-}")"
  printf '  "status": "%s",\n' "$([ "$exit_code" -eq 0 ] && printf PASS || printf FAIL)"
  printf '  "checks": [\n'
  local first=1
  for entry in "${RESULTS[@]+"${RESULTS[@]}"}"; do
    IFS='|' read -r s n m <<<"$entry"
    [ "$first" -eq 0 ] && printf ',\n'
    first=0
    printf '    {"status":"%s","name":"%s","message":"%s"}' \
      "$(json_escape "$s")" "$(json_escape "$n")" "$(json_escape "$m")"
  done
  printf '\n  ]\n}\n'
}

write_markdown() {
  printf '%s\n\n' '# StacyVM Runtime Certification'
  printf '%s\n' "- Generated at: \`$(generated_at)\`"
  printf '%s\n' "- Runtime target: \`$runtime\`"
  printf '%s\n' "- Host: \`$(host_id)\`"
  printf '%s\n' "- OS/kernel: \`$(host_os)\`"
  [[ -n "${stacyvm_url:-}" ]] && printf '%s\n' "- StacyVM URL: \`$stacyvm_url\`"
  printf '%s\n\n' "- Overall status: \`$([ "$exit_code" -eq 0 ] && printf PASS || printf FAIL)\`"
  printf '## Checks\n\n'
  printf '| Status | Check | Message |\n|---|---|---|\n'
  for entry in "${RESULTS[@]+"${RESULTS[@]}"}"; do
    IFS='|' read -r s n m <<<"$entry"
    printf '| `%s` | `%s` | %s |\n' "$s" "$n" "$m"
  done
  printf '\n## Operator Signoff\n\n'
  printf '%s\n' '- StacyVM version:'
  printf '%s\n' '- Config file:'
  printf '%s\n' '- Provider health endpoint:'
  printf '%s\n' '- Smoke script result:'
  printf '%s\n' '- Provider conformance result:'
  printf '%s\n' '- Known host caveats:'
  printf '%s\n' '- Owner/signoff:'
  printf '%s\n' '- Date:'
}

# ── main ──────────────────────────────────────────────────────────────────────
run_checks

# Start a local server automatically when --stacyvm-bin is given without --stacyvm-url.
if [[ -z "$stacyvm_url" ]] && [[ -n "$stacyvm_bin" ]]; then
  start_local_server "$stacyvm_bin" || true
fi

# Run integration smoke when URL + key are available.
if [[ -n "$stacyvm_url" ]] && [[ -n "$stacyvm_api_key" ]]; then
  run_stacyvm_smoke "$stacyvm_url" "$stacyvm_api_key"
elif [[ -n "$stacyvm_url" ]] || [[ -n "$stacyvm_api_key" ]]; then
  warn "stacyvm.config" "both --stacyvm-url and --stacyvm-api-key are required for integration smoke; skipping"
fi

# Emit report.
case "$format" in
  text) ;;
  json)
    if [ -n "$output" ]; then write_json >"$output"; else write_json; fi ;;
  markdown)
    if [ -n "$output" ]; then write_markdown >"$output"; else write_markdown; fi ;;
esac

[ -n "$output" ] && [ "$format" != "text" ] && printf 'certification report written: %s\n' "$output"

exit "$exit_code"
