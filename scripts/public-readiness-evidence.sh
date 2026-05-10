#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUTPUT="${STACYVM_PUBLIC_READINESS_OUTPUT:-public-readiness-evidence.md}"
CONFIG_FILE="${STACYVM_PUBLIC_READINESS_CONFIG:-deploy/stacyvm.production.yaml}"
RUN_CLUSTER="${STACYVM_RUN_CLUSTER_CONFORMANCE:-false}"
POST_RELEASE_VERSION="${STACYVM_POST_RELEASE_VERSION:-}"
RUNTIMES="${STACYVM_RUNTIME_CERTIFY:-}"

usage() {
  cat <<'USAGE'
usage: scripts/public-readiness-evidence.sh [--output file] [--config file]

Generates a Markdown evidence report for the public self-serve go-live gate.
The report records local CI-equivalent checks and clearly marks tag/host-gated
items that must be captured before announcement.

Required environment for production config lint:
  STACYVM_AUTH_API_KEY
  STACYVM_AUTH_ADMIN_API_KEY

Optional environment:
  STACYVM_PUBLIC_READINESS_OUTPUT     Report path, default public-readiness-evidence.md
  STACYVM_PUBLIC_READINESS_CONFIG     Config path, default deploy/stacyvm.production.yaml
  STACYVM_RUN_CLUSTER_CONFORMANCE     true to run scripts/ci-cluster-conformance.sh
  STACYVM_POST_RELEASE_VERSION        Version tag for scripts/post-release-validate.sh
  STACYVM_VALIDATE_INSTALLER          true to include install.sh verify-only in post-release gate
  STACYVM_RUNTIME_CERTIFY             Comma-separated runtimes: docker,gvisor,kata,firecracker,proot

The script exits non-zero when required local checks fail. Skipped tag/host-gated
items are recorded as SKIP in the report instead of pretending the release is
fully ready.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) OUTPUT="$2"; shift 2 ;;
    --config) CONFIG_FILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ ! -f "$CONFIG_FILE" ]]; then
  printf 'config file not found: %s\n' "$CONFIG_FILE" >&2
  exit 2
fi

if [[ -z "${STACYVM_AUTH_API_KEY:-}" || -z "${STACYVM_AUTH_ADMIN_API_KEY:-}" ]]; then
  printf 'STACYVM_AUTH_API_KEY and STACYVM_AUTH_ADMIN_API_KEY are required for production config lint evidence\n' >&2
  exit 2
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

failures=0
skips=0
rows=()

now_utc="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
git_sha="$(git rev-parse HEAD 2>/dev/null || printf 'unknown')"
git_branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || printf 'unknown')"
git_status="$(git status --short 2>/dev/null || true)"

record_row() {
  local status="$1"
  local gate="$2"
  local detail="$3"
  rows+=("| $status | $gate | $detail |")
}

run_gate() {
  local gate="$1"
  shift
  local log="$tmpdir/$(printf '%s' "$gate" | tr -cs 'A-Za-z0-9' '-').log"
  printf '==> %s\n' "$gate"
  if "$@" >"$log" 2>&1; then
    record_row "PASS" "$gate" "See \`$log\` during this run."
  else
    failures=$((failures + 1))
    record_row "FAIL" "$gate" "Command failed; last 40 log lines are included below."
    {
      printf '\n### %s Failure Log\n\n' "$gate"
      printf '```text\n'
      tail -40 "$log"
      printf '\n```\n'
    } >>"$tmpdir/failures.md"
  fi
}

skip_gate() {
  local gate="$1"
  local detail="$2"
  skips=$((skips + 1))
  record_row "SKIP" "$gate" "$detail"
}

run_gate "Shell script syntax" bash -n \
  scripts/install.sh \
  scripts/verify-release.sh \
  scripts/post-release-validate.sh \
  scripts/public-readiness-evidence.sh \
  scripts/ci-public-release-sanity.sh \
  scripts/ci-upgrade-migration.sh \
  scripts/ci-cluster-conformance.sh \
  scripts/certify-runtime.sh \
  scripts/smoke-remote-worker.sh

