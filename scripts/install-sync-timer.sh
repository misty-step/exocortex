#!/bin/sh
set -eu
# Policy: run this only after `go install` from this checkout. The script
# does not verify binary identity; an old ~/.local/bin/exocortex still runs.

USER_SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$USER_SYSTEMD_DIR"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_DIR=$(dirname "$SCRIPT_DIR")

cp "$REPO_DIR/systemd/exocortex-sync.service" "$USER_SYSTEMD_DIR/exocortex-sync.service"
cp "$REPO_DIR/systemd/exocortex-sync.timer" "$USER_SYSTEMD_DIR/exocortex-sync.timer"

systemctl --user daemon-reload
systemctl --user enable --now exocortex-sync.timer

echo "Exocortex background sync timer installed and active (5min interval)."
