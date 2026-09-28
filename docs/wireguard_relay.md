# Chameleon WireGuard relay

Preferred two-server layout when the original Chameleon VPS cannot accept
connections from the client network.

```text
Android / Windows / Linux
          |
          | Chameleon QUIC or TLS/TCP
          v
Server 2 — public ingress
80.86.216.169:9443 TCP+UDP
          |
          | private WireGuard link
          | Server 1 initiates/keeps the peer alive
          v
Server 1 — Chameleon exit
10.77.0.2:9443
(public 88.210.20.127 may be unreachable to clients)
          |
          v
Internet
```

The client never needs to connect to `88.210.20.127` on this path. Server 2
forwards both TCP and UDP, so Chameleon TLS/TCP **and QUIC datagrams** remain
available. This is preferred over the older reverse-SSH relay when Android VPN
mode needs UDP/DNS.

## Security model

The public client-facing hop is still the real IP of Server 2. It cannot be
cryptographically hidden from the access network because packets must have a
routable destination.

The old exit IP can be hidden from the client path. Chameleon authentication,
PSK and TLS certificate pinning stay end-to-end at Server 1. Server 2 only
forwards packets over the private relay link.

The WireGuard keys are dedicated to the inter-server link. Private keys never
need to be copied between servers.

## 1. Server 1 — prepare pairing identity

On the existing Chameleon server:

```bash
cd /root/Chameleon-Protocol
git fetch origin
git reset --hard origin/main
sudo bash deploy/install-server.sh
sudo bash deploy/prepare-wireguard-relay-exit.sh
```

Copy the printed **public** WireGuard key.

Do not paste `/etc/chameleon/client-profile.txt` into chat: it contains the
Chameleon PSK.

## 2. Server 2 — install the ingress relay

On Ubuntu 24.04 Server 2:

```bash
sudo apt update
sudo apt install -y git

git clone https://github.com/crakacr-alt/Chameleon-Protocol.git /root/Chameleon-Protocol
cd /root/Chameleon-Protocol

sudo bash deploy/install-wireguard-relay-ingress.sh 'SERVER1_PUBLIC_KEY' 9443 51820 9443
```

This creates:

- `wgcham0 = 10.77.0.1/24`;
- WireGuard listener `UDP/51820`;
- public Chameleon ingress `TCP+UDP/9443`;
- DNAT/SNAT forwarding to `10.77.0.2:9443`.

The command prints **Server 2 WireGuard public key**. Copy it back to Server 1.

## 3. Server 1 — enable the persistent private link

For Server 2 `80.86.216.169`:

```bash
cd /root/Chameleon-Protocol
sudo bash deploy/enable-wireguard-relay-exit.sh \
  80.86.216.169 \
  'SERVER2_PUBLIC_KEY' \
  51820 \
  9443 \
  9443
```

The script requires a successful WireGuard handshake before it creates the
client profile.

It writes:

```text
/etc/chameleon/client-profile-relay.txt
```

This profile keeps the original Chameleon PSK and TLS fingerprint, but points
TCP/TLS/QUIC at `80.86.216.169:9443`.

## 4. Verify both servers

Server 1:

```bash
sudo wg show wgcham0
systemctl status wg-quick@wgcham0 --no-pager
systemctl status chameleon-tunnel --no-pager
```

Server 2:

```bash
sudo wg show wgcham0
systemctl status wg-quick@wgcham0 --no-pager
curl -k -I --connect-timeout 5 https://10.77.0.2:9443/
```

From an external client:

```bash
curl -k -I --connect-timeout 8 https://80.86.216.169:9443/
```

A response proves the TCP/TLS relay path reaches the old Chameleon server.

## 5. Use the relay profile

Securely copy:

`/etc/chameleon/client-profile-relay.txt`

from Server 1 to the Android/Windows/Linux client.

For Android VPN mode, import that profile and select:

`VPN — весь телефон через Chameleon`

Then verify the public egress:

```bash
curl https://api.ipify.org
```

The expected result is the public exit address of Server 1, while the client
itself connects only to Server 2.

## Operational note

Two servers do not automatically make one flow faster. The second server adds
an ingress/fallback path and usually adds a small amount of latency. It is useful
because the old exit can remain behind an unreachable public IP while QUIC/TCP
traffic still reaches it through the private link.

The older reverse-SSH relay remains available as a simple TCP-only fallback.