run_gate "Production config lint" go run ./cmd/stacyvm config lint --production --file "$CONFIG_FILE"
run_gate "Go test suite" env -u STACYVM_AUTH_API_KEY -u STACYVM_AUTH_ADMIN_API_KEY go test ./...
run_gate "Web production build" npm --prefix web run build
run_gate "Public release sanity" scripts/ci-public-release-sanity.sh
run_gate "Upgrade and migration sanity" env -u STACYVM_AUTH_API_KEY -u STACYVM_AUTH_ADMIN_API_KEY scripts/ci-upgrade-migration.sh

if [[ "$RUN_CLUSTER" == "true" ]]; then
  run_gate "Cluster conformance" env -u STACYVM_AUTH_API_KEY -u STACYVM_AUTH_ADMIN_API_KEY scripts/ci-cluster-conformance.sh
else
  skip_gate "Cluster conformance" "Set \`STACYVM_RUN_CLUSTER_CONFORMANCE=true\` to capture this gate; it opens local listener ports."
fi

if [[ -n "$POST_RELEASE_VERSION" ]]; then
  run_gate "Post-release asset validation" scripts/post-release-validate.sh "$POST_RELEASE_VERSION"
else
  skip_gate "Post-release asset validation" "Set \`STACYVM_POST_RELEASE_VERSION=<tag>\` after publishing a real GitHub release."
fi

if [[ -n "$RUNTIMES" ]]; then
  IFS=',' read -r -a runtime_list <<<"$RUNTIMES"
  for runtime in "${runtime_list[@]}"; do
    runtime="$(printf '%s' "$runtime" | xargs)"
    [[ -z "$runtime" ]] && continue
    run_gate "Runtime certification: $runtime" scripts/certify-runtime.sh "$runtime" --format markdown --output "$tmpdir/$runtime-certification.md"
  done
else
  skip_gate "Runtime certification" "Set \`STACYVM_RUNTIME_CERTIFY=docker,gvisor,kata,firecracker,proot\` for the runtime claims in this launch."
fi

readiness="PUBLIC SELF-SERVE CANDIDATE"
if [[ "$failures" -gt 0 ]]; then
  readiness="NOT READY"
elif [[ "$skips" -eq 0 ]]; then
  readiness="PUBLIC SELF-SERVE READY"
fi

{
  printf '# StacyVM Public Readiness Evidence\n\n'
  printf '%s\n' "- Generated: \`$now_utc\`"
  printf '%s\n' "- Branch: \`$git_branch\`"
  printf '%s\n' "- Commit: \`$git_sha\`"
  printf '%s\n' "- Config: \`$CONFIG_FILE\`"
  printf '%s\n\n' "- Verdict: **$readiness**"

  printf '## Gate Results\n\n'
  printf '| Status | Gate | Detail |\n'
  printf '|---|---|---|\n'
  for row in "${rows[@]}"; do
    printf '%s\n' "$row"
  done

  printf '\n## Interpretation\n\n'
  if [[ "$failures" -gt 0 ]]; then
    printf 'This report is **not ready** for public announcement because one or more required gates failed.\n'
  elif [[ "$skips" -gt 0 ]]; then
    printf 'This report is a **public self-serve candidate**. All required local gates passed, but skipped tag/host-gated evidence must be captured before announcement.\n'
  else
    printf 'This report is **public self-serve ready** for the tested release, host, runtime, and network scope.\n'
  fi

  printf '\n## External Evidence Required Before Announcement\n\n'
  printf '%s\n' '- Real tagged release validated by `scripts/post-release-validate.sh <version>`.'
  printf '%s\n' '- Runtime certification reports for every runtime claimed publicly.'
  printf '%s\n' '- Live Postgres contract evidence for cluster/multi-worker claims.'
  printf '%s\n' '- Target-network worker RPC mTLS smoke with deployment-issued certificates for enterprise/multi-worker claims.'
  printf '%s\n' '- Staging install rehearsal from published artifacts, including `doctor --production`, smoke deployment, backup/restore, upgrade rehearsal, and support bundle redaction.'

  if [[ -n "$git_status" ]]; then
    printf '\n## Working Tree Note\n\n'
    printf 'The working tree had local changes when this report was generated:\n\n'
    printf '```text\n%s\n```\n' "$git_status"
  fi

  if [[ -f "$tmpdir/failures.md" ]]; then
    cat "$tmpdir/failures.md"
  fi
} >"$OUTPUT"

printf 'public readiness evidence written: %s\n' "$OUTPUT"

if [[ "$failures" -gt 0 ]]; then
  exit 1
fi
