#!/usr/bin/env bash
#
# format.bash - format the sources of the project with formattington.
#
# Formattington drives the formatters of the project from the configuration
# in .formattington.json: prettier, shfmt, taplo, gofmt and goimports. The
# tool itself is installed with uv, so the formatters never have to be
# installed by hand.
#
# The formatters are run one after another, because gofmt and goimports
# write to the same files and would race against each other when they run
# concurrently.
#
# Usage:
#   ./format.bash                # format every source of the repository
#   ./format.bash --check        # change nothing, fail when a file needs formatting
#   ./format.bash --dry-run      # print the differences, change nothing
#   ./format.bash --tool shfmt   # run a single formatter
#
# Prerequisites:
#   uv (https://docs.astral.sh/uv/) and the formatters of the project. The
#   Go formatter goimports is installed with:
#     go install golang.org/x/tools/cmd/goimports@latest
#
# Environment:
#   FORMATTINGTON_CONFIG_PATH  Path to the formattington configuration file
#                              (default: .formattington.json in this repository)
#   UV_PROJECT                 Path to the uv project (default: this repository)

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/entry.sh"

CHECK=0
DRY_RUN=0
VERBOSE=0
PARALLEL=0
TOOL=""

usage() {
    cat <<EOF
Usage: ./format.bash [options]

Format the sources of the project with formattington. The formatters and
their arguments come from .formattington.json.

Options:
  --check          Change nothing, exit 1 when a file needs formatting.
  --dry-run        Print the differences, change nothing.
  --tool TOOL      Run only TOOL (prettier, shfmt, taplo, gofmt, goimports).
                   The arguments of the tool are taken from the
                   configuration file, so it runs like it does in a full run.
  --parallel       Run the formatters concurrently instead of one after
                   another. Not safe while gofmt and goimports are both used.
  --verbose        Show which executable every formatter resolved to.
  --help, -h       Show this help.

Environment:
  FORMATTINGTON_CONFIG_PATH  Path to the formattington configuration file.
  UV_PROJECT                 Path to the uv project (default: this repository).
EOF
}

die() {
    log ERROR "$*" || true
    exit 1
}

usage_error() {
    log ERROR "$*" || true
    usage >&2
    exit 2
}

need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# config_args <tool> prints the arguments of the tool from the configuration
# file, one per line. A single tool is then run exactly like it runs in a
# full run, with the arguments of the configuration.
config_args() {
    local tool="$1"
    uv run --project "${UV_PROJECT:-$SCRIPT_DIR}" python -c '
import json
import os
import sys

tool = sys.argv[1]
path = os.environ.get("FORMATTINGTON_CONFIG_PATH") or ".formattington.json"
try:
    with open(path, encoding="utf-8") as handle:
        commands = json.load(handle)["commands"]
except (OSError, ValueError, KeyError, TypeError):
    raise SystemExit(0)
for entry in commands:
    if entry and entry[0] == tool:
        for argument in entry[1:]:
            print(argument)
        break
' "$tool" 2>/dev/null || true
}

while [[ $# -gt 0 ]]; do
    case "$1" in
    --check)
        CHECK=1
        shift
        ;;
    --dry-run)
        DRY_RUN=1
        shift
        ;;
    --tool)
        [[ $# -ge 2 ]] || usage_error "--tool requires a value"
        TOOL="$2"
        shift 2
        ;;
    --tool=*)
        TOOL="${1#--tool=}"
        shift
        ;;
    --parallel)
        PARALLEL=1
        shift
        ;;
    --verbose)
        VERBOSE=1
        shift
        ;;
    --help | -h)
        usage
        exit 0
        ;;
    *)
        usage_error "unknown argument: $1"
        ;;
    esac
done

if [[ "$CHECK" -eq 1 && "$DRY_RUN" -eq 1 ]]; then
    usage_error "--check and --dry-run are mutually exclusive"
fi

cd "$SCRIPT_DIR"

need_cmd uv

# goimports is installed with "go install golang.org/x/tools/cmd/goimports@latest",
# which puts it into the Go bin directory. That directory is not on the PATH
# of every machine, so it is added here when it exists.
if command -v go >/dev/null 2>&1; then
    GOPATH_BIN="$(go env GOPATH)/bin"
    if [[ -d "$GOPATH_BIN" ]]; then
        PATH="$GOPATH_BIN:$PATH"
        export PATH
    fi
fi

ARGS=()
if [[ "$CHECK" -eq 1 ]]; then
    ARGS+=(--check)
fi
if [[ "$DRY_RUN" -eq 1 ]]; then
    ARGS+=(--dry-run)
fi
if [[ "$VERBOSE" -eq 1 ]]; then
    ARGS+=(--verbose)
fi
if [[ "$PARALLEL" -eq 0 ]]; then
    ARGS+=(--series)
fi

if [[ -n "$TOOL" ]]; then
    # The arguments of the tool come from the configuration file, so a single
    # tool is not run with different arguments than in a full run.
    TOOL_ARGS=()
    while IFS= read -r argument; do
        TOOL_ARGS+=("$argument")
    done < <(config_args "$TOOL")
    TARGET=("$TOOL" "${TOOL_ARGS[@]+"${TOOL_ARGS[@]}"}")
else
    TARGET=(all)
fi

log INFO "formatting the sources with formattington..."
STATUS=0
# The standard input is redirected to /dev/null: a formatter without matching
# files reads from the standard input and would block on a terminal.
uv run --project "${UV_PROJECT:-$SCRIPT_DIR}" formattington "${ARGS[@]}" "${TARGET[@]}" </dev/null || STATUS=$?

case "$STATUS" in
0)
    log INFO "format complete"
    ;;
1)
    if [[ "$CHECK" -eq 1 ]]; then
        log ERROR "some sources are not formatted, run ./format.bash to fix them"
    else
        log ERROR "a formatter failed, see the output above"
    fi
    ;;
2)
    log ERROR "formattington reported a usage error"
    ;;
127)
    log ERROR "a formatter is not installed, see the output above"
    ;;
*)
    log ERROR "formattington failed with exit code $STATUS"
    ;;
esac

exit "$STATUS"
