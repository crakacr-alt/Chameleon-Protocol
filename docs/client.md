# Chameleon 1.0 client

Version 1.0 introduces one user-facing client command:

```text
chameleon
```

The old research commands remain available, but normal users should not need to
assemble planner, proxy and tunnel flags by hand.

## Import the server profile

The server installer creates:

```text
/etc/chameleon/client-profile.txt
```

Copy that file securely to the client and run:

```bash
chameleon import client-profile.txt
```

The generated JSON config is stored with owner-only permissions in the normal
user config directory.

Check it without exposing the PSK:

```bash
chameleon show
```

The PSK is redacted in output.

## Doctor

Before connecting:

```bash
chameleon doctor
```

Doctor checks:

- config schema;
- local network context;
- authenticated QUIC first hop;
- authenticated TLS first hop;
- raw TCP first hop when configured.

For support/automation:

```bash
chameleon doctor --json > doctor.json
```

The report does not contain the PSK.

## Connect

```bash
chameleon connect
```

The default local entry point is:

```text
SOCKS5 127.0.0.1:1080
```

TCP CONNECT and SOCKS5 UDP ASSOCIATE are supported.

## Modes

### smart

Default.

The client may use direct connectivity when it works and learned Chameleon
carriers when the access network requires them.

### proxy

Direct carrier is removed from the candidate set. Chameleon QUIC/TLS/TCP
carriers are used for proxied TCP traffic.

For UDP, `auto` becomes QUIC in proxy mode.

## Bypass

The JSON config supports:

```json
"bypass": [
  "localhost",
  "127.0.0.0/8",
  "::1/128",
  ".home.arpa",
  "192.168.0.0/16"
]
```

Rules may be:

- exact host names;
- domain suffixes beginning with a dot;
- IPv4/IPv6 CIDR ranges.

Bypass affects TCP destination routing in 1.0.

## Stable config schema

1.0 starts the stable config migration contract.

Every config contains:

```json
"schema_version": 1
```

The loader:

- fills defaults for missing v1 fields;
- rejects future unknown schema versions instead of guessing;
- validates mode/endpoints/PSK/durations before opening a listener.

Future compatible releases must migrate older schemas explicitly.

## Linux service installation

On the client machine:

```bash
git clone https://github.com/crakacr-alt/Chameleon-Protocol.git
cd Chameleon-Protocol
sudo ./deploy/install-client.sh /path/to/client-profile.txt
```

This installs:

- `/usr/local/bin/chameleon`;
- `/etc/chameleon-client/config.json`;
- `/var/lib/chameleon-client`;
- `chameleon-client.service`.

Useful commands:

```bash
systemctl status chameleon-client
journalctl -u chameleon-client -f
sudo -u chameleon-client chameleon doctor --config /etc/chameleon-client/config.json
```

The systemd service restarts automatically after process failures. New
connections detect the current network context, so Wi-Fi/mobile changes do not
require editing the config.

1.0 does not claim transparent resumption of already-open application streams
during a network handoff. That belongs to the later session-resume architecture.

## Windows service installation

Run PowerShell as Administrator from a source checkout with Go 1.27+ installed:

```powershell
.\deploy\install-client.ps1 -Profile C:\path\client-profile.txt
```

The script:

- runs the Go tests;
- builds `chameleon.exe`;
- imports the profile;
- registers `ChameleonClient` as an automatic Windows service.

The Linux CI cross-builds the Windows client and parses the PowerShell installer
syntax. Native Windows runtime validation remains a separate platform smoke-test
target for later CI expansion.

## Status

```bash
chameleon status
```

This checks whether the configured local SOCKS listener is reachable.

## Rollback

Stable source snapshots are kept under:

```text
versions/vX.Y.Z
```

A server/client can therefore be rebuilt from an earlier frozen release without
depending on the current `main` branch.

Configuration compatibility must still be checked against that older release.
