#!/usr/bin/env bash
set -euo pipefail

runtime="all"
format="text"
output=""

usage() {
  cat <<'USAGE'
usage: scripts/certify-runtime.sh [all|docker|gvisor|kata|firecracker|proot] [--format text|json|markdown] [--output path]

Runs host-level runtime dependency checks and can write a durable certification
artifact for production signoff.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    all|docker|gvisor|kata|firecracker|proot)
      runtime="$1"
      shift
      ;;
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
    *)
      usage >&2
      exit 2
      ;;
  esac
done

case "$format" in
  text|json|markdown) ;;
  *)
    printf 'unsupported format: %s\n' "$format" >&2
    exit 2
    ;;
esac

exit_code=0
RESULTS=()

record() {
  local status="$1"
  local name="$2"
  local message="$3"
  RESULTS+=("${status}|${name}|${message}")
  if [ "$format" = "text" ]; then
    printf '[%s] %s: %s\n' "$status" "$name" "$message"
  fi
  if [ "$status" = "FAIL" ]; then
    exit_code=1
  fi
}

pass() { record "PASS" "$1" "$2"; }
warn() { record "WARN" "$1" "$2"; }
fail() { record "FAIL" "$1" "$2"; }

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

host_os() {
  uname -srm 2>/dev/null || printf 'unknown'
}

host_id() {
  hostname 2>/dev/null || printf 'unknown'
}

generated_at() {
  date -u '+%Y-%m-%dT%H:%M:%SZ'
}

docker_runtimes() {
  docker info --format '{{json .Runtimes}}' 2>/dev/null || true
}

check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker.cli" "Docker CLI not found"
    return
  fi
  pass "docker.cli" "Docker CLI found"
  if docker info >/dev/null 2>&1; then
    pass "docker.daemon" "Docker daemon reachable"
  else
    fail "docker.daemon" "Docker daemon unreachable"
  fi
  if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eq 'seccomp|name=seccomp'; then
    pass "docker.seccomp" "seccomp advertised by docker info"
  else
    warn "docker.seccomp" "seccomp not advertised by docker info"
  fi
}

check_gvisor() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    fail "gvisor.discovery" "Docker daemon unavailable; cannot discover runsc runtime"
    return
  fi
  if docker_runtimes | grep -Eq 'runsc|gvisor'; then
    pass "gvisor.runtime" "runsc/gVisor runtime discovered"
  else
    fail "gvisor.runtime" "runsc/gVisor runtime not discovered"
  fi
}

check_kata() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    fail "kata.discovery" "Docker daemon unavailable; cannot discover Kata runtime"
    return
  fi
  if docker_runtimes | grep -Eq 'kata'; then
    pass "kata.runtime" "Kata runtime discovered"
  else
    fail "kata.runtime" "Kata runtime not discovered"
  fi
}

check_firecracker() {
  if command -v firecracker >/dev/null 2>&1; then
    pass "firecracker.binary" "Firecracker binary found"
  else
    fail "firecracker.binary" "Firecracker binary not found"
  fi
  if [ -e /dev/kvm ]; then
    pass "firecracker.kvm" "/dev/kvm present"
  else
    fail "firecracker.kvm" "/dev/kvm missing"
  fi
  if [ -n "${STACYVM_FIRECRACKER_KERNEL:-}" ] && [ -f "$STACYVM_FIRECRACKER_KERNEL" ]; then
    pass "firecracker.kernel" "kernel image exists at STACYVM_FIRECRACKER_KERNEL"
  elif [ -n "${STACYVM_FIRECRACKER_KERNEL:-}" ]; then
    fail "firecracker.kernel" "kernel image missing at STACYVM_FIRECRACKER_KERNEL"
  else
    warn "firecracker.kernel" "STACYVM_FIRECRACKER_KERNEL not set; validate configured kernel_path separately"
  fi
}

