#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${CHAMELEON_REPO_URL:-https://github.com/crakacr-alt/Chameleon-Protocol.git}"
REPO_DIR="${CHAMELEON_REPO_DIR:-/opt/chameleon}"
ENV_DIR="/etc/chameleon"
ENV_FILE="$ENV_DIR/tunnel.env"
CERT_FILE="$ENV_DIR/tls.crt"
KEY_FILE="$ENV_DIR/tls.key"
DECOY_FILE="$ENV_DIR/decoy.html"
CLIENT_FILE="$ENV_DIR/client-profile.txt"
SERVICE_FILE="/etc/systemd/system/chameleon-tunnel.service"
HEALTH_SERVICE="/etc/systemd/system/chameleon-health.service"
HEALTH_TIMER="/etc/systemd/system/chameleon-health.timer"
BIN_SERVER="/usr/local/bin/chameleon-tunnel-server"
BIN_PROXY="/usr/local/bin/chameleon-proxy"
BIN_CTL="/usr/local/bin/chameleonctl"
LIB_DIR="/usr/local/lib/chameleon"
GO_VERSION="${CHAMELEON_GO_VERSION:-1.25.0}"

log() {
  printf '[chameleon] %s\n' "$*"
}

die() {
  printf '[chameleon] ERROR: %s\n' "$*" >&2
  exit 1
}

if [ "$(id -u)" -ne 0 ]; then
  die "run as root: sudo ./deploy/install-server.sh"
fi

export DEBIAN_FRONTEND=noninteractive

install_packages() {
  if command -v apt-get >/dev/null 2>&1; then
    log "installing base packages"
    apt-get update -y
    apt-get install -y --no-install-recommends git ca-certificates curl openssl iproute2
    return
  fi

  for cmd in git curl openssl ss; do
    command -v "$cmd" >/dev/null 2>&1 || die "missing $cmd; install it first"
  done
}

