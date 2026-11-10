#!/usr/bin/env bash
#
# build.bash - cross compile the mirror-clone binary.
#
# The binary is built from the Go sources in this repository. Release builds
# are the default (optimized, stripped, version stamped), debug builds keep
# the symbols and the debug information of the compiler.
#
# Usage:
#   ./build.bash                        # release build for the host platform
#   ./build.bash --debug                # debug build for the host platform
#   ./build.bash --all                  # release build for every platform
#   ./build.bash --platform linux-arm64 # release build for one platform
#
# Environment:
#   DEBUG=true        same as --debug
#   BUILD_MODE        release (default) or debug
#   PLATFORMS         space separated list of platforms to build
#   OUTPUT_DIR        output directory (default: dist)
#
# The output layout is:
#   <output>/<platform>/mirror-clone[.exe]      release builds
#   <output>/debug/<platform>/mirror-clone[.exe] debug builds

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/entry.sh"

APP_NAME="mirror-clone"
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/dist}"
MODE="release"
CLEAN=0
ALL=0
PLATFORM_FLAGS=()

SUPPORTED_PLATFORMS=(
    linux-x86_64
    linux-arm64
    macos-x86_64
    macos-arm64
    windows-x86_64
    windows-arm64
)

usage() {
    cat <<EOF
Usage: ./build.bash [options]

Cross compile the $APP_NAME binary.

Options:
  --debug              Build with debug information (no optimizations, no
                       stripping). DEBUG=true does the same.
  --release            Build in release mode (default).
  --all                Build every supported platform.
  --platform PLATFORM  Build the given platform (repeatable). Supported:
                       ${SUPPORTED_PLATFORMS[*]}
  --output DIR         Write the binaries to DIR (default: $OUTPUT_DIR).
  --clean              Remove the output directory before building.
  --help, -h           Show this help.

Environment:
  DEBUG=true           Same as --debug.
  BUILD_MODE           release or debug.
  PLATFORMS            Space separated list of platforms.
  OUTPUT_DIR           Same as --output.
EOF
}

die() {
    log ERROR "$*" || true
    exit 1
}

# --- Arguments ---------------------------------------------------------------

while [[ $# -gt 0 ]]; do
    case "$1" in
    --debug)
        MODE="debug"
        shift
        ;;
    --release)
        MODE="release"
        shift
        ;;
    --all)
        ALL=1
        shift
        ;;
    --platform)
        [[ $# -ge 2 ]] || die "--platform requires a value"
        PLATFORM_FLAGS+=("$2")
        shift 2
        ;;
    --platform=*)
        PLATFORM_FLAGS+=("${1#--platform=}")
        shift
        ;;
    --output)
        [[ $# -ge 2 ]] || die "--output requires a value"
        OUTPUT_DIR="$2"
        shift 2
        ;;
    --output=*)
        OUTPUT_DIR="${1#--output=}"
        shift
        ;;
    --clean)
        CLEAN=1
        shift
        ;;
    --help | -h)
        usage
        exit 0
        ;;
    *)
        log ERROR "unknown argument: $1"
        usage >&2
        exit 2
        ;;
    esac
done

# --- Mode --------------------------------------------------------------------

if [[ "${DEBUG:-false}" == "true" || "${DEBUG:-}" == "1" ]]; then
    MODE="debug"
fi
if [[ -n "${BUILD_MODE:-}" ]]; then
    MODE="$BUILD_MODE"
fi
case "$MODE" in
release | debug) ;;
*) die "invalid build mode: $MODE (expected release or debug)" ;;
esac

if [[ "$MODE" == "debug" ]]; then
    OUTPUT_DIR="$OUTPUT_DIR/debug"
    log INFO "build mode: debug"
else
    log INFO "build mode: release"
fi

# --- Toolchain ---------------------------------------------------------------

need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

need_cmd go
need_cmd git

GO_VERSION="$(go version)"
log INFO "$GO_VERSION"

# --- Targets -----------------------------------------------------------------

# go_target <platform> sets GOOS, GOARCH and EXT for the given platform.
go_target() {
    local platform="$1"
    case "$platform" in
    linux-x86_64)
        GOOS="linux"
        GOARCH="amd64"
        EXT=""
        ;;
    linux-arm64)
        GOOS="linux"
        GOARCH="arm64"
        EXT=""
        ;;
    macos-x86_64)
        GOOS="darwin"
        GOARCH="amd64"
        EXT=""
        ;;
    macos-arm64)
        GOOS="darwin"
        GOARCH="arm64"
        EXT=""
        ;;
    windows-x86_64)
        GOOS="windows"
        GOARCH="amd64"
        EXT=".exe"
        ;;
    windows-arm64)
        GOOS="windows"
        GOARCH="arm64"
        EXT=".exe"
        ;;
    *)
        return 1
        ;;
    esac
}

