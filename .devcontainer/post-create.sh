#!/usr/bin/env bash
set -euo pipefail

WORKSPACE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_DIR="$(dirname "$WORKSPACE_DIR")"

echo "==> [post-create] Workspace: $WORKSPACE_DIR"

echo "==> [post-create] Installing dependencies..."
yay -Syu --noconfirm
yay -S go mage wails libayatana-appindicator --noconfirm

# ---- Claude Code ----
echo "==> [claude] Installing Claude Code..."
curl -fsSL https://claude.ai/install.sh | bash

echo "==> [claude] Setting up data directory..."
mkdir -p "$WORKSPACE_DIR/.devcontainer/do-not-commit"
ln -sfn "$WORKSPACE_DIR/.devcontainer/do-not-commit" "$HOME/.claude"

echo "==> [claude] Setting up PATH and environment variables..."
echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"
echo 'export CLAUDE_DATA_DIR="$HOME/.claude"' >> "$HOME/.bashrc"

# ---- Done ----
echo "==> [post-create] Done."
