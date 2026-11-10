#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "scripts/entry.sh"

usage() {
	cat <<'EOF'
Usage: ./build.bash [--debug]

Build mirror-clone binaries for supported platforms.

Options:
  --debug, -d   Build debug binaries (unstripped, no trimpath optimizations)
  --help, -h    Show this help

Environment:
  DEBUG=true    Same as --debug
EOF
}

DEBUG_BUILD=false
while [[ $# -gt 0 ]]; do
	case "$1" in
	--debug | -d)
		DEBUG_BUILD=true
		shift
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		log ERROR "Unknown argument: $1"
		usage >&2
		exit 2
		;;
	esac
done

if [[ "${DEBUG:-}" == "true" || "${DEBUG:-}" == "1" ]]; then
	DEBUG_BUILD=true
fi

APP_NAME="mirror-clone"
OUT_DIR="${SCRIPT_DIR}/dist"
PKG="./cmd/mirror-clone"

TARGETS=(
	"darwin/amd64"
	"darwin/arm64"
	"linux/amd64"
	"linux/arm64"
	"windows/amd64"
	"windows/arm64"
)

mkdir -p "$OUT_DIR"

if [[ "$DEBUG_BUILD" == "true" ]]; then
	log INFO "Building ${APP_NAME} in debug mode"
	LDFLAGS=""
	GCFLAGS="all=-N -l"
else
	log INFO "Building ${APP_NAME} in release mode"
	LDFLAGS="-s -w"
	GCFLAGS=""
fi

for target in "${TARGETS[@]}"; do
	GOOS="${target%/*}"
	GOARCH="${target#*/}"
	ext=""
	if [[ "$GOOS" == "windows" ]]; then
		ext=".exe"
	fi
	out="${OUT_DIR}/${APP_NAME}-${GOOS}-${GOARCH}${ext}"
	log INFO "Building ${GOOS}/${GOARCH} -> ${out}"
	if [[ "$DEBUG_BUILD" == "true" ]]; then
		CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
			go build -gcflags "$GCFLAGS" -o "$out" "$PKG"
	else
		CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
			go build -trimpath -ldflags "$LDFLAGS" -o "$out" "$PKG"
	fi
done

log INFO "Build complete. Artifacts in ${OUT_DIR}"
