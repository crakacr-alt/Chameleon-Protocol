#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/etc/chameleon/tunnel.env"

if [ ! -r "$ENV_FILE" ]; then
  echo "missing $ENV_FILE" >&2
  exit 1
fi

listen="$(sed -n 's/^CHAMELEON_LISTEN=//p' "$ENV_FILE" | tail -n1)"
port="${listen##*:}"

if ! [[ "$port" =~ ^[0-9]+$ ]]; then
  echo "invalid CHAMELEON_LISTEN in $ENV_FILE" >&2
  exit 1
fi

healthy() {
  curl -kfsS --connect-timeout 3 --max-time 5     "https://127.0.0.1:$port/" >/dev/null &&
    ss -H -lun "sport = :$port" 2>/dev/null | grep -q .
}

if healthy; then
  exit 0
fi

echo "Chameleon TLS/QUIC health check failed; restarting service" >&2
systemctl restart chameleon-tunnel.service
sleep 2

healthy
