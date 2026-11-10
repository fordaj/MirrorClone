#!/usr/bin/env bash
#
# test.bash - run every check of the project.
#
# The checks are:
#   1. gofmt            the sources have to be formatted
#   2. go vet           the sources have to be free of vet findings
#   3. go build         everything has to compile
#   4. go test          the unit and integration tests have to pass
#   5. smoke test       the built binary has to run
#   6. shellcheck       the shell scripts have to be free of findings
#
# Usage:
#   ./test.bash              # run every check
#   ./test.bash --coverage   # also write a coverage profile
#   ./test.bash --quick      # skip the race detector
#
# Environment:
#   GO_TEST_FLAGS   extra flags passed to "go test"
#   SKIP_SHELLCHECK set to true to skip the shellcheck run

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/entry.sh"

APP_NAME="mirror-clone"
COVERAGE=0
QUICK=0
FAILURES=0

usage() {
	cat <<EOF
Usage: ./test.bash [options]

Run every check of the project: gofmt, go vet, go build, go test, a smoke
test of the built binary and shellcheck of the shell scripts.

Options:
  --coverage        Write a coverage profile to coverage.out.
  --quick           Skip the race detector.
  --help, -h        Show this help.

Environment:
  GO_TEST_FLAGS     Extra flags passed to "go test".
  SKIP_SHELLCHECK   Set to true to skip the shellcheck run.
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--coverage)
		COVERAGE=1
		shift
		;;
	--quick)
		QUICK=1
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

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || {
		log ERROR "required command not found: $1"
		FAILURES=$((FAILURES + 1))
		return 1
	}
}

# run_check <name> <command...> runs a check and counts the failures.
run_check() {
	local name="$1"
	shift
	log INFO "running $name..."
	if "$@"; then
		log INFO "$name passed"
		return 0
	fi
	log ERROR "$name failed"
	FAILURES=$((FAILURES + 1))
	return 1
}

cd "$SCRIPT_DIR"

if ! need_cmd go; then
	log ERROR "the go toolchain is required"
	exit 1
fi

log INFO "$(go version)"

# --- 1. gofmt ----------------------------------------------------------------

UNFORMATTED="$(gofmt -l . 2>/dev/null | grep -v '^vendor/' || true)"
if [[ -n "$UNFORMATTED" ]]; then
	log ERROR "the following files are not formatted (run: gofmt -w .):"
	printf '%s\n' "$UNFORMATTED" | sed 's/^/test.bash:   /' >&2
	FAILURES=$((FAILURES + 1))
else
	log INFO "gofmt passed"
fi

# --- 2. go vet ---------------------------------------------------------------

run_check "go vet" go vet ./... || true

# --- 3. go build -------------------------------------------------------------

SMOKE_DIR="$(mktemp -d)"
cleanup() {
	rm -rf -- "$SMOKE_DIR"
}
trap cleanup EXIT

mkdir -p "$SMOKE_DIR/bin"
run_check "go build" go build -o "$SMOKE_DIR/bin/" ./... || true

# --- 4. go test --------------------------------------------------------------

TEST_FLAGS=(-count=1)
if [[ -n "${GO_TEST_FLAGS:-}" ]]; then
	read -r -a EXTRA_TEST_FLAGS <<<"$GO_TEST_FLAGS"
	TEST_FLAGS+=("${EXTRA_TEST_FLAGS[@]}")
fi
if [[ "$COVERAGE" -eq 1 ]]; then
	TEST_FLAGS+=(-coverprofile=coverage.out -covermode=atomic)
fi
if [[ "$QUICK" -eq 0 ]] && [[ "$(go env CGO_ENABLED)" == "1" ]]; then
	TEST_FLAGS+=(-race)
fi

run_check "go test" go test "${TEST_FLAGS[@]}" ./... || true

# --- 5. smoke test -----------------------------------------------------------

if go build -o "$SMOKE_DIR/$APP_NAME" .; then
	if "$SMOKE_DIR/$APP_NAME" --version >/dev/null && "$SMOKE_DIR/$APP_NAME" --help >/dev/null; then
		log INFO "smoke test passed ($("$SMOKE_DIR/$APP_NAME" --version))"
	else
		log ERROR "the smoke test failed"
		FAILURES=$((FAILURES + 1))
	fi
else
	log ERROR "could not build the binary for the smoke test"
	FAILURES=$((FAILURES + 1))
fi

# --- 6. shellcheck -----------------------------------------------------------

if [[ "${SKIP_SHELLCHECK:-false}" == "true" || "${SKIP_SHELLCHECK:-}" == "1" ]]; then
	log INFO "shellcheck skipped"
elif command -v shellcheck >/dev/null 2>&1; then
	SCRIPTS=(
		"$SCRIPT_DIR/build.bash"
		"$SCRIPT_DIR/release.bash"
		"$SCRIPT_DIR/test.bash"
		"$SCRIPT_DIR/run.bash"
		"$SCRIPT_DIR/scripts/entry.sh"
		"$SCRIPT_DIR/scripts/env.sh"
		"$SCRIPT_DIR/scripts/version.bash"
	)
	run_check "shellcheck" shellcheck -x "${SCRIPTS[@]}" || true
else
	log WARNING "shellcheck is not installed, skipping the shell scripts"
fi

# --- Result ------------------------------------------------------------------

if [[ "$FAILURES" -gt 0 ]]; then
	log ERROR "$FAILURES check(s) failed"
	exit 1
fi

log INFO "all checks passed"
