#!/usr/bin/env bash
set -euo pipefail

# Run on the new reachable VPS.
# The node accepts only one restricted reverse TCP forward from the old exit.

RELAY_USER="chameleon-relay"
RELAY_PORT="${CHAMELEON_RELAY_PORT:-9443}"
PUBLIC_KEY="${CHAMELEON_RELAY_PUBLIC_KEY:-}"

log() { printf '[chameleon-relay] %s\n' "$*" >&2; }
die() { printf '[chameleon-relay] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"

if [ "$#" -ge 1 ]; then
  PUBLIC_KEY="$1"
fi
if [ "$#" -ge 2 ]; then
  RELAY_PORT="$2"
fi

[ -n "$PUBLIC_KEY" ] || die "usage: sudo bash install-reverse-relay-ingress.sh 'ssh-ed25519 AAAA...' [relay-port]"
case "$RELAY_PORT" in
  ''|*[!0-9]*) die "relay port must be numeric" ;;
esac
[ "$RELAY_PORT" -ge 1 ] && [ "$RELAY_PORT" -le 65535 ] || die "invalid relay port"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y --no-install-recommends openssh-server ca-certificates

if ! id "$RELAY_USER" >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash "$RELAY_USER"
fi

HOME_DIR="$(getent passwd "$RELAY_USER" | cut -d: -f6)"
SSH_DIR="${HOME_DIR}/.ssh"
AUTH_KEYS="${SSH_DIR}/authorized_keys"

install -d -o "$RELAY_USER" -g "$RELAY_USER" -m 0700 "$SSH_DIR"

# restrict disables shell/PTY/agent/X11; port-forwarding re-enables only the
# forwarding capability. permitlisten limits the remotely exposed listener.
printf 'restrict,port-forwarding,permitlisten="0.0.0.0:%s" %s\n'   "$RELAY_PORT" "$PUBLIC_KEY" > "$AUTH_KEYS"
chown "$RELAY_USER:$RELAY_USER" "$AUTH_KEYS"
chmod 0600 "$AUTH_KEYS"

DROPIN="/etc/ssh/sshd_config.d/90-chameleon-reverse-relay.conf"
cat > "$DROPIN" <<'EOF'
# Chameleon reverse ingress needs sshd to honor the address requested by -R.
# The authorized key itself limits which listener may be opened.
GatewayPorts clientspecified
EOF

sshd -t
systemctl restart ssh

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then
  ufw allow "${RELAY_PORT}/tcp" comment 'Chameleon reverse relay' >/dev/null
fi

log "reverse ingress ready"
printf 'Relay listener (after exit connects): 0.0.0.0:%s\n' "$RELAY_PORT"
printf 'SSH user: %s\n' "$RELAY_USER"
printf 'SSH host fingerprint — keep this for verification:\n'
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
