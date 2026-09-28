#!/usr/bin/env bash
set -euo pipefail

# Run on the new reachable ingress VPS.
#
# Public TCP+UDP RELAY_PORT is forwarded through wgcham0 to the existing
# Chameleon server on 10.77.0.2:CHAMELEON_PORT. The old server therefore does
# not need any reachable public inbound address.

EXIT_PUBLIC_KEY="${1:-}"
RELAY_PORT="${2:-9443}"
WG_PORT="${3:-51820}"
CHAMELEON_PORT="${4:-9443}"

WG_IF="wgcham0"
WG_ADDR="10.77.0.1/24"
EXIT_WG_IP="10.77.0.2"
KEY_DIR="/etc/chameleon/wg-relay"
PRIVATE_KEY="${KEY_DIR}/private.key"
PUBLIC_KEY="${KEY_DIR}/public.key"
WG_CONFIG="/etc/wireguard/${WG_IF}.conf"
SYSCTL_FILE="/etc/sysctl.d/99-chameleon-wg-relay.conf"

log() { printf '[chameleon-wg] %s\n' "$*" >&2; }
die() { printf '[chameleon-wg] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$EXIT_PUBLIC_KEY" ] || die "usage: sudo bash install-wireguard-relay-ingress.sh EXIT_WG_PUBLIC_KEY [relay-port] [wg-port] [chameleon-port]"

for value in "$RELAY_PORT" "$WG_PORT" "$CHAMELEON_PORT"; do
  case "$value" in
    ''|*[!0-9]*) die "ports must be numeric" ;;
  esac
  [ "$value" -ge 1 ] && [ "$value" -le 65535 ] || die "invalid port: $value"
done

if [ "$RELAY_PORT" = "$WG_PORT" ]; then
  die "relay port and WireGuard port must be different"
fi

# Refuse to steal an existing service port.
if ss -H -ltn "sport = :$RELAY_PORT" 2>/dev/null | grep -q .; then
  die "TCP/$RELAY_PORT is already in use"
fi
if ss -H -lun "sport = :$RELAY_PORT" 2>/dev/null | grep -q .; then
  die "UDP/$RELAY_PORT is already in use"
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y --no-install-recommends wireguard-tools iptables ca-certificates

install -d -o root -g root -m 0700 "$KEY_DIR"
install -d -o root -g root -m 0700 /etc/wireguard

if [ ! -s "$PRIVATE_KEY" ]; then
  umask 077
  wg genkey > "$PRIVATE_KEY"
  wg pubkey < "$PRIVATE_KEY" > "$PUBLIC_KEY"
fi

chmod 0600 "$PRIVATE_KEY"
chmod 0644 "$PUBLIC_KEY"

cat > "$SYSCTL_FILE" <<'EOF'
net.ipv4.ip_forward=1
EOF
sysctl --system >/dev/null

private="$(cat "$PRIVATE_KEY")"

cat > "$WG_CONFIG" <<EOF
[Interface]
Address = $WG_ADDR
ListenPort = $WG_PORT
PrivateKey = $private

# Public Chameleon TCP/TLS and UDP/QUIC ingress.
PostUp = iptables -t nat -I PREROUTING 1 -p tcp --dport $RELAY_PORT -j DNAT --to-destination $EXIT_WG_IP:$CHAMELEON_PORT
PostUp = iptables -t nat -I PREROUTING 1 -p udp --dport $RELAY_PORT -j DNAT --to-destination $EXIT_WG_IP:$CHAMELEON_PORT
PostUp = iptables -t nat -I POSTROUTING 1 -o %i -p tcp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j MASQUERADE
PostUp = iptables -t nat -I POSTROUTING 1 -o %i -p udp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j MASQUERADE
PostUp = iptables -I FORWARD 1 -o %i -p tcp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j ACCEPT
PostUp = iptables -I FORWARD 1 -o %i -p udp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j ACCEPT
PostUp = iptables -I FORWARD 1 -i %i -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

PostDown = iptables -t nat -D PREROUTING -p tcp --dport $RELAY_PORT -j DNAT --to-destination $EXIT_WG_IP:$CHAMELEON_PORT
PostDown = iptables -t nat -D PREROUTING -p udp --dport $RELAY_PORT -j DNAT --to-destination $EXIT_WG_IP:$CHAMELEON_PORT
PostDown = iptables -t nat -D POSTROUTING -o %i -p tcp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -o %i -p udp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j MASQUERADE
PostDown = iptables -D FORWARD -o %i -p tcp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j ACCEPT
PostDown = iptables -D FORWARD -o %i -p udp -d $EXIT_WG_IP --dport $CHAMELEON_PORT -j ACCEPT
PostDown = iptables -D FORWARD -i %i -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

[Peer]
PublicKey = $EXIT_PUBLIC_KEY
AllowedIPs = 10.77.0.2/32
EOF

chmod 0600 "$WG_CONFIG"

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then
  ufw allow "$WG_PORT/udp" comment 'Chameleon WG relay' >/dev/null
  ufw allow "$RELAY_PORT/tcp" comment 'Chameleon TCP relay' >/dev/null
  ufw allow "$RELAY_PORT/udp" comment 'Chameleon QUIC relay' >/dev/null
fi

systemctl enable --now "wg-quick@${WG_IF}.service"
systemctl restart "wg-quick@${WG_IF}.service"

log "Server 2 ingress is ready"
printf '\nServer 2 WireGuard public pairing key:\n\n'
cat "$PUBLIC_KEY"
printf '\nKeep this key for the Server 1 enable command.\n'
printf 'WireGuard: UDP/%s\n' "$WG_PORT"
printf 'Public Chameleon relay: TCP+UDP/%s\n' "$RELAY_PORT"
printf 'Private exit target: %s:%s\n' "$EXIT_WG_IP" "$CHAMELEON_PORT"
