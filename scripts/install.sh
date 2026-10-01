#!/usr/bin/env bash
# Copyright (c) ScanDrix Authors. All rights reserved.
# Licensed under the Apache License, Version 2.0.

set -euo pipefail

# ═══════════════════════════════════════════════════════════════
# SCANDRIX CLI INSTALLER (macOS & Linux)
# ═══════════════════════════════════════════════════════════════

REPO="scandrix/scandrix"
INSTALL_DIR="${SCANDRIX_INSTALL_DIR:-$HOME/.scandrix/bin}"
BINARY_NAME="scandrix"

main() {
    echo -e "\033[1;36m==>\033[0m Installing ScanDrix CLI..."

    # Detect OS
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$OS" in
        linux*)  OS="linux" ;;
        darwin*) OS="darwin" ;;
        *)
            echo -e "\033[1;31mError:\033[0m Unsupported operating system: $OS" >&2
            exit 1
            ;;
    esac

    # Detect Architecture
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)
            echo -e "\033[1;31mError:\033[0m Unsupported architecture: $ARCH" >&2
            exit 1
            ;;
    esac

    # Determine Version
    VERSION="${SCANDRIX_VERSION:-latest}"
    if [ "$VERSION" = "latest" ]; then
        LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
        TAG="$(curl -fsSL "$LATEST_URL" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)"
        if [ -n "$TAG" ]; then
            VERSION="$TAG"
        else
            VERSION="v1.0.0"
        fi
    fi

    echo -e "    Target: \033[1m${OS}-${ARCH}\033[0m (${VERSION})"

    # Create destination directory
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$HOME/.scandrix/sessions"

    # The release workflow publishes raw binaries named scandrix-cli-<os>-<arch>
    # (see cli-release.yml matrix output_name) plus checksums.txt. This script
    # used to request scandrix-<os>-<arch>.tar.gz, which is never published, so
    # the download always failed and silently fell back to compiling locally -
    # installed users were never getting the release artifact.
    ASSET="scandrix-cli-${OS}-${ARCH}"
    BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
    DOWNLOAD_URL="${BASE_URL}/${ASSET}"
    CHECKSUM_URL="${BASE_URL}/checksums.txt"

    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    # Portable SHA-256. A downloaded binary is executed immediately after this,
    # so an unverified download is remote code execution with no gate.
    sha256_of() {
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum "$1" | awk '{print $1}'
        elif command -v shasum >/dev/null 2>&1; then
            shasum -a 256 "$1" | awk '{print $1}'
        else
            echo ""
        fi
    }

    echo -e "\033[1;34m==>\033[0m Downloading binary..."
    if curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$ASSET" 2>/dev/null; then

        # ── Verify before executing ──────────────────────────────────────────
        # checksums.txt is published by cli-release.yml alongside the binaries.
        if ! curl -fsSL "$CHECKSUM_URL" -o "$TMP_DIR/checksums.txt" 2>/dev/null; then
            echo -e "\033[1;31mError:\033[0m Could not fetch checksums.txt; refusing to run an unverified binary." >&2
            exit 1
        fi

        EXPECTED="$(awk -v f="$ASSET" '$2 == f || $2 == "*" f { print $1 }' "$TMP_DIR/checksums.txt" | head -1)"
        if [ -z "$EXPECTED" ]; then
            echo -e "\033[1;31mError:\033[0m No checksum entry for ${ASSET} in checksums.txt; refusing to continue." >&2
            exit 1
        fi

        ACTUAL="$(sha256_of "$TMP_DIR/$ASSET")"
        if [ -z "$ACTUAL" ]; then
            echo -e "\033[1;31mError:\033[0m No sha256 tool found (need sha256sum or shasum); refusing to continue." >&2
            exit 1
        fi
        if [ "$ACTUAL" != "$EXPECTED" ]; then
            echo -e "\033[1;31mError:\033[0m Checksum mismatch for ${ASSET}." >&2
            echo "  expected: $EXPECTED" >&2
            echo "  actual:   $ACTUAL" >&2
            echo "Refusing to install. Re-run once the release is fetched over a trusted channel." >&2
            exit 1
        fi
        echo -e "    \033[0;32mChecksum verified.\033[0m"

        mv "$TMP_DIR/$ASSET" "$INSTALL_DIR/$BINARY_NAME"
    else
        # If building from local repo checkout
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
        if [ -f "$SCRIPT_DIR/go.mod" ] && command -v go >/dev/null 2>&1; then
            echo -e "    Remote archive not reachable; compiling from local source..."
            (cd "$SCRIPT_DIR" && go build -o "$INSTALL_DIR/$BINARY_NAME" ./cmd/scandrix)
        else
            echo -e "\033[1;31mError:\033[0m Failed downloading release archive from: $DOWNLOAD_URL" >&2
            exit 1
        fi
    fi

    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    echo -e "\033[1;32m==>\033[0m ScanDrix installed successfully to: \033[1m$INSTALL_DIR/$BINARY_NAME\033[0m"

    # Shell PATH recommendation
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        echo ""
        echo -e "\033[1;33mAction Required:\033[0m Add ScanDrix to your PATH:"
        echo ""
        echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
        echo ""
        echo "Add the line above to your ~/.bashrc, ~/.zshrc, or profile."
    fi

    if [ -n "$TEAM_KEY" ]; then
        echo ""
        echo -e "\033[1;34m==>\033[0m Authenticating with team key..."
        # Passed via the environment, not --key: argv is world-readable through
        # ps, so a command-line secret leaks to every user on the host.
        if SCANDRIX_TEAM_KEY="$TEAM_KEY" "$INSTALL_DIR/$BINARY_NAME" auth team-key; then
            echo -e "\033[1;32m✓\033[0m Authenticated successfully"
        else
            echo -e "\033[1;31mError:\033[0m Authentication with team key failed" >&2
        fi
    fi

    echo ""
    echo -e "\033[1;34m==>\033[0m Installing bundled ScanDrix agent skills..."
    if "$INSTALL_DIR/$BINARY_NAME" skills install >/dev/null 2>&1; then
        echo -e "\033[1;32m✓\033[0m Agent skills installed"
    else
        echo -e "\033[1;33m!\033[0m Agent skills were not installed. The CLI is installed and usable;"
        echo "  re-run \`scandrix skills install\` once the cause is fixed."
    fi

    echo ""
    echo "Get started:"
    echo "  scandrix --help"
    echo "  scandrix auth login"
    echo "  scandrix review"
    echo ""
}

TEAM_KEY=""
while [ $# -gt 0 ]; do
    case "$1" in
        --team-key)
            TEAM_KEY="$2"
            shift 2
            ;;
        --team-key=*)
            TEAM_KEY="${1#*=}"
            shift
            ;;
        -h|--help)
            echo "Usage: install.sh [options]"
            echo ""
            echo "Options:"
            echo "  --team-key <key>  Authenticate with team key after installation"
            echo "  -h, --help        Show this help message"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

main "$@"
