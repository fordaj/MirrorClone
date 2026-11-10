#!/usr/bin/env bash
set -euo pipefail

# --- Source Scripts --- #
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/entry.sh"

cd "$SCRIPT_DIR/.."

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || {
		log ERROR "required command not found: $1"
		exit 1
	}
}

need_cmd gofmt

log INFO "formatting the go sources..."
gofmt -w -l .

if command -v goimports >/dev/null 2>&1; then
	log INFO "running goimports..."
	goimports -w .
else
	log INFO "goimports is not installed, skipping the import formatting"
fi

log INFO "format complete"
