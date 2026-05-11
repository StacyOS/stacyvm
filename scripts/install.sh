#!/bin/bash
# StacyVM Installer — download pre-built binaries and get running in one command
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/StacyOs/stacyvm/main/scripts/install.sh | bash
#
# Environment variables:
#   STACYVM_VERSION       — specific version to install (default: latest)
#   STACYVM_INSTALL_DIR   — where to put binaries (default: /usr/local/bin)
#   STACYVM_REQUIRE_SIGNATURES — require Sigstore verification with cosign (default: false)
#   STACYVM_VERIFY_ONLY   — download and verify release assets, then exit before installing
#
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
fail()  { echo -e "${RED}[x]${NC} $1"; exit 1; }

INSTALL_DIR="${STACYVM_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="/var/lib/stacyvm"
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
REPO="StacyOs/stacyvm"
COSIGN_IDENTITY_REGEXP="${STACYVM_COSIGN_IDENTITY_REGEXP:-https://github.com/StacyOS/stacyvm/.github/workflows/release.yml@refs/tags/v.*}"
COSIGN_ISSUER="${STACYVM_COSIGN_ISSUER:-https://token.actions.githubusercontent.com}"

echo ""
echo "  ╔═══════════════════════════════════╗"
echo "  ║       StacyVM Installer           ║"
echo "  ╚═══════════════════════════════════╝"
echo ""

# ── Check OS + arch ──────────────────────────────────────
if [[ "$(uname -s)" != "Linux" ]]; then
    fail "StacyVM requires Linux. You're on $(uname -s)."
fi

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH_SUFFIX="amd64" ;;
    aarch64|arm64) ARCH_SUFFIX="arm64" ;;
    *) fail "StacyVM supports x86_64 and aarch64. You're on $ARCH." ;;
esac
info "Detected architecture: $ARCH ($ARCH_SUFFIX)"

# ── Detect version ───────────────────────────────────────
if [[ -n "${STACYVM_VERSION:-}" ]]; then
    VERSION="$STACYVM_VERSION"
    info "Using specified version: $VERSION"
else
    info "Detecting latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$VERSION" ]]; then
        fail "Could not detect latest version. Set STACYVM_VERSION manually."
    fi
    info "Latest version: $VERSION"
fi

RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

# ── Download binaries ────────────────────────────────────
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading stacyvm ${VERSION} (${ARCH_SUFFIX})..."
curl -fSL -o "$TMPDIR/stacyvm" "${RELEASE_URL}/stacyvm-linux-${ARCH_SUFFIX}"
curl -fSL -o "$TMPDIR/stacyvm-agent" "${RELEASE_URL}/stacyvm-agent-linux-${ARCH_SUFFIX}"
curl -fSL -o "$TMPDIR/checksums.txt" "${RELEASE_URL}/checksums.txt"

# ── Verify Sigstore signatures when available ───────────
verify_signature() {
    local file="$1"
    local release_name="$2"
    if ! curl -fSL -o "${file}.sig" "${RELEASE_URL}/${release_name}.sig"; then
        return 2
    fi
    if ! curl -fSL -o "${file}.pem" "${RELEASE_URL}/${release_name}.pem"; then
        return 2
    fi
    cosign verify-blob "$file" \
        --signature "${file}.sig" \
        --certificate "${file}.pem" \
        --certificate-identity-regexp "$COSIGN_IDENTITY_REGEXP" \
        --certificate-oidc-issuer "$COSIGN_ISSUER" >/dev/null
}

if command -v cosign >/dev/null 2>&1; then
    info "Verifying Sigstore signatures..."
    signatures_missing=false
    for item in \
        "$TMPDIR/stacyvm:stacyvm-linux-${ARCH_SUFFIX}" \
        "$TMPDIR/stacyvm-agent:stacyvm-agent-linux-${ARCH_SUFFIX}" \
        "$TMPDIR/checksums.txt:checksums.txt"
    do
        file="${item%%:*}"
        release_name="${item#*:}"
        set +e
        verify_signature "$file" "$release_name"
        verify_status=$?
        set -e
        if [[ "$verify_status" -eq 2 ]]; then
            signatures_missing=true
        elif [[ "$verify_status" -ne 0 ]]; then
            fail "Sigstore signature verification failed for ${release_name}"
        fi
    done
    if [[ "$signatures_missing" == "true" ]]; then
        if [[ "${STACYVM_REQUIRE_SIGNATURES:-false}" == "true" ]]; then
            fail "Sigstore signature assets are missing for this release."
        fi
        warn "Sigstore signature assets are missing for this release; relying on checksums only."
    else
        info "Signatures verified"
    fi
elif [[ "${STACYVM_REQUIRE_SIGNATURES:-false}" == "true" ]]; then
    fail "cosign is required because STACYVM_REQUIRE_SIGNATURES=true. Install cosign and rerun."
else
    warn "cosign not found; skipping Sigstore signature verification and relying on checksums only."
    warn "For public installs, install cosign or set STACYVM_REQUIRE_SIGNATURES=true."
fi

# ── Verify checksums ────────────────────────────────────
info "Verifying checksums..."
cd "$TMPDIR"
# checksums.txt has names like stacyvm-linux-amd64, rename for verification
sha256sum -c checksums.txt --ignore-missing 2>/dev/null || {
    # Try manual verification with renamed files
    EXPECTED_VM=$(grep "stacyvm-linux-${ARCH_SUFFIX}" checksums.txt | grep -v agent | awk '{print $1}')
    EXPECTED_AGENT=$(grep "stacyvm-agent-linux-${ARCH_SUFFIX}" checksums.txt | awk '{print $1}')
    ACTUAL_VM=$(sha256sum stacyvm | awk '{print $1}')
    ACTUAL_AGENT=$(sha256sum stacyvm-agent | awk '{print $1}')

    if [[ "$EXPECTED_VM" != "$ACTUAL_VM" ]]; then
        fail "Checksum mismatch for stacyvm binary!"
    fi
    if [[ "$EXPECTED_AGENT" != "$ACTUAL_AGENT" ]]; then
        fail "Checksum mismatch for stacyvm-agent binary!"
    fi
}
info "Checksums verified"
cd - > /dev/null

