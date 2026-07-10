#!/usr/bin/env bash
# Nimbus Sync — Linux installer
# Builds (or downloads) the binary, installs it to /usr/local/bin,
# seeds the user config, and optionally enables a systemd user timer.
set -euo pipefail

APP=nimbus
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="${PREFIX:-/usr/local}"
BIN_DIR="$PREFIX/bin"
CONF_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/nimbus-sync"
UNIT_DIR="$HOME/.config/systemd/user"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }

bold "Nimbus Sync installer"

# 1. Build ------------------------------------------------------------------
if ! command -v go >/dev/null 2>&1; then
  echo "Go toolchain not found. Install Go 1.22+ first:"
  echo "  https://go.dev/doc/install   (or: sudo apt install golang-go)"
  exit 1
fi

bold "→ Building $APP ..."
cd "$REPO_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "/tmp/$APP" ./cmd/nimbus

# 2. Install binary ---------------------------------------------------------
bold "→ Installing to $BIN_DIR/$APP"
if [ -w "$BIN_DIR" ]; then
  install -m 0755 "/tmp/$APP" "$BIN_DIR/$APP"
else
  sudo install -m 0755 "/tmp/$APP" "$BIN_DIR/$APP"
fi

# 3. Seed configuration ------------------------------------------------------
mkdir -p "$CONF_DIR"
if [ ! -f "$CONF_DIR/config.yaml" ]; then
  cp "$REPO_DIR/configs/config.example.yaml" "$CONF_DIR/config.yaml"
  chmod 600 "$CONF_DIR/config.yaml"
  bold "→ Created $CONF_DIR/config.yaml — edit it with your credentials and remotes."
else
  echo "→ Existing config kept at $CONF_DIR/config.yaml"
fi

# 4. Optional systemd user timer ---------------------------------------------
if command -v systemctl >/dev/null 2>&1; then
  read -r -p "Install systemd user service + timer for scheduled syncs? [y/N] " ans
  if [[ "${ans:-n}" =~ ^[Yy]$ ]]; then
    mkdir -p "$UNIT_DIR"
    cp "$REPO_DIR/systemd/nimbus-sync.service" "$UNIT_DIR/"
    cp "$REPO_DIR/systemd/nimbus-sync.timer" "$UNIT_DIR/"
    systemctl --user daemon-reload
    systemctl --user enable --now nimbus-sync.timer
    bold "→ Timer enabled. Check with: systemctl --user list-timers nimbus-sync.timer"
  fi
fi

bold "Done!"
echo
echo "Next steps:"
echo "  1. Edit $CONF_DIR/config.yaml"
echo "  2. Authorize providers:  nimbus auth gdrive | onedrive | dropbox"
echo "  3. First sync:           nimbus sync --dry-run"
