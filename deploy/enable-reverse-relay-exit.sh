#!/usr/bin/env bash
set -euo pipefail

# Run on the existing Chameleon exit VPS after the ingress has been paired.
# It maintains an outgoing SSH connection. No inbound connection to the old
# blocked public IP is needed.

INGRESS_HOST="${1:-}"
SSH_PORT="${2:-22}"
RELAY_PORT="${3:-9443}"
LOCAL_CHAMELEON_PORT="${4:-9443}"

KEY="/etc/chameleon/reverse-relay/id_ed25519"
KNOWN_HOSTS="/etc/chameleon/reverse-relay/known_hosts"
SERVICE="/etc/systemd/system/chameleon-reverse-relay.service"
SOURCE_PROFILE="/etc/chameleon/client-profile.txt"
RELAY_PROFILE="/etc/chameleon/client-profile-relay.txt"

log() { printf '[chameleon-relay] %s\n' "$*" >&2; }
die() { printf '[chameleon-relay] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$INGRESS_HOST" ] || die "usage: sudo bash enable-reverse-relay-exit.sh INGRESS_IP [ssh-port] [relay-port] [local-chameleon-port]"
[ -s "$KEY" ] || die "pairing key missing; run prepare-reverse-relay-exit.sh first"
[ -s "$SOURCE_PROFILE" ] || die "source Chameleon profile missing: $SOURCE_PROFILE"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y --no-install-recommends openssh-client

install -d -o root -g root -m 0700 "$(dirname "$KEY")"

tmp_hosts="$(mktemp)"
trap 'rm -f "$tmp_hosts"' EXIT
ssh-keyscan -T 8 -p "$SSH_PORT" -t ed25519 "$INGRESS_HOST" > "$tmp_hosts" 2>/dev/null
[ -s "$tmp_hosts" ] || die "could not read ingress SSH host key"
install -o root -g root -m 0600 "$tmp_hosts" "$KNOWN_HOSTS"

cat > "$SERVICE" <<EOF
[Unit]
Description=Chameleon persistent reverse ingress link
After=network-online.target chameleon-tunnel.service
Wants=network-online.target
Requires=chameleon-tunnel.service

[Service]
Type=simple
ExecStart=/usr/bin/ssh -NT \
  -i $KEY \
  -o IdentitiesOnly=yes \
  -o UserKnownHostsFile=$KNOWN_HOSTS \
  -o StrictHostKeyChecking=yes \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=20 \
  -o ServerAliveCountMax=3 \
  -p $SSH_PORT \
  -R 0.0.0.0:$RELAY_PORT:127.0.0.1:$LOCAL_CHAMELEON_PORT \
  chameleon-relay@$INGRESS_HOST
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# Build a TCP/TLS-only client profile for the relay. We deliberately leave
# CHAMELEON_SERVER and QUIC empty because an SSH reverse forward transports TCP,
# not UDP/QUIC.
psk="$(awk -F= '$1=="CHAMELEON_TUNNEL_PSK"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_PROFILE")"
fingerprint="$(awk -F= '$1=="CHAMELEON_TLS_FINGERPRINT"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_PROFILE")"
[ -n "$psk" ] || die "PSK missing from source profile"
[ -n "$fingerprint" ] || die "TLS fingerprint missing from source profile"

umask 077
cat > "$RELAY_PROFILE" <<EOF
CHAMELEON_SERVER=
CHAMELEON_QUIC_SERVER=
CHAMELEON_TLS_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_TUNNEL_PSK=$psk
CHAMELEON_TLS_FINGERPRINT=$fingerprint
EOF
chmod 0600 "$RELAY_PROFILE"

systemctl daemon-reload
systemctl enable --now chameleon-reverse-relay.service

sleep 2
if ! systemctl is-active --quiet chameleon-reverse-relay.service; then
  systemctl status chameleon-reverse-relay.service --no-pager || true
  journalctl -u chameleon-reverse-relay.service -n 80 --no-pager || true
  die "reverse relay did not start"
fi

log "reverse relay active"
printf 'Client endpoint: %s:%s (TLS/TCP only)\n' "$INGRESS_HOST" "$RELAY_PORT"
printf 'Relay profile: %s\n' "$RELAY_PROFILE"
printf 'The original exit IP is not used by clients on this path.\n'
