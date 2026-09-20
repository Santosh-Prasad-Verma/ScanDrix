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

    TARBALL="scandrix-${OS}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    echo -e "\033[1;34m==>\033[0m Downloading binary..."
    if curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$TARBALL" 2>/dev/null; then
        tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"
        mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
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
        if "$INSTALL_DIR/$BINARY_NAME" auth team-key --key "$TEAM_KEY"; then
            echo -e "\033[1;32m✓\033[0m Authenticated successfully"
        else
            echo -e "\033[1;31mError:\033[0m Authentication with team key failed" >&2
        fi
    fi

    echo ""
    echo -e "\033[1;34m==>\033[0m Installing bundled ScanDrix agent skills..."
    "$INSTALL_DIR/$BINARY_NAME" skills install >/dev/null 2>&1 || true
    echo -e "\033[1;32m✓\033[0m Agent skills installed"

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