if [[ "${STACYVM_VERIFY_ONLY:-false}" == "true" ]]; then
    info "Verify-only mode complete; skipping install and host setup."
    exit 0
fi

# ── Install binaries ─────────────────────────────────────
info "Installing to ${INSTALL_DIR}..."
chmod +x "$TMPDIR/stacyvm" "$TMPDIR/stacyvm-agent"

if [[ -w "$INSTALL_DIR" ]]; then
    cp "$TMPDIR/stacyvm" "$INSTALL_DIR/stacyvm"
    cp "$TMPDIR/stacyvm-agent" "$INSTALL_DIR/stacyvm-agent"
else
    sudo cp "$TMPDIR/stacyvm" "$INSTALL_DIR/stacyvm"
    sudo cp "$TMPDIR/stacyvm-agent" "$INSTALL_DIR/stacyvm-agent"
fi
info "Installed stacyvm and stacyvm-agent to ${INSTALL_DIR}"

# ── Install Firecracker if missing ───────────────────────
if command -v firecracker &>/dev/null; then
    FC_VER=$(firecracker --version 2>&1 | head -1)
    info "Firecracker already installed: $FC_VER"
elif [[ "$ARCH_SUFFIX" == "arm64" ]]; then
    warn "Firecracker auto-install not supported on ARM64."
    warn "On ARM64, consider using the PRoot or Docker provider instead."
    warn "Set STACYVM_PROVIDERS_DEFAULT=proot or STACYVM_PROVIDERS_DEFAULT=docker"
else
    warn "Firecracker not found. Installing latest release..."
    FC_VERSION=$(curl -fsSL https://api.github.com/repos/firecracker-microvm/firecracker/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
    if [[ -z "$FC_VERSION" ]]; then
        warn "Could not fetch Firecracker version. Install manually: https://github.com/firecracker-microvm/firecracker/releases"
    else
        FC_ARCH="x86_64"
        FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VERSION}/firecracker-${FC_VERSION}-${FC_ARCH}.tgz"
        FC_TMP=$(mktemp -d)
        curl -fSL -o "$FC_TMP/firecracker.tgz" "$FC_URL"
        tar -xzf "$FC_TMP/firecracker.tgz" -C "$FC_TMP"
        FC_BIN=$(find "$FC_TMP" -name "firecracker-${FC_VERSION}-${FC_ARCH}" -type f | head -1)
        if [[ -n "$FC_BIN" ]]; then
            sudo cp "$FC_BIN" /usr/local/bin/firecracker
            sudo chmod +x /usr/local/bin/firecracker
            info "Firecracker ${FC_VERSION} installed"
        else
            warn "Could not find firecracker binary in release archive"
        fi
        rm -rf "$FC_TMP"
    fi
fi

# ── Data directory + kernel ──────────────────────────────
if [[ ! -d "$DATA_DIR" ]]; then
    info "Creating ${DATA_DIR}..."
    sudo mkdir -p "$DATA_DIR"
    sudo chown "$(whoami)" "$DATA_DIR"
fi

if [[ "$ARCH_SUFFIX" == "arm64" ]]; then
    info "Skipping kernel download on ARM64 (Firecracker kernel not needed for PRoot/Docker)"
elif [[ -f "$DATA_DIR/vmlinux.bin" ]]; then
    info "Kernel already exists at $DATA_DIR/vmlinux.bin"
else
    info "Downloading Firecracker kernel..."
    curl -fSL -o "$DATA_DIR/vmlinux.bin" "$KERNEL_URL" || sudo curl -fSL -o "$DATA_DIR/vmlinux.bin" "$KERNEL_URL"
    chmod 644 "$DATA_DIR/vmlinux.bin" 2>/dev/null || sudo chmod 644 "$DATA_DIR/vmlinux.bin"
    info "Kernel downloaded"
fi

# ── Check KVM (warn only) ───────────────────────────────
if [[ -e /dev/kvm ]]; then
    if [[ -r /dev/kvm ]] && [[ -w /dev/kvm ]]; then
        info "KVM is accessible"
    else
        warn "KVM exists but no permission. Fix with: sudo chmod 666 /dev/kvm"
        warn "StacyVM still works in mock mode without KVM."
    fi
else
    warn "KVM not available. StacyVM will work in mock mode (no hardware VM isolation)."
    warn "For microVM support, enable virtualization in BIOS."
fi

# ── Check Docker (advisory) ─────────────────────────────
if command -v docker &>/dev/null; then
    info "Docker found (needed for custom environment images)"
else
    warn "Docker not found. Optional — needed only for custom Docker images."
    warn "Install: https://docs.docker.com/engine/install/"
fi

# ── Done ─────────────────────────────────────────────────
echo ""
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo -e "${GREEN}  StacyVM ${VERSION} installed!${NC}"
echo -e "${GREEN}══════════════════════════════════════${NC}"
echo ""
echo "  Start the server:"
echo -e "    ${BOLD}stacyvm serve${NC}"
echo ""
echo "  Test it:"
echo "    curl -s -X POST localhost:7423/api/v1/sandboxes \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -d '{\"image\":\"alpine:latest\"}' | jq .id"
echo ""
echo "  For XFS reflink support (faster snapshots), run:"
echo "    ./scripts/setup.sh from the source repo"
echo ""
