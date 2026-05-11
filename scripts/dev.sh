#!/usr/bin/env bash
# Local StacyVM developer bootstrap.
#
# Usage:
#   ./scripts/dev.sh
#   make dev
#
# This script intentionally does not install Docker Desktop or system packages.
# It checks the local host, prints OS-specific remediation, builds StacyVM, and
# starts the API server.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${STACYVM_SERVER_PORT:-7423}"

red() { printf '\033[0;31m%s\033[0m\n' "$*"; }
green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[1;33m%s\033[0m\n' "$*"; }
step() { printf '\n\033[1m%s\033[0m\n' "$*"; }

fail() {
  red "x $*"
  exit 1
}

host_name() {
  case "$(uname -s)" in
    Darwin) echo "macOS" ;;
    Linux)
      if grep -qi microsoft /proc/version 2>/dev/null; then
        echo "Windows WSL"
      else
        echo "Linux"
      fi
      ;;
    *) uname -s ;;
  esac
}

print_go_help() {
  case "$(host_name)" in
    macOS)
      cat <<'EOF'
Install Go with:
  brew install go
EOF
      ;;
    "Windows WSL"|Linux)
      cat <<'EOF'
Install Go with:
  sudo apt update
  sudo apt install -y golang-go
EOF
      ;;
    *)
      echo "Install Go from https://go.dev/dl/"
      ;;
  esac
}

print_docker_help() {
  case "$(host_name)" in
    macOS)
      cat <<'EOF'
Install and start Docker Desktop:
  https://docs.docker.com/desktop/setup/install/mac-install/

Then verify:
  docker run --rm hello-world
EOF
      ;;
    "Windows WSL")
      cat <<'EOF'
Install Docker Desktop, enable WSL integration for your Ubuntu distro, then run:
  docker run --rm hello-world
EOF
      ;;
    Linux)
      cat <<'EOF'
Install Docker, start it, and add your user to the docker group:
  sudo apt update
  sudo apt install -y docker.io
  sudo systemctl enable --now docker
  sudo usermod -aG docker "$USER"

Log out and back in, then verify:
  docker run --rm hello-world
EOF
      ;;
    *)
      echo "Install Docker from https://docs.docker.com/get-docker/"
      ;;
  esac
}

check_command() {
  local name="$1"
  local remediation="$2"
  if ! command -v "$name" >/dev/null 2>&1; then
    yellow "! Missing ${name}."
    eval "$remediation"
    exit 1
  fi
}

step "StacyVM local setup"
echo "Host: $(host_name)"
echo "Repo: ${ROOT_DIR}"

step "Checking tools"
check_command go print_go_help
green "+ Go: $(go version)"

check_command docker print_docker_help
green "+ Docker CLI: $(docker --version)"

if ! docker info >/dev/null 2>&1; then
  yellow "! Docker CLI is installed, but the Docker daemon is not reachable."
  print_docker_help
  exit 1
fi
green "+ Docker daemon is reachable"

if command -v lsof >/dev/null 2>&1 && lsof -iTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  yellow "! Port ${PORT} is already in use."
  echo "Stop the process using port ${PORT}, or run StacyVM with a different server port in your config."
  exit 1
fi
green "+ Port ${PORT} is available"

step "Building StacyVM"
cd "$ROOT_DIR"
make build

step "Starting StacyVM"
echo "API: http://localhost:${PORT}"
echo "Health check: curl http://localhost:${PORT}/api/v1/live"
echo ""
exec ./stacyvm serve
