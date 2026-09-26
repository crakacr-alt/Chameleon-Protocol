#!/usr/bin/env bash
set -euo pipefail

REPO_URL="https://github.com/crakacr-alt/Chameleon-Protocol.git"
REPO_DIR="/opt/chameleon"
SERVICE_DEST="/etc/systemd/system/chameleon-tunnel.service"
ENV_DIR="/etc/chameleon"
ENV_FILE="$ENV_DIR/tunnel.env"

if [ -z "${CHAMELEON_TUNNEL_PSK:-}" ]; then
  echo "CHAMELEON_TUNNEL_PSK is required." >&2
  echo "Example: export CHAMELEON_TUNNEL_PSK=\$(openssl rand -hex 32)" >&2
  exit 2
fi

mkdir -p /opt
if [ ! -d "$REPO_DIR/.git" ]; then
  git clone "$REPO_URL" "$REPO_DIR"
else
  git -C "$REPO_DIR" pull --ff-only origin main
fi

cd "$REPO_DIR"
GOFLAGS='' go build -o "$REPO_DIR/chameleon-tunnel-server" ./cmd/tunnel-server

install -d -m 0700 "$ENV_DIR"
umask 077
printf 'CHAMELEON_TUNNEL_PSK=%s\n' "$CHAMELEON_TUNNEL_PSK" > "$ENV_FILE"
if [ -n "${CHAMELEON_TLS_CERT:-}" ] || [ -n "${CHAMELEON_TLS_KEY:-}" ]; then
  if [ -z "${CHAMELEON_TLS_CERT:-}" ] || [ -z "${CHAMELEON_TLS_KEY:-}" ]; then
    echo "Both CHAMELEON_TLS_CERT and CHAMELEON_TLS_KEY are required for TLS mode." >&2
    exit 2
  fi
  printf 'CHAMELEON_TLS_CERT=%s\n' "$CHAMELEON_TLS_CERT" >> "$ENV_FILE"
  printf 'CHAMELEON_TLS_KEY=%s\n' "$CHAMELEON_TLS_KEY" >> "$ENV_FILE"
fi
chmod 0600 "$ENV_FILE"

install -m 0644 "$REPO_DIR/deploy/chameleon-tunnel.service" "$SERVICE_DEST"
systemctl daemon-reload
systemctl enable --now chameleon-tunnel
systemctl status chameleon-tunnel --no-pager
