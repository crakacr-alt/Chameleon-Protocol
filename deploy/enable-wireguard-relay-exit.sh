#!/usr/bin/env bash
set -euo pipefail

# Run on the existing Chameleon exit VPS after Server 2 is prepared.

INGRESS_HOST="${1:-}"
INGRESS_PUBLIC_KEY="${2:-}"
WG_PORT="${3:-51820}"
RELAY_PORT="${4:-9443}"
LOCAL_CHAMELEON_PORT="${5:-9443}"
RELAY_QUIC="${CHAMELEON_RELAY_QUIC:-0}"

WG_IF="wgcham0"
WG_ADDR="10.77.0.2/24"
INGRESS_WG_IP="10.77.0.1"
KEY_DIR="/etc/chameleon/wg-relay"
PRIVATE_KEY="${KEY_DIR}/private.key"
WG_CONFIG="/etc/wireguard/${WG_IF}.conf"
SOURCE_PROFILE="/etc/chameleon/client-profile.txt"
SOURCE_V2_PROFILE="/etc/chameleon/client-profile-v2.txt"
RELAY_PROFILE="/etc/chameleon/client-profile-relay.txt"
RELAY_V2_PROFILE="/etc/chameleon/client-profile-relay-v2.txt"

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
CHAMELEON_TLS_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_TUNNEL_PSK=$psk
CHAMELEON_TLS_FINGERPRINT=$fingerprint
CHAMELEON_TCP_TRANSPORT=tls
CHAMELEON_UDP_MODE=auto
${server_name:+CHAMELEON_TLS_SERVER_NAME=$server_name}
EOF
if [ "$RELAY_QUIC" = "1" ]; then
  sed -i "/^CHAMELEON_TLS_SERVER=/a CHAMELEON_QUIC_SERVER=$INGRESS_HOST:$RELAY_PORT" "$RELAY_PROFILE"
fi
chmod 0600 "$RELAY_PROFILE"

if [ -s "$SOURCE_V2_PROFILE" ]; then
  client_id="$(awk -F= '$1=="CHAMELEON_CLIENT_ID"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_V2_PROFILE")"
  client_secret="$(awk -F= '$1=="CHAMELEON_CLIENT_SECRET"{print substr($0,index($0,"=")+1); exit}' "$SOURCE_V2_PROFILE")"
  if [ -n "$client_id" ] && [ -n "$client_secret" ]; then
    cat > "$RELAY_V2_PROFILE" <<EOF
CHAMELEON_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_TLS_SERVER=$INGRESS_HOST:$RELAY_PORT
CHAMELEON_CLIENT_ID=$client_id
CHAMELEON_CLIENT_SECRET=$client_secret
CHAMELEON_TLS_FINGERPRINT=$fingerprint
CHAMELEON_TCP_TRANSPORT=tls
CHAMELEON_UDP_MODE=auto
${server_name:+CHAMELEON_TLS_SERVER_NAME=$server_name}
EOF
    if [ "$RELAY_QUIC" = "1" ]; then
      sed -i "/^CHAMELEON_TLS_SERVER=/a CHAMELEON_QUIC_SERVER=$INGRESS_HOST:$RELAY_PORT" "$RELAY_V2_PROFILE"
    fi
    chmod 0600 "$RELAY_V2_PROFILE"
  fi
fi

log "Server 1 is paired with Server 2"
printf 'Private relay link: %s <-> %s\n' "$INGRESS_WG_IP" "10.77.0.2"
if [ "$RELAY_QUIC" = "1" ]; then
  printf 'Client ingress: %s:%s TLS/TCP + QUIC/UDP\n' "$INGRESS_HOST" "$RELAY_PORT"
else
  printf 'Client ingress: %s:%s TLS/TCP\n' "$INGRESS_HOST" "$RELAY_PORT"
fi
printf 'Relay legacy profile: %s\n' "$RELAY_PROFILE"
[ -s "$RELAY_V2_PROFILE" ] && printf 'Relay Auth v2 profile: %s\n' "$RELAY_V2_PROFILE"
printf 'Expected public exit after Chameleon: the original Server 1 public IP.\n'
