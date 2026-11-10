#!/usr/bin/env bash
#
# run.bash - run the built mirror-clone binary.
#
# The binary is resolved with the environment cascade (scripts/env.sh): the
# platform env file of the machine decides which build is used, the same way
# it decides which bundled tools are used. Every argument is passed straight
# to the binary.
#
# Usage:
#   ./run.bash --branch main git@github.com:fordaj/AirFrame.git
#   ./run.bash --help
#   DEBUG=true ./run.bash --help
#
# The build is looked up in this order:
#   1. $MIRROR_CLONE_BIN
#   2. dist/<platform>/mirror-clone[.exe]        (release build)
#   3. dist/debug/<platform>/mirror-clone[.exe]  (DEBUG=true)
#
# Environment:
#   DEBUG=true        Run the debug build instead of the release build.
#   MIRROR_CLONE_BIN  Run this binary instead of the one in dist/.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/entry.sh"

APP_NAME="mirror-clone"

die() {
    log ERROR "$*" || true
    exit 1
}

# --- Resolve the binary ------------------------------------------------------

PLATFORM="${PROJECT_PLATFORM:-}"
[[ -n "$PLATFORM" ]] || die "the platform could not be resolved from the environment cascade"

case "$PLATFORM" in
windows-*) BINARY_NAME="$APP_NAME.exe" ;;
*) BINARY_NAME="$APP_NAME" ;;
esac

BUILD_DIR="$SCRIPT_DIR/dist"
if [[ "${DEBUG:-false}" == "true" || "${DEBUG:-}" == "1" ]]; then
    BUILD_DIR="$BUILD_DIR/debug"
fi

BINARY="${MIRROR_CLONE_BIN:-$BUILD_DIR/$PLATFORM/$BINARY_NAME}"

if [[ ! -x "$BINARY" ]]; then
    if [[ -n "${MIRROR_CLONE_BIN:-}" ]]; then
        die "the binary $BINARY does not exist or is not executable"
    fi
    log ERROR "no binary for $PLATFORM at $BINARY" >&2
    if [[ -x "$SCRIPT_DIR/dist/debug/$PLATFORM/$BINARY_NAME" ]]; then
        log ERROR "there is a debug build, run it with: DEBUG=true $0 $*" >&2
    else
        log ERROR "build it with: $SCRIPT_DIR/build.bash --platform $PLATFORM" >&2
    fi
    exit 1
fi

# The log goes to stderr, so the output of the binary stays untouched.
log INFO "running $BINARY" >&2

exec "$BINARY" "$@"
