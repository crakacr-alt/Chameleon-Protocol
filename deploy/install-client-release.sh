#!/usr/bin/env bash
set -euo pipefail

# Standalone Linux installer for published Chameleon Desktop releases.
# It downloads only official GitHub release assets and verifies SHA-256 before
# replacing the installed client binary.

REPO="crakacr-alt/Chameleon-Protocol"
RAW_BASE="https://raw.githubusercontent.com/${REPO}/main"
RELEASE_BASE="https://github.com/${REPO}/releases/download"
BIN="/usr/local/bin/chameleon"
CONFIG_DIR="/etc/chameleon-client"
CONFIG_FILE="${CONFIG_DIR}/config.json"
STATE_DIR="/var/lib/chameleon-client"
SERVICE="/etc/systemd/system/chameleon-client.service"
UPDATE_SERVICE="/etc/systemd/system/chameleon-client-update.service"
UPDATE_TIMER="/etc/systemd/system/chameleon-client-update.timer"
INSTALLER="/usr/local/lib/chameleon/install-client-release.sh"

log() { printf '[chameleon] %s\n' "$*" >&2; }
die() { printf '[chameleon] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root: sudo bash install-client-release.sh ..."

update_only=false
profile=""
mode="${CHAMELEON_MODE:-smart}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --update-only) update_only=true ;;
    --mode)
      shift
      [ "$#" -gt 0 ] || die "--mode requires smart or proxy"
      mode="$1"
      ;;
    --help|-h)
      cat <<'EOF'
Usage:
  sudo bash install-client-release.sh /path/client-profile.txt [--mode smart|proxy]
  sudo bash install-client-release.sh --update-only

The first form installs the client and daily update timer.
The update-only form safely upgrades an existing installation.
EOF
      exit 0
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      [ -z "$profile" ] || die "only one profile path is allowed"
      profile="$1"
      ;;
  esac
  shift
done

case "$mode" in
  smart|proxy) ;;
  *) die "mode must be smart or proxy" ;;
esac

for cmd in curl sha256sum install systemctl; do
  command -v "$cmd" >/dev/null 2>&1 || die "required command missing: $cmd"
done

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

latest_version() {
  curl -fsSL --retry 3 "${RAW_BASE}/VERSION" | tr -d '[:space:]'
}

install_release_binary() {
  local version="$1"
  local asset="chameleon-linux-${arch}"
  local base="${RELEASE_BASE}/desktop-v${version}"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  log "downloading Chameleon Desktop ${version} for linux/${arch}"
  curl -fL --retry 3 "${base}/${asset}" -o "${tmp}/${asset}"
  curl -fL --retry 3 "${base}/SHA256SUMS" -o "${tmp}/SHA256SUMS"

  local expected actual
  expected="$(awk -v f="$asset" '$2 == f {print $1}' "${tmp}/SHA256SUMS")"
  [ -n "$expected" ] || die "checksum for ${asset} is missing"
  actual="$(sha256sum "${tmp}/${asset}" | awk '{print $1}')"
  [ "$actual" = "$expected" ] || die "SHA-256 mismatch for ${asset}"

  install -o root -g root -m 0755 "${tmp}/${asset}" "$BIN"
  log "installed $("$BIN" version)"
}

version="$(latest_version)"
[ -n "$version" ] || die "could not resolve latest version"

current=""
if [ -x "$BIN" ]; then
  current="$("$BIN" version 2>/dev/null | tr -d '[:space:]' || true)"
fi

if [ "$current" != "$version" ]; then
  install_release_binary "$version"
else
  log "binary already current: ${version}"
fi

if "$update_only"; then
  if systemctl is-enabled chameleon-client.service >/dev/null 2>&1; then
    systemctl restart chameleon-client.service
  fi
  exit 0
fi

[ -n "$profile" ] || {
  [ -s "$CONFIG_FILE" ] || die "first install requires client-profile.txt"
}

if ! id chameleon-client >/dev/null 2>&1; then
  useradd --system --user-group --home-dir "$STATE_DIR"     --create-home --shell /usr/sbin/nologin chameleon-client
fi

install -d -o root -g chameleon-client -m 0750 "$CONFIG_DIR"
install -d -o chameleon-client -g chameleon-client -m 0750 "$STATE_DIR"
install -d -o root -g root -m 0755 "$(dirname "$INSTALLER")"

# Keep a local updater copy. It is replaced whenever the installer is rerun.
if [ -r "$0" ]; then
  install -o root -g root -m 0755 "$0" "$INSTALLER"
fi

if [ -n "$profile" ]; then
  [ -r "$profile" ] || die "cannot read profile: $profile"
  "$BIN" import "$profile"     --config "$CONFIG_FILE"     --state-dir "$STATE_DIR"     --mode "$mode"
fi

chown root:chameleon-client "$CONFIG_FILE"
chmod 0640 "$CONFIG_FILE"

cat > "$SERVICE" <<'EOF'
[Unit]
Description=Chameleon Protocol adaptive client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=chameleon-client
Group=chameleon-client
ExecStart=/usr/local/bin/chameleon connect --config /etc/chameleon-client/config.json
Restart=always
RestartSec=2
TimeoutStopSec=10
LimitNOFILE=65536
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
RestrictNamespaces=true
RestrictAddressFamilies=AF_INET AF_INET6
ReadWritePaths=/var/lib/chameleon-client

[Install]
WantedBy=multi-user.target
EOF

cat > "$UPDATE_SERVICE" <<'EOF'
[Unit]
Description=Update Chameleon client from verified GitHub release
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/lib/chameleon/install-client-release.sh --update-only
EOF

cat > "$UPDATE_TIMER" <<'EOF'
[Unit]
Description=Daily Chameleon client update check

[Timer]
OnBootSec=15min
OnUnitActiveSec=24h
RandomizedDelaySec=30min
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now chameleon-client.service
if [ -x "$INSTALLER" ]; then
  systemctl enable --now chameleon-client-update.timer
else
  log "automatic update timer skipped because installer could not be persisted"
fi

sleep 1
systemctl is-active --quiet chameleon-client.service || {
  systemctl status chameleon-client.service --no-pager || true
  die "client service failed to start"
}

printf 'Installed: %s\n' "$("$BIN" version)"
printf 'SOCKS5:   127.0.0.1:1080\n'
printf 'Status:   systemctl status chameleon-client\n'
printf 'Doctor:   sudo -u chameleon-client chameleon doctor --config %s\n' "$CONFIG_FILE"
printf 'Updates:  systemctl status chameleon-client-update.timer\n'
