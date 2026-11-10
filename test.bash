#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "scripts/entry.sh"

log INFO "Running gofmt check"
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
	log ERROR "gofmt found unformatted files:"
	printf '%s\n' "$unformatted" >&2
	exit 1
fi

log INFO "Running go vet"
go vet ./...

log INFO "Running go test"
go test ./...

log INFO "All tests passed"
