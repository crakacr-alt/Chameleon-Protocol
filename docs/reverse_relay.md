# Chameleon reverse relay

This deployment is for a case where the existing Chameleon exit VPS cannot be
reached directly by a client, but the exit VPS can make normal outgoing
connections to a second reachable VPS.

Example:

```text
Android / Windows / Linux
          |
          | TLS Chameleon
          v
NEW reachable VPS
80.86.216.169:9443
          |
          | restricted reverse SSH TCP forward
          | connection is initiated by the old VPS
          v
OLD Chameleon exit
127.0.0.1:9443
          |
          v
Internet
```

The relay does not terminate the Chameleon TLS tunnel. It only forwards bytes.
The existing Chameleon PSK and pinned TLS certificate remain on the old exit.

## Important properties

- clients do not connect to the old public IP on this route;
- the old exit initiates the persistent link toward the new VPS;
- the ingress SSH key is restricted to one reverse listener;
- the generated relay client profile is root-readable only;
- the current reverse link transports TCP/TLS only;
- QUIC/UDP is intentionally disabled in the relay profile.

The extra hop can increase latency. It is primarily an availability/ingress
solution, not a claim that two servers automatically make a connection faster.

## Step 1 — old exit: generate pairing key

On the existing Chameleon VPS:

```bash
cd /opt/chameleon
sudo git fetch origin
sudo git reset --hard origin/main
sudo bash deploy/prepare-reverse-relay-exit.sh
```

Copy only the printed `ssh-ed25519 ...` public key. It is not a secret.

Do not copy the Chameleon client profile or PSK into chat.

## Step 2 — new ingress: install restricted relay account

On the new reachable Ubuntu 24.04 VPS:

```bash
git clone https://github.com/crakacr-alt/Chameleon-Protocol.git
cd Chameleon-Protocol
sudo bash deploy/install-reverse-relay-ingress.sh 'PASTE_PUBLIC_KEY_HERE' 9443
```

The installer prints the new VPS SSH host fingerprint. Keep it for
out-of-band verification if possible.

The listener on TCP/9443 appears after the old exit establishes the reverse
link.

## Step 3 — old exit: enable persistent link

For ingress `80.86.216.169`:

```bash
sudo bash deploy/enable-reverse-relay-exit.sh 80.86.216.169 22 9443 9443
```

Check:

```bash
systemctl status chameleon-reverse-relay --no-pager
journalctl -u chameleon-reverse-relay -n 50 --no-pager
```

The script creates:

```text
/etc/chameleon/client-profile-relay.txt
```

This is the profile to import into Android/Windows/Linux when testing the relay
path. It points TLS at the new ingress VPS while preserving the old exit's PSK
and TLS fingerprint.

## Step 4 — external test

From a client network:

```bash
curl -k -v --connect-timeout 8 https://80.86.216.169:9443/ -o /dev/null
```

A TLS/HTTP response proves the TCP route reaches the old Chameleon TLS front
through the new ingress.

Then import `client-profile-relay.txt` and test through Chameleon SOCKS5.

## Security note

The setup script pins the SSH host key it sees during pairing and uses
`StrictHostKeyChecking=yes` afterwards. For the strongest first-use check,
compare the host fingerprint printed on the new VPS console before enabling the
old exit link.

A future native Chameleon relay can add authenticated UDP/QUIC forwarding and
multi-ingress selection. This reverse-SSH deployment intentionally stays
simple, inspectable and TCP-only.
