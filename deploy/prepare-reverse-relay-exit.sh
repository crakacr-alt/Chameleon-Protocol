#!/usr/bin/env bash
set -euo pipefail

# Run on the existing Chameleon exit VPS.
# Creates a dedicated SSH key used only to maintain the reverse ingress link.

KEY_DIR="/etc/chameleon/reverse-relay"
KEY="${KEY_DIR}/id_ed25519"

die() { printf '[chameleon-relay] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
command -v ssh-keygen >/dev/null 2>&1 || {
  apt-get update -y
  apt-get install -y --no-install-recommends openssh-client
}

install -d -o root -g root -m 0700 "$KEY_DIR"

if [ ! -s "$KEY" ]; then
  ssh-keygen -q -t ed25519 -N "" -C "chameleon-reverse-exit" -f "$KEY"
fi

chmod 0600 "$KEY"
chmod 0644 "${KEY}.pub"

printf '\nPublic pairing key — this is safe to copy to the ingress VPS:\n\n'
cat "${KEY}.pub"
printf '\nNext: run install-reverse-relay-ingress.sh on the new VPS with this public key.\n'
