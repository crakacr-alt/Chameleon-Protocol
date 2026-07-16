#!/usr/bin/env bash
set -euo pipefail

REPO_URL=https://github.com/crakacr-alt/Chameleon-Protocol.git
REPO_DIR=/opt/chameleon
SERVICE_DEST=/etc/systemd/system/chameleon.service

mkdir -p /opt
if [ ! -d "$REPO_DIR/.git" ]; then
  git clone "$REPO_URL" "$REPO_DIR"
else
  git -C "$REPO_DIR" pull origin main
fi

cd "$REPO_DIR"
GOFLAGS='' go build -o "$REPO_DIR/chameleon-server" ./cmd/server
install -m 0644 "$REPO_DIR/deploy/chameleon.service" "$SERVICE_DEST"
systemctl daemon-reload
systemctl enable --now chameleon
systemctl status chameleon --no-pager
