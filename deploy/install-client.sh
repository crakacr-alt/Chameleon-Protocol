#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${CHAMELEON_REPO_URL:-https://github.com/crakacr-alt/Chameleon-Protocol.git}"
REPO_DIR="${CHAMELEON_CLIENT_REPO_DIR:-/opt/chameleon-client}"
REF="${CHAMELEON_REF:-main}"
GO_VERSION="${CHAMELEON_GO_VERSION:-1.27.0}"
BIN="/usr/local/bin/chameleon"
CONFIG_DIR="/etc/chameleon-client"
CONFIG_FILE="$CONFIG_DIR/config.json"
STATE_DIR="/var/lib/chameleon-client"
SERVICE="/etc/systemd/system/chameleon-client.service"

log() { printf '[chameleon-client] %s\n' "$*" >&2; }
die() { printf '[chameleon-client] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"

profile="${1:-${CHAMELEON_PROFILE:-}}"

if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y --no-install-recommends git ca-certificates curl
fi

ensure_go() {
  local current="" arch="" archive=""
  if command -v go >/dev/null 2>&1; then
    current="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  fi
  if [ -n "$current" ] && [ "$(printf '%s\n%s\n' "1.27.0" "$current" | sort -V | head -n1)" = "1.27.0" ]; then
    return
  fi
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "unsupported CPU architecture: $(uname -m)" ;;
  esac
  archive="/tmp/go${GO_VERSION}.linux-${arch}.tar.gz"
  log "installing Go $GO_VERSION"
  curl -fL --retry 3 "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o "$archive"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$archive"
  rm -f "$archive"
  export PATH="/usr/local/go/bin:$PATH"
}

ensure_go

if ! id chameleon-client >/dev/null 2>&1; then
  useradd --system --user-group --home-dir "$STATE_DIR" --create-home --shell /usr/sbin/nologin chameleon-client
fi
install -d -o root -g chameleon-client -m 0750 "$CONFIG_DIR"
install -d -o chameleon-client -g chameleon-client -m 0750 "$STATE_DIR"

if [ ! -d "$REPO_DIR/.git" ]; then
  [ ! -e "$REPO_DIR" ] || die "$REPO_DIR exists but is not a git checkout"
  git clone "$REPO_URL" "$REPO_DIR"
fi

git -C "$REPO_DIR" fetch origin "$REF"
git -C "$REPO_DIR" checkout -B chameleon-client-install "origin/$REF" 2>/dev/null ||   git -C "$REPO_DIR" checkout -B chameleon-client-install "$REF"

log "testing source"
(
  cd "$REPO_DIR"
  go test ./...
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /tmp/chameleon-client-bin ./cmd/chameleon
)
install -o root -g root -m 0755 /tmp/chameleon-client-bin "$BIN"
rm -f /tmp/chameleon-client-bin

if [ -n "$profile" ]; then
  [ -r "$profile" ] || die "cannot read profile: $profile"
  "$BIN" import "$profile" --config "$CONFIG_FILE" --state-dir "$STATE_DIR" --mode "${CHAMELEON_MODE:-smart}"
elif [ ! -s "$CONFIG_FILE" ]; then
  die "first install requires server profile: sudo ./deploy/install-client.sh /path/client-profile.txt"
fi

chown root:chameleon-client "$CONFIG_FILE"
chmod 0640 "$CONFIG_FILE"

install -o root -g root -m 0644 "$REPO_DIR/deploy/chameleon-client.service" "$SERVICE"
systemctl daemon-reload
systemctl enable chameleon-client.service >/dev/null
systemctl restart chameleon-client.service

sleep 1
if ! systemctl is-active --quiet chameleon-client.service; then
  systemctl status chameleon-client.service --no-pager || true
  journalctl -u chameleon-client.service -n 80 --no-pager || true
  die "client service failed to start"
fi

log "client installed"
printf 'SOCKS5: 127.0.0.1:1080\n'
printf 'Status: systemctl status chameleon-client\n'
printf 'Doctor: sudo -u chameleon-client chameleon doctor --config %s\n' "$CONFIG_FILE"
printf 'Logs:   journalctl -u chameleon-client -f\n'
