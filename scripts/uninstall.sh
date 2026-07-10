#!/usr/bin/env bash
# Nimbus Sync — Linux uninstaller
set -euo pipefail
BIN="/usr/local/bin/nimbus"
UNIT_DIR="$HOME/.config/systemd/user"

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user disable --now nimbus-sync.timer 2>/dev/null || true
  rm -f "$UNIT_DIR/nimbus-sync.service" "$UNIT_DIR/nimbus-sync.timer"
  systemctl --user daemon-reload || true
fi

if [ -f "$BIN" ]; then
  if [ -w "$(dirname "$BIN")" ]; then rm -f "$BIN"; else sudo rm -f "$BIN"; fi
  echo "Removed $BIN"
fi

echo "Config, tokens and reports kept at ~/.config/nimbus-sync — delete manually if desired:"
echo "  rm -rf ~/.config/nimbus-sync"
