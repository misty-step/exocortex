#!/bin/sh
set -eu
# Requires ~/.local/bin/exocortex built from THIS checkout (go install
# ./cmd/exocortex). Older SHAs including 88675d8 and ad5f242 are rejected.

USER_SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$USER_SYSTEMD_DIR"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_DIR=$(dirname "$SCRIPT_DIR")

cp "$REPO_DIR/systemd/exocortex-sync.service" "$USER_SYSTEMD_DIR/exocortex-sync.service"
cp "$REPO_DIR/systemd/exocortex-sync.timer" "$USER_SYSTEMD_DIR/exocortex-sync.timer"

systemctl --user daemon-reload
systemctl --user enable --now exocortex-sync.timer

echo "Exocortex background sync timer installed and active (5min interval)."
