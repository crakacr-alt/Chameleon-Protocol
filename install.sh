#!/usr/bin/env bash
set -euo pipefail
# Bundled installer: use these exact sources, including local fixes.
export CHAMELEON_USE_CURRENT_SOURCE=1
bundle_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec bash "$bundle_dir/deploy/setup-network.sh" "$@"
