#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${CHAMELEON_REPO_URL:-https://github.com/crakacr-alt/Chameleon-Protocol.git}"
REPO_DIR="${CHAMELEON_REPO_DIR:-/opt/chameleon}"
DEFAULT_WG_PORT="${CHAMELEON_WG_PORT:-51821}"
DEFAULT_RELAY_PORT="${CHAMELEON_RELAY_PORT:-9443}"

log() { printf '[chameleon-setup] %s\n' "$*" >&2; }
die() { printf '[chameleon-setup] ERROR: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root: sudo bash setup-network.sh"

install_base() {
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y --no-install-recommends git curl ca-certificates iproute2
  else
    command -v git >/dev/null 2>&1 || die "git is required"
    command -v curl >/dev/null 2>&1 || die "curl is required"
  fi
}

sync_repo() {
  install_base
  mkdir -p "$(dirname "$REPO_DIR")"

  if [ ! -d "$REPO_DIR/.git" ]; then
    [ ! -e "$REPO_DIR" ] || die "$REPO_DIR exists but is not a git checkout"
    log "cloning Chameleon to $REPO_DIR"
    git clone --depth=1 --branch=main "$REPO_URL" "$REPO_DIR"
  else
    log "updating Chameleon source in $REPO_DIR"
    git -C "$REPO_DIR" fetch --depth=1 origin main
    git -C "$REPO_DIR" reset --hard HEAD
    git -C "$REPO_DIR" clean -fdx
    git -C "$REPO_DIR" checkout -B main origin/main
    git -C "$REPO_DIR" reset --hard origin/main
    git -C "$REPO_DIR" clean -fdx
  fi
}

prompt_default() {
  local prompt="$1" default="$2" value=""
  if [ -t 0 ]; then
    read -r -p "$prompt [$default]: " value
  fi
  printf '%s' "${value:-$default}"
}

prompt_required() {
  local prompt="$1" value=""
  while [ -z "$value" ]; do
    if [ ! -t 0 ]; then
      return 1
    fi
    read -r -p "$prompt: " value
  done
  printf '%s' "$value"
}

valid_port() {
  local value="$1"
  [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -ge 1 ] && [ "$value" -le 65535 ]
}

server1_install() {
  sync_repo
  log "installing/updating Chameleon exit server"
  bash "$REPO_DIR/deploy/install-server.sh"
  bash "$REPO_DIR/deploy/prepare-wireguard-relay-exit.sh"

  printf '\n=== SERVER 1 READY ===\n'
  printf 'Server 1 pairing key (PUBLIC, safe to copy):\n'
  cat /etc/chameleon/wg-relay/public.key
  printf '\n\nNext: run this installer on Server 2 and choose "Server 2 / ingress".\n'
}

server2_install() {
  local exit_key relay_port wg_port local_port
  sync_repo

  exit_key="${CHAMELEON_EXIT_PUBLIC_KEY:-}"
  if [ -z "$exit_key" ]; then
    exit_key="$(prompt_required 'Paste Server 1 pairing key (public WireGuard key)')" || die "Server 1 pairing key is required"
  fi

  relay_port="${CHAMELEON_RELAY_PORT:-$(prompt_default 'Public Chameleon port' "$DEFAULT_RELAY_PORT")}"
  wg_port="${CHAMELEON_WG_PORT:-$(prompt_default 'Private WireGuard port' "$DEFAULT_WG_PORT")}"
  local_port="${CHAMELEON_EXIT_PORT:-$(prompt_default 'Chameleon port on Server 1' '9443')}"

  valid_port "$relay_port" || die "invalid public relay port: $relay_port"
  valid_port "$wg_port" || die "invalid WireGuard port: $wg_port"
  valid_port "$local_port" || die "invalid Server 1 Chameleon port: $local_port"

  log "installing Server 2 ingress"
  bash "$REPO_DIR/deploy/install-wireguard-relay-ingress.sh" \
    "$exit_key" "$relay_port" "$wg_port" "$local_port"

  printf '\n=== SERVER 2 READY ===\n'
  printf 'Server 2 pairing key (PUBLIC, safe to copy):\n'
  cat /etc/chameleon/wg-relay/public.key
  printf '\n\nServer 2 public IP: '
  curl -4fsS --connect-timeout 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}'
  printf '\nNow return to Server 1, run this installer again and choose "Pair Server 1 -> Server 2".\n'
}

