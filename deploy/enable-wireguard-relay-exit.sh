#!/usr/bin/env bash
set -euo pipefail

# Run on the existing Chameleon exit VPS after Server 2 is prepared.

INGRESS_HOST="${1:-}"
INGRESS_PUBLIC_KEY="${2:-}"
WG_PORT="${3:-51820}"
RELAY_PORT="${4:-9443}"
LOCAL_CHAMELEON_PORT="${5:-9443}"

WG_IF="wgcham0"
WG_ADDR="10.77.0.2/24"
INGRESS_WG_IP="10.77.0.1"
KEY_DIR="/etc/chameleon/wg-relay"
PRIVATE_KEY="${KEY_DIR}/private.key"
WG_CONFIG="/etc/wireguard/${WG_IF}.conf"
SOURCE_PROFILE="/etc/chameleon/client-profile.txt"
RELAY_PROFILE="/etc/chameleon/client-profile-relay.txt"

log() { printf '[chameleon-wg] %s\n' "$*" >&2; }
die() { printf '[chameleon-wg] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$INGRESS_HOST" ] || die "usage: sudo bash enable-wireguard-relay-exit.sh INGRESS_IP INGRESS_WG_PUBLIC_KEY [wg-port] [relay-port] [local-chameleon-port]"
[ -n "$INGRESS_PUBLIC_KEY" ] || die "Server 2 WireGuard public key is required"
[ -s "$PRIVATE_KEY" ] || die "Server 1 pairing key missing; run prepare-wireguard-relay-exit.sh first"
[ -s "$SOURCE_PROFILE" ] || die "source profile missing: $SOURCE_PROFILE"

for value in "$WG_PORT" "$RELAY_PORT" "$LOCAL_CHAMELEON_PORT"; do
  case "$value" in
    ''|*[!0-9]*) die "ports must be numeric" ;;
  esac
done

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y --no-install-recommends wireguard-tools ca-certificates

install -d -o root -g root -m 0700 /etc/wireguard

# Verify the local Chameleon TLS listener before wiring a public relay to it.
if ! curl -kfsS --connect-timeout 4 --max-time 6     "https://127.0.0.1:$LOCAL_CHAMELEON_PORT/" >/dev/null; then
  die "local Chameleon TLS listener is not healthy on 127.0.0.1:$LOCAL_CHAMELEON_PORT"
fi

private="$(cat "$PRIVATE_KEY")"

cat > "$WG_CONFIG" <<EOF
[Interface]
Address = $WG_ADDR
PrivateKey = $private

[Peer]
PublicKey = $INGRESS_PUBLIC_KEY
Endpoint = $INGRESS_HOST:$WG_PORT
AllowedIPs = $INGRESS_WG_IP/32
PersistentKeepalive = 25
EOF

chmod 0600 "$WG_CONFIG"

systemctl enable --now "wg-quick@${WG_IF}.service"
systemctl restart "wg-quick@${WG_IF}.service"

sleep 3

latest_handshake="$(wg show "$WG_IF" latest-handshakes | awk 'NR==1 {print $2}')"
if [ -z "$latest_handshake" ] || [ "$latest_handshake" = "0" ]; then
  wg show "$WG_IF" >&2 || true
  die "WireGuard handshake with Server 2 has not completed"
fi

psk="$(awk -F= '$1=="CHAMELEON_TUNNEL_PSK"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_PROFILE")"
fingerprint="$(awk -F= '$1=="CHAMELEON_TLS_FINGERPRINT"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_PROFILE")"
server_name="$(awk -F= '$1=="CHAMELEON_TLS_SERVER_NAME"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_PROFILE")"
[ -n "$psk" ] || die "PSK missing from source profile"
[ -n "$fingerprint" ] || die "TLS fingerprint missing from source profile"

umask 077
cat > "$RELAY_PROFILE" <<EOF
CHAMELEON_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_QUIC_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_TLS_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_TUNNEL_PSK=$psk
CHAMELEON_TLS_FINGERPRINT=$fingerprint
${server_name:+CHAMELEON_TLS_SERVER_NAME=$server_name}
EOF
chmod 0600 "$RELAY_PROFILE"

log "Server 1 is paired with Server 2"
printf 'Private relay link: %s <-> %s\n' "$INGRESS_WG_IP" "10.77.0.2"
printf 'Client ingress: %s:%s TCP+UDP\n' "$INGRESS_HOST" "$RELAY_PORT"
printf 'Relay client profile: %s\n' "$RELAY_PROFILE"
printf 'Expected public exit after Chameleon: the original Server 1 public IP.\n'
