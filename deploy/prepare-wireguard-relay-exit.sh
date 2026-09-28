#!/usr/bin/env bash
set -euo pipefail

# Run on the existing Chameleon exit VPS.
# Generates the WireGuard identity used only for the private relay link.

KEY_DIR="/etc/chameleon/wg-relay"
PRIVATE_KEY="${KEY_DIR}/private.key"
PUBLIC_KEY="${KEY_DIR}/public.key"

die() { printf '[chameleon-wg] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"

if ! command -v wg >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y --no-install-recommends wireguard-tools
fi

install -d -o root -g root -m 0700 "$KEY_DIR"

if [ ! -s "$PRIVATE_KEY" ]; then
  umask 077
  wg genkey > "$PRIVATE_KEY"
  wg pubkey < "$PRIVATE_KEY" > "$PUBLIC_KEY"
fi

chmod 0600 "$PRIVATE_KEY"
chmod 0644 "$PUBLIC_KEY"

printf '\nServer 1 WireGuard public pairing key:\n\n'
cat "$PUBLIC_KEY"
printf '\nCopy only this public key to Server 2. The private key never leaves Server 1.\n'