check_proot() {
  if command -v proot >/dev/null 2>&1; then
    pass "proot.binary" "PRoot binary found"
  else
    fail "proot.binary" "PRoot binary not found"
  fi
  if [ -n "${STACYVM_PROOT_ROOTFS:-}" ] && [ -d "$STACYVM_PROOT_ROOTFS" ]; then
    pass "proot.rootfs" "rootfs directory exists at STACYVM_PROOT_ROOTFS"
  elif [ -n "${STACYVM_PROOT_ROOTFS:-}" ]; then
    fail "proot.rootfs" "rootfs directory missing at STACYVM_PROOT_ROOTFS"
  else
    warn "proot.rootfs" "STACYVM_PROOT_ROOTFS not set; validate configured rootfs_path separately"
  fi
  if [ -n "${STACYVM_PROOT_WORKSPACE_BASE:-}" ] && [ -w "$STACYVM_PROOT_WORKSPACE_BASE" ]; then
    pass "proot.workspace" "workspace base is writable"
  elif [ -n "${STACYVM_PROOT_WORKSPACE_BASE:-}" ]; then
    fail "proot.workspace" "workspace base is not writable"
  else
    warn "proot.workspace" "STACYVM_PROOT_WORKSPACE_BASE not set; validate configured workspace_base separately"
  fi
}

run_checks() {
  case "$runtime" in
    all)
      check_docker
      check_gvisor
      check_kata
      check_firecracker
      check_proot
      ;;
    docker)
      check_docker
      ;;
    gvisor)
      check_gvisor
      ;;
    kata)
      check_kata
      ;;
    firecracker)
      check_firecracker
      ;;
    proot)
      check_proot
      ;;
  esac
}

write_json() {
  {
    printf '{\n'
    printf '  "generated_at": "%s",\n' "$(generated_at)"
    printf '  "runtime": "%s",\n' "$(json_escape "$runtime")"
    printf '  "host": {\n'
    printf '    "hostname": "%s",\n' "$(json_escape "$(host_id)")"
    printf '    "os": "%s"\n' "$(json_escape "$(host_os)")"
    printf '  },\n'
    printf '  "status": "%s",\n' "$([ "$exit_code" -eq 0 ] && printf PASS || printf FAIL)"
    printf '  "checks": [\n'
    local first=1
    local entry status name message
    for entry in "${RESULTS[@]}"; do
      IFS='|' read -r status name message <<<"$entry"
      if [ "$first" -eq 0 ]; then
        printf ',\n'
      fi
      first=0
      printf '    {"status": "%s", "name": "%s", "message": "%s"}' "$(json_escape "$status")" "$(json_escape "$name")" "$(json_escape "$message")"
    done
    printf '\n  ]\n'
    printf '}\n'
  }
}

write_markdown() {
  {
    printf '# StacyVM Runtime Certification\n\n'
    printf '%s\n' "- Generated at: \`$(generated_at)\`"
    printf '%s\n' "- Runtime target: \`$runtime\`"
    printf '%s\n' "- Host: \`$(host_id)\`"
    printf '%s\n' "- OS/kernel: \`$(host_os)\`"
    printf '%s\n\n' "- Overall status: \`$([ "$exit_code" -eq 0 ] && printf PASS || printf FAIL)\`"
    printf '## Checks\n\n'
    printf '| Status | Check | Message |\n'
    printf '|---|---|---|\n'
    local entry status name message
    for entry in "${RESULTS[@]}"; do
      IFS='|' read -r status name message <<<"$entry"
      printf '| `%s` | `%s` | %s |\n' "$status" "$name" "$message"
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
}

run_checks

case "$format" in
  text)
    ;;
  json)
    if [ -n "$output" ]; then
      write_json >"$output"
    else
      write_json
    fi
    ;;
  markdown)
    if [ -n "$output" ]; then
      write_markdown >"$output"
    else
      write_markdown
    fi
    ;;
esac

if [ -n "$output" ] && [ "$format" != "text" ]; then
  printf 'certification report written: %s\n' "$output"
fi

exit "$exit_code"