server1_pair() {
  local ingress_ip ingress_key relay_port wg_port local_port
  sync_repo

  [ -s /etc/chameleon/client-profile.txt ] || die "Server 1 Chameleon is not installed; choose Server 1 install first"
  [ -s /etc/chameleon/wg-relay/private.key ] || bash "$REPO_DIR/deploy/prepare-wireguard-relay-exit.sh"

  ingress_ip="${CHAMELEON_INGRESS_IP:-}"
  if [ -z "$ingress_ip" ]; then
    ingress_ip="$(prompt_required 'Server 2 public IP')" || die "Server 2 IP is required"
  fi

  ingress_key="${CHAMELEON_INGRESS_PUBLIC_KEY:-}"
  if [ -z "$ingress_key" ]; then
    ingress_key="$(prompt_required 'Paste Server 2 pairing key (public WireGuard key)')" || die "Server 2 pairing key is required"
  fi

  relay_port="${CHAMELEON_RELAY_PORT:-$(prompt_default 'Public Chameleon port on Server 2' "$DEFAULT_RELAY_PORT")}"
  wg_port="${CHAMELEON_WG_PORT:-$(prompt_default 'WireGuard port on Server 2' "$DEFAULT_WG_PORT")}"

  local_port="${CHAMELEON_EXIT_PORT:-}"
  if [ -z "$local_port" ] && [ -f /etc/chameleon/tunnel.env ]; then
    local_port="$(sed -n 's/^CHAMELEON_LISTEN=.*://p' /etc/chameleon/tunnel.env | tail -n1)"
  fi
  local_port="${local_port:-9443}"

  valid_port "$relay_port" || die "invalid public relay port: $relay_port"
  valid_port "$wg_port" || die "invalid WireGuard port: $wg_port"
  valid_port "$local_port" || die "invalid local Chameleon port: $local_port"

  log "pairing Server 1 with Server 2"
  bash "$REPO_DIR/deploy/enable-wireguard-relay-exit.sh" \
    "$ingress_ip" "$ingress_key" "$wg_port" "$relay_port" "$local_port"

  printf '\n=== PAIRING COMPLETE ===\n'
  wg show wgcham0 || true
  printf '\nClient relay profile: /etc/chameleon/client-profile-relay.txt\n'
  printf 'Endpoint: '
  awk -F= '$1=="CHAMELEON_SERVER"{print $2; exit}' /etc/chameleon/client-profile-relay.txt
  printf '\nIMPORTANT: do not paste the profile text into chats; it contains the tunnel PSK.\n'
}

show_status() {
  printf '=== Chameleon service ===\n'
  systemctl --no-pager --full status chameleon-tunnel.service 2>/dev/null || true
  printf '\n=== WireGuard relay ===\n'
  wg show wgcham0 2>/dev/null || printf 'wgcham0 is not configured\n'
  printf '\n=== Profiles ===\n'
  for p in /etc/chameleon/client-profile.txt /etc/chameleon/client-profile-relay.txt; do
    if [ -s "$p" ]; then
      printf '%s -> ' "$p"
      awk -F= '$1=="CHAMELEON_SERVER"{print $2; exit}' "$p"
    fi
  done
}

usage() {
  cat <<'EOF_USAGE'
Chameleon network setup

Usage:
  sudo bash setup-network.sh [server1|server2|pair|status]

Interactive mode with no argument shows a menu.
Private WireGuard keys never leave their server. The "pairing key" shown by
this installer is the PUBLIC WireGuard key and is safe to copy between VPSes.
EOF_USAGE
}

main() {
  local action="${1:-}"
  if [ -z "$action" ]; then
    cat <<'EOF_MENU'

Chameleon automatic network installer

  1) Server 1 - install/update Chameleon exit
  2) Server 2 - install public ingress
  3) Pair Server 1 -> Server 2
  4) Show status
  5) Exit

EOF_MENU
    if [ ! -t 0 ]; then
      usage
      exit 2
    fi
    read -r -p 'Choose: ' action
    case "$action" in
      1) action="server1" ;;
      2) action="server2" ;;
      3) action="pair" ;;
      4) action="status" ;;
      5) exit 0 ;;
      *) die "unknown menu choice" ;;
    esac
  fi

  case "$action" in
    server1|exit) server1_install ;;
    server2|ingress) server2_install ;;
    pair) server1_pair ;;
    status) show_status ;;
    -h|--help|help) usage ;;
    *) usage; die "unknown action: $action" ;;
  esac
}

main "$@"
