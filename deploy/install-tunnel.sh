#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[chameleon] install-tunnel.sh is kept for compatibility." >&2
echo "[chameleon] forwarding to install-server.sh" >&2
exec "$SCRIPT_DIR/install-server.sh" "$@"
