#!/usr/bin/env bash
set -euo pipefail

# --- Source Scripts --- #
(return 0 2>/dev/null) && sourced=true || sourced=false
if ! "${sourced}"; then
	source scripts/entry.sh
fi

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

# shellcheck disable=SC1091
. "$SCRIPT_DIR/env.sh"

log INFO "running formattington $* all --config .formattington.json"
uv run formattington "$@" all --config .formattington.json
log INFO "format complete"
