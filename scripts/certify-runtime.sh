#!/usr/bin/env bash
set -euo pipefail

runtime="${1:-all}"

pass() { printf '[PASS] %s\n' "$1"; }
warn() { printf '[WARN] %s\n' "$1"; }
fail() {
  printf '[FAIL] %s\n' "$1"
  exit_code=1
}

exit_code=0

check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker: CLI not found"
    return
  fi
  pass "docker: CLI found"
  if docker info >/dev/null 2>&1; then
    pass "docker: daemon reachable"
  else
    fail "docker: daemon unreachable"
  fi
  if docker info --format '{{json .SecurityOptions}}' 2>/dev/null | grep -Eq 'seccomp|name=seccomp'; then
    pass "docker: seccomp advertised"
  else
    warn "docker: seccomp not advertised by docker info"
  fi
}

check_gvisor_kata() {
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    warn "gvisor/kata: docker daemon unavailable; skipping runtime discovery"
    return
  fi
  runtimes="$(docker info --format '{{json .Runtimes}}' 2>/dev/null || true)"
  if printf '%s' "$runtimes" | grep -Eq 'runsc|gvisor'; then
    pass "gvisor: runsc runtime discovered"
  else
    warn "gvisor: runsc runtime not discovered"
  fi
  if printf '%s' "$runtimes" | grep -Eq 'kata'; then
    pass "kata: runtime discovered"
  else
    warn "kata: runtime not discovered"
  fi
}

check_firecracker() {
  if command -v firecracker >/dev/null 2>&1; then
    pass "firecracker: binary found"
  else
    fail "firecracker: binary not found"
  fi
  if [ -e /dev/kvm ]; then
    pass "firecracker: /dev/kvm present"
  else
    fail "firecracker: /dev/kvm missing"
  fi
}

check_proot() {
  if command -v proot >/dev/null 2>&1; then
    pass "proot: binary found"
  else
    fail "proot: binary not found"
  fi
}

case "$runtime" in
  all)
    check_docker
    check_gvisor_kata
    check_firecracker
    check_proot
    ;;
  docker)
    check_docker
    ;;
  gvisor|kata)
    check_gvisor_kata
    ;;
  firecracker)
    check_firecracker
    ;;
  proot)
    check_proot
    ;;
  *)
    printf 'usage: %s [all|docker|gvisor|kata|firecracker|proot]\n' "$0" >&2
    exit 2
    ;;
esac

exit "$exit_code"