# detect_host_platform prints the platform of the current machine.
detect_host_platform() {
    local os arch
    case "$(uname -s)" in
    Darwin) os="macos" ;;
    Linux) os="linux" ;;
    MINGW* | MSYS* | CYGWIN* | Windows_NT) os="windows" ;;
    *) os="unknown" ;;
    esac
    case "$(uname -m)" in
    x86_64 | amd64) arch="x86_64" ;;
    arm64 | aarch64) arch="arm64" ;;
    *) arch="unknown" ;;
    esac
    printf '%s-%s' "$os" "$arch"
}

TARGETS=()

add_target() {
    local platform="$1"
    local existing
    for existing in ${TARGETS[@]+"${TARGETS[@]}"}; do
        [[ "$existing" == "$platform" ]] && return 0
    done
    TARGETS+=("$platform")
}

if [[ "$ALL" -eq 1 ]]; then
    for platform in "${SUPPORTED_PLATFORMS[@]}"; do
        add_target "$platform"
    done
fi
if [[ ${#PLATFORM_FLAGS[@]} -gt 0 ]]; then
    for platform in "${PLATFORM_FLAGS[@]}"; do
        add_target "$platform"
    done
elif [[ "$ALL" -eq 0 && -n "${PLATFORMS:-}" ]]; then
    for platform in $PLATFORMS; do
        add_target "$platform"
    done
fi
if [[ ${#TARGETS[@]} -eq 0 ]]; then
    HOST_PLATFORM="$(detect_host_platform)"
    go_target "$HOST_PLATFORM" >/dev/null 2>&1 || die "unsupported host platform: $HOST_PLATFORM"
    add_target "$HOST_PLATFORM"
fi

for platform in "${TARGETS[@]}"; do
    go_target "$platform" >/dev/null 2>&1 || die "unsupported platform: $platform"
done

log INFO "platforms: ${TARGETS[*]}"

# --- Version -----------------------------------------------------------------

VERSION="dev"
if [[ -x "${VERSIONER_BIN:-}" ]]; then
    version_output="$("$SCRIPT_DIR/scripts/version.bash" --format text)" || die "could not compute the version"
    VERSION="$(printf '%s\n' "$version_output" | sed -n 's/^VERSION=//p' | tail -1)"
    [[ -n "$VERSION" ]] || die "could not parse VERSION from the versioner output"
else
    log WARNING "versioner not found, the version is set to dev"
fi

COMMIT="$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
BUILD_DATE="$(git -C "$SCRIPT_DIR" show -s --format=%cI HEAD 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)"
log INFO "version: $VERSION (commit $COMMIT, $BUILD_DATE)"

LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$BUILD_DATE -X main.mode=$MODE"
GCFLAGS=()
if [[ "$MODE" == "debug" ]]; then
    # Keep the symbols and disable the optimizations.
    GCFLAGS=(-gcflags "all=-N -l")
else
    LDFLAGS="-s -w $LDFLAGS"
fi

# --- Build -------------------------------------------------------------------

if [[ "$CLEAN" -eq 1 ]]; then
    log INFO "removing $OUTPUT_DIR"
    rm -rf -- "$OUTPUT_DIR"
fi
mkdir -p -- "$OUTPUT_DIR"

cd "$SCRIPT_DIR"

for platform in "${TARGETS[@]}"; do
    go_target "$platform" || die "unsupported platform: $platform"
    target_dir="$OUTPUT_DIR/$platform"
    binary="$target_dir/$APP_NAME$EXT"

    mkdir -p -- "$target_dir"
    log INFO "building $platform ($GOOS/$GOARCH)..."

    if ! env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -trimpath "${GCFLAGS[@]+"${GCFLAGS[@]}"}" -ldflags "$LDFLAGS" -o "$binary" .; then
        die "build failed for $platform"
    fi

    chmod +x "$binary"
    size="$(wc -c <"$binary" | tr -d ' ')"
    log INFO "built $binary ($size bytes)"
done

log INFO "build complete: $OUTPUT_DIR"