version_ge() {
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

ensure_go() {
  local current="" arch="" archive=""
  if command -v go >/dev/null 2>&1; then
    current="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  fi

  if [ -n "$current" ] && version_ge "$current" "1.25.0"; then
    log "using Go $current"
    return
  fi

  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported CPU architecture: $(uname -m)" ;;
  esac

  archive="/tmp/go${GO_VERSION}.linux-${arch}.tar.gz"
  log "installing Go $GO_VERSION for $arch"
  curl -fL --retry 3 --connect-timeout 10     "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz"     -o "$archive"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$archive"
  rm -f "$archive"
  export PATH="/usr/local/go/bin:$PATH"

  command -v go >/dev/null 2>&1 || die "Go installation failed"
}

prepare_source() {
  local script_dir source_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  source_dir="$(cd "$script_dir/.." && pwd)"

  if [ -f "$source_dir/go.mod" ] && [ "${CHAMELEON_FORCE_GIT:-0}" != "1" ]; then
    log "building from current source: $source_dir"
    printf '%s' "$source_dir"
    return
  fi

  mkdir -p "$(dirname "$REPO_DIR")"
  if [ ! -d "$REPO_DIR/.git" ]; then
    log "cloning repository"
    git clone --depth=1 --branch=main "$REPO_URL" "$REPO_DIR"
  else
    log "updating managed source"
    git -C "$REPO_DIR" fetch --depth=1 origin main
    git -C "$REPO_DIR" checkout -B main origin/main
  fi
  printf '%s' "$REPO_DIR"
}

ensure_user() {
  if ! id chameleon >/dev/null 2>&1; then
    useradd --system --user-group --home-dir /var/lib/chameleon       --create-home --shell /usr/sbin/nologin chameleon
  fi
  install -d -o root -g chameleon -m 0750 "$ENV_DIR"
  install -d -o chameleon -g chameleon -m 0750 /var/lib/chameleon
  install -d -o root -g root -m 0755 "$LIB_DIR"
}

read_existing_value() {
  local key="$1"
  [ -f "$ENV_FILE" ] || return 0
  sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n1
}

port_in_use() {
  local port="$1"
  ss -H -ltn "sport = :$port" 2>/dev/null | grep -q .
}

choose_port() {
  local requested="${CHAMELEON_PORT:-}"
  local existing_listen existing_port

  if [ -n "$requested" ]; then
    [[ "$requested" =~ ^[0-9]+$ ]] || die "CHAMELEON_PORT must be numeric"
    [ "$requested" -ge 1 ] && [ "$requested" -le 65535 ] || die "invalid CHAMELEON_PORT"
    printf '%s' "$requested"
    return
  fi

  existing_listen="$(read_existing_value CHAMELEON_LISTEN || true)"
  existing_port="${existing_listen##*:}"
  if [[ "$existing_port" =~ ^[0-9]+$ ]]; then
    printf '%s' "$existing_port"
    return
  fi

  # Older installs used 9443. Keep that port on upgrade.
  if [ -f "$ENV_FILE" ]; then
    printf '9443'
    return
  fi

  if ! port_in_use 443; then
    printf '443'
  else
    printf '9443'
  fi
}

ensure_psk() {
  local value="${CHAMELEON_TUNNEL_PSK:-}"
  if [ -z "$value" ]; then
    value="$(read_existing_value CHAMELEON_TUNNEL_PSK || true)"
  fi
  if [ -z "$value" ]; then
    value="$(openssl rand -hex 32)"
  fi

  [[ "$value" =~ ^[A-Za-z0-9._~+-]{16,256}$ ]] ||     die "PSK must contain 16-256 safe ASCII characters"
  printf '%s' "$value"
}

ensure_certificate() {
  if [ -n "${CHAMELEON_TLS_CERT:-}" ] || [ -n "${CHAMELEON_TLS_KEY:-}" ]; then
    [ -n "${CHAMELEON_TLS_CERT:-}" ] && [ -n "${CHAMELEON_TLS_KEY:-}" ] ||       die "set both CHAMELEON_TLS_CERT and CHAMELEON_TLS_KEY"
    [ -r "$CHAMELEON_TLS_CERT" ] || die "cannot read CHAMELEON_TLS_CERT"
    [ -r "$CHAMELEON_TLS_KEY" ] || die "cannot read CHAMELEON_TLS_KEY"
    install -o root -g chameleon -m 0640 "$CHAMELEON_TLS_CERT" "$CERT_FILE"
    install -o root -g chameleon -m 0640 "$CHAMELEON_TLS_KEY" "$KEY_FILE"
    return
  fi

  if [ -s "$CERT_FILE" ] && [ -s "$KEY_FILE" ]; then
    log "keeping existing TLS certificate"
    chown root:chameleon "$CERT_FILE" "$KEY_FILE"
    chmod 0640 "$CERT_FILE" "$KEY_FILE"
    return
  fi

  log "generating pinned self-signed TLS certificate"
  openssl ecparam -name prime256v1 -genkey -noout -out "$KEY_FILE"
  openssl req -new -x509 -sha256 -days 825     -key "$KEY_FILE"     -out "$CERT_FILE"     -subj "/CN=Chameleon Transport"
  chown root:chameleon "$CERT_FILE" "$KEY_FILE"
  chmod 0640 "$CERT_FILE" "$KEY_FILE"
}

ensure_decoy() {
  if [ -n "${CHAMELEON_DECOY_FILE:-}" ]; then
    [ -r "$CHAMELEON_DECOY_FILE" ] || die "cannot read CHAMELEON_DECOY_FILE"
    install -o root -g chameleon -m 0640 "$CHAMELEON_DECOY_FILE" "$DECOY_FILE"
    return
  fi

  if [ -s "$DECOY_FILE" ]; then
    return
  fi

  cat >"$DECOY_FILE" <<'HTML'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Welcome</title></head>
<body><h1>Welcome</h1><p>Service is online.</p></body>
</html>
HTML
  chown root:chameleon "$DECOY_FILE"
  chmod 0640 "$DECOY_FILE"
}

build_binaries() {
  local src="$1"
  log "running tests before installation"
  (
    cd "$src"
    go test ./...
  )

  log "building server and proxy"
  (
    cd "$src"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"       -o "$BIN_SERVER.tmp" ./cmd/tunnel-server
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"       -o "$BIN_PROXY.tmp" ./cmd/proxy
  )
  install -o root -g root -m 0755 "$BIN_SERVER.tmp" "$BIN_SERVER"
  install -o root -g root -m 0755 "$BIN_PROXY.tmp" "$BIN_PROXY"
  rm -f "$BIN_SERVER.tmp" "$BIN_PROXY.tmp"
}

write_environment() {
  local port="$1" psk="$2"
  umask 077
  cat >"$ENV_FILE" <<EOF
CHAMELEON_TUNNEL_PSK=$psk
CHAMELEON_LISTEN=:$port
CHAMELEON_TLS_CERT=$CERT_FILE
CHAMELEON_TLS_KEY=$KEY_FILE
CHAMELEON_DECOY_FILE=$DECOY_FILE
EOF
  chown root:chameleon "$ENV_FILE"
  chmod 0640 "$ENV_FILE"
}

install_units() {
  local src="$1"
  install -o root -g root -m 0644     "$src/deploy/chameleon-tunnel.service" "$SERVICE_FILE"
  install -o root -g root -m 0644     "$src/deploy/chameleon-health.service" "$HEALTH_SERVICE"
  install -o root -g root -m 0644     "$src/deploy/chameleon-health.timer" "$HEALTH_TIMER"
  install -o root -g root -m 0755     "$src/deploy/healthcheck.sh" "$LIB_DIR/healthcheck.sh"
  install -o root -g root -m 0755     "$src/deploy/chameleonctl" "$BIN_CTL"

  systemctl daemon-reload
  systemctl enable --now chameleon-tunnel.service
  systemctl enable --now chameleon-health.timer
}

open_firewall() {
  local port="$1"
  if [ "${CHAMELEON_SKIP_FIREWALL:-0}" = "1" ]; then
    return
  fi

  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
    ufw allow "$port/tcp" comment 'Chameleon tunnel' >/dev/null
    log "allowed TCP/$port in UFW"
    return
  fi

  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port/tcp" >/dev/null
    firewall-cmd --reload >/dev/null
    log "allowed TCP/$port in firewalld"
  fi
}

detect_public_host() {
  local value="${CHAMELEON_PUBLIC_HOST:-}"
  if [ -n "$value" ]; then
    printf '%s' "$value"
    return
  fi

  value="$(curl -4fsS --connect-timeout 3 https://api.ipify.org 2>/dev/null || true)"
  if [ -z "$value" ]; then
    value="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  if [ -z "$value" ]; then
    value="YOUR_SERVER_IP"
  fi
  printf '%s' "$value"
}

write_client_profile() {
  local host="$1" port="$2" psk="$3" fingerprint="$4"
  umask 077
  cat >"$CLIENT_FILE" <<EOF
CHAMELEON_SERVER=$host:$port
CHAMELEON_TUNNEL_PSK=$psk
CHAMELEON_TLS_FINGERPRINT=$fingerprint

Linux local proxy example:
  export CHAMELEON_TUNNEL_PSK='$psk'
  chameleon-proxy --chameleon-tls='$host:$port' --tls-fingerprint='$fingerprint'
EOF
  chmod 0600 "$CLIENT_FILE"
}

verify_service() {
  local port="$1"
  log "checking service"
  systemctl is-active --quiet chameleon-tunnel.service || {
    systemctl status chameleon-tunnel.service --no-pager || true
    journalctl -u chameleon-tunnel.service -n 50 --no-pager || true
    die "chameleon-tunnel did not start"
  }

  if ! curl -kfsS --connect-timeout 4 --max-time 6     "https://127.0.0.1:$port/" >/dev/null; then
    journalctl -u chameleon-tunnel.service -n 50 --no-pager || true
    die "local TLS health check failed"
  fi
}

main() {
  install_packages
  ensure_go
  ensure_user

  local src port psk fingerprint public_host
  src="$(prepare_source)"
  port="$(choose_port)"
  psk="$(ensure_psk)"

  ensure_certificate
  ensure_decoy
  build_binaries "$src"
  write_environment "$port" "$psk"
  install_units "$src"
  open_firewall "$port"
  verify_service "$port"

  fingerprint="$(
    openssl x509 -in "$CERT_FILE" -outform DER |
      openssl dgst -sha256 -hex |
      awk '{print $2}'
  )"
  public_host="$(detect_public_host)"
  write_client_profile "$public_host" "$port" "$psk" "$fingerprint"

  printf '\n'
  log "installation complete"
  printf 'Server:      %s:%s\n' "$public_host" "$port"
  printf 'TLS pin:     %s\n' "$fingerprint"
  printf 'Client info: %s\n' "$CLIENT_FILE"
  printf 'Status:      chameleonctl status\n'
  printf 'Logs:        chameleonctl logs\n'
  printf 'Health:      chameleonctl health\n'
  printf '\n'
  printf 'Secret PSK is stored only in %s and %s (root-readable).\n' "$ENV_FILE" "$CLIENT_FILE"
}

main "$@"
