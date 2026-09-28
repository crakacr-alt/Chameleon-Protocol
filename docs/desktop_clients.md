# Chameleon Desktop clients

Chameleon Desktop packages the same open-source Go client for Windows and Linux.
There is no separate closed binary or second routing engine.

## Published assets

Every desktop release contains:

- `chameleon-linux-amd64`;
- `chameleon-linux-arm64`;
- `chameleon-windows-amd64.exe`;
- `chameleon-windows-arm64.exe`;
- `SHA256SUMS`;
- standalone Linux and Windows installers.

The GitHub release tag is:

`desktop-v<VERSION>`

For example, protocol/client 1.0.2 is published as `desktop-v1.0.2`.

## Linux

Download `install-client-release.sh` from the matching GitHub release and run:

```bash
sudo bash install-client-release.sh /path/to/client-profile.txt --mode smart
```

The installer:

1. reads the current project version;
2. downloads the matching Linux binary for amd64 or arm64;
3. downloads `SHA256SUMS`;
4. verifies SHA-256 before replacing the binary;
5. stores the imported profile under `/etc/chameleon-client`;
6. creates a restricted `chameleon-client` systemd service;
7. creates a daily update timer.

Useful commands:

```bash
systemctl status chameleon-client
systemctl status chameleon-client-update.timer
journalctl -u chameleon-client -f
sudo -u chameleon-client chameleon doctor --config /etc/chameleon-client/config.json
```

The automatic updater only installs an official release whose binary matches the
published SHA-256 list.

## Windows

Download `install-client-release.ps1` from the matching GitHub release.
Open PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-client-release.ps1 -Profile C:\path\client-profile.txt -Mode smart
```

The installer:

1. detects x64 or ARM64;
2. downloads the matching official `.exe`;
3. verifies it against `SHA256SUMS`;
4. imports the server profile into `%ProgramData%\Chameleon`;
5. registers `ChameleonClient` as a startup Scheduled Task;
6. registers a daily verified update task.

Useful commands:

```powershell
Get-ScheduledTask ChameleonClient
Get-ScheduledTask ChameleonClientUpdate
& "$env:ProgramFiles\Chameleon\chameleon.exe" doctor --config "$env:ProgramData\Chameleon\config.json"
```

A Scheduled Task is used instead of pretending the console client implements
the native Windows Service Control Manager protocol.

## Modes

`smart` keeps direct connectivity available and lets the adaptive engine use
Chameleon carriers when needed.

`proxy` removes the direct TCP carrier for traffic entering the local SOCKS5
listener and therefore requires a reachable Chameleon ingress.

Both desktop clients expose:

`SOCKS5 127.0.0.1:1080`

## Source and reproducibility

All installers, build workflows and client sources live in this repository.
GitHub Actions runs the Go tests before producing release binaries.

The release build uses `-trimpath` and disables CGO for the portable desktop
artifacts. Published hashes allow an installed file to be compared with the
release asset.
