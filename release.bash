#!/usr/bin/env bash
#
# release.bash - build, package and publish a GitHub release.
#
# The version and the tag of the release are computed by versioner, so the
# tag always matches the history of the repository. Every platform is cross
# compiled by build.bash and packaged into one archive that contains the
# binary, the LICENSE and the README.md. The archives are uploaded to the
# GitHub release of the tag.
#
# Usage:
#   ./release.bash                # build, package, tag and publish
#   ./release.bash --dry-run      # verify everything, change nothing
#   ./release.bash --skip-build   # use the binaries that are already in dist/
#
# Prerequisites:
#   git, gh (authenticated with "gh auth login"), zip (or the bundled 7zz)
#   and a clean working tree.
#
# Environment:
#   OUTPUT_DIR     directory with the built binaries (default: dist)
#   RELEASE_DIR    directory with the archives (default: temp/release)
#   SKIP_BUILD     same as --skip-build

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/entry.sh"

APP_NAME="mirror-clone"
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/dist}"
RELEASE_DIR="${RELEASE_DIR:-$SCRIPT_DIR/temp/release}"
DRY_RUN=0
SKIP_BUILD=0
PUBLISH=1

PLATFORMS=(
	linux-x86_64
	linux-arm64
	macos-x86_64
	macos-arm64
	windows-x86_64
	windows-arm64
)

usage() {
	cat <<EOF
Usage: ./release.bash [options]

Build, package and publish a GitHub release. The version and the tag are
computed by versioner, the binaries are cross compiled by build.bash and
packaged with the LICENSE and the README.md into one archive per platform.

Options:
  --dry-run, -n     Verify everything and print the plan, change nothing.
  --skip-build      Use the binaries that are already in $OUTPUT_DIR.
  --package-only    Build and package the archives, but do not tag, push or
                    upload anything.
  --help, -h        Show this help.

Environment:
  OUTPUT_DIR        Directory with the built binaries (default: $OUTPUT_DIR).
  RELEASE_DIR       Directory with the archives (default: $RELEASE_DIR).
  SKIP_BUILD        Same as --skip-build.
EOF
}

die() {
	log ERROR "$*" || true
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run | -n)
		DRY_RUN=1
		shift
		;;
	--skip-build)
		SKIP_BUILD=1
		shift
		;;
	--package-only)
		PUBLISH=0
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

if [[ "${SKIP_BUILD:-false}" == "true" || "${SKIP_BUILD:-}" == "1" ]]; then
	SKIP_BUILD=1
fi

need_cmd git
need_cmd gh

# The archives are created with zip. When zip is not installed the bundled
# 7zz is used to write the very same zip format.
ZIP_TOOL=""
if command -v zip >/dev/null 2>&1; then
	ZIP_TOOL="zip"
elif [[ -n "${SEVENZZ_BIN:-}" && -x "${SEVENZZ_BIN:-}" ]]; then
	ZIP_TOOL="$SEVENZZ_BIN"
else
	die "no zip tool found (install zip or 7z)"
fi
log INFO "creating the archives with $ZIP_TOOL"

if [[ "$DRY_RUN" -eq 1 ]]; then
	log INFO "DRY RUN: nothing is built, tagged, pushed or uploaded"
fi

# --- Repository ---------------------------------------------------------------

if ! REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null)"; then
	die "not inside a git repository"
fi
cd "$REPO_ROOT"

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
log INFO "repo: $REPO_ROOT"
log INFO "branch: $BRANCH  HEAD: $HEAD_SHORT"

if [[ -n "$(git status --porcelain)" ]]; then
	if [[ "$DRY_RUN" -eq 1 ]]; then
		log WARNING "the working tree is not clean (allowed in --dry-run)"
		git status --short | sed 's/^/release.bash:   /' >&2
	else
		die "the working tree is not clean (commit or stash first):"$'\n'"$(git status --short)"
	fi
fi

if ! gh auth status >/dev/null 2>&1; then
	if [[ "$DRY_RUN" -eq 1 ]]; then
		log WARNING "gh is not authenticated (allowed in --dry-run)"
	else
		die "gh is not authenticated (run: gh auth login)"
	fi
fi

# Best effort fetch, so the check for an existing tag is up to date.
if ! git fetch origin --tags >/dev/null 2>&1; then
	if [[ "$DRY_RUN" -eq 1 ]]; then
		log WARNING "git fetch origin --tags failed, the remote tag check may be stale"
	else
		die "git fetch origin --tags failed"
	fi
fi

# --- Version and tag (computed by versioner) ---------------------------------

if [[ -z "${VERSIONER_BIN:-}" || ! -x "${VERSIONER_BIN:-}" ]]; then
	die "no versioner binary found (run tools/versioner/update.bash or set VERSIONER_BIN)"
fi
log INFO "using versioner: $VERSIONER_BIN"

VERSION_OUTPUT="$("$SCRIPT_DIR/scripts/version.bash" --format text)" || die "could not compute the version"
VERSION="$(printf '%s\n' "$VERSION_OUTPUT" | sed -n 's/^VERSION=//p' | tail -1)"
TAG="$(printf '%s\n' "$VERSION_OUTPUT" | sed -n 's/^TAG=//p' | tail -1)"
[[ -n "$VERSION" ]] || die "could not parse VERSION from the versioner output"
[[ -n "$TAG" ]] || die "could not parse TAG from the versioner output"
if ! [[ "$TAG" =~ ^v[0-9]+(\.[0-9]+)+$ ]]; then
	die "the computed tag looks invalid: '$TAG' (expected vX.Y.Z...)"
fi
log INFO "VERSION=$VERSION  TAG=$TAG"

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; then
	die "the tag $TAG already exists locally (make a new commit to bump the version, then re-run)"
fi
# git ls-remote exits with 2 when no matching ref was found and with 128
# when the remote could not be reached.
remote_status=0
git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1 || remote_status=$?
case "$remote_status" in
0)
	die "the tag $TAG already exists on origin (make a new commit to bump the version, then re-run)"
	;;
2) ;;
*)
	log WARNING "could not check the tags on origin (git ls-remote failed)"
	;;
esac

# --- Plan --------------------------------------------------------------------

if [[ "$DRY_RUN" -eq 1 ]]; then
	log INFO "dry run: would package the following archives into $RELEASE_DIR"
	for platform in "${PLATFORMS[@]}"; do
		if [[ -f "$OUTPUT_DIR/$platform/$APP_NAME" || -f "$OUTPUT_DIR/$platform/$APP_NAME.exe" ]]; then
			log INFO "  ${APP_NAME}_${VERSION}_$platform.zip (binary present)"
		else
			log INFO "  ${APP_NAME}_${VERSION}_$platform.zip (binary missing, it would be built)"
		fi
	done
	log INFO "dry run: would run:"
	log INFO "  $SCRIPT_DIR/build.bash --all --release"
	log INFO "  git tag -a $TAG -m \"$APP_NAME $TAG\""
	log INFO "  git push origin $TAG"
	log INFO "  gh release create $TAG --title \"$APP_NAME $TAG\" --verify-tag --generate-notes <archives>"
	log INFO "dry run complete: VERSION=$VERSION TAG=$TAG"
	exit 0
fi

# --- Build -------------------------------------------------------------------

if [[ "$SKIP_BUILD" -eq 0 ]]; then
	log INFO "building every platform in release mode..."
	"$SCRIPT_DIR/build.bash" --all --release || die "the build failed"
else
	log INFO "skipping the build, using the binaries in $OUTPUT_DIR"
fi

# --- Packaging ---------------------------------------------------------------

[[ -f "$REPO_ROOT/LICENSE" ]] || die "the LICENSE file is missing"
[[ -f "$REPO_ROOT/README.md" ]] || die "the README.md file is missing"

rm -rf -- "$RELEASE_DIR"
mkdir -p -- "$RELEASE_DIR"

ASSETS=()
for platform in "${PLATFORMS[@]}"; do
	binary="$OUTPUT_DIR/$platform/$APP_NAME"
	[[ -f "$binary" ]] || binary="$OUTPUT_DIR/$platform/$APP_NAME.exe"
	[[ -f "$binary" ]] || die "no binary for $platform in $OUTPUT_DIR (run ./build.bash --all)"

	staging="$RELEASE_DIR/.staging/$platform"
	mkdir -p -- "$staging"
	cp -p -- "$binary" "$staging/"
	cp -p -- "$REPO_ROOT/LICENSE" "$staging/LICENSE"
	cp -p -- "$REPO_ROOT/README.md" "$staging/README.md"

	archive="$RELEASE_DIR/${APP_NAME}_${VERSION}_$platform.zip"
	rm -f -- "$archive"
	log INFO "packaging $archive"
	(
		cd "$staging"
		files=(LICENSE README.md "$(basename "$binary")")
		if [[ "$ZIP_TOOL" == "zip" ]]; then
			zip -q -j "$archive" "${files[@]}"
		else
			"$ZIP_TOOL" a -tzip -mx=9 "$archive" "${files[@]}" >/dev/null
		fi
	) || die "could not create $archive"
	[[ -f "$archive" ]] || die "could not create $archive"
	ASSETS+=("$archive")
done
rm -rf -- "$RELEASE_DIR/.staging"

# --- Checksums ---------------------------------------------------------------

CHECKSUMS="$RELEASE_DIR/checksums.txt"
: >"$CHECKSUMS"
for asset in "${ASSETS[@]}"; do
	if command -v shasum >/dev/null 2>&1; then
		(cd "$RELEASE_DIR" && shasum -a 256 "$(basename "$asset")") >>"$CHECKSUMS"
	elif command -v sha256sum >/dev/null 2>&1; then
		(cd "$RELEASE_DIR" && sha256sum "$(basename "$asset")") >>"$CHECKSUMS"
	else
		die "neither shasum nor sha256sum is available"
	fi
done
ASSETS+=("$CHECKSUMS")
log INFO "checksums written to $CHECKSUMS"

if [[ "$PUBLISH" -eq 0 ]]; then
	log INFO "the archives are ready in $RELEASE_DIR:"
	for asset in "${ASSETS[@]}"; do
		log INFO "  $(basename "$asset")"
	done
	log INFO "package only: nothing was tagged, pushed or uploaded"
	exit 0
fi

# --- Tag ---------------------------------------------------------------------

log INFO "creating the annotated tag $TAG..."
git tag -a "$TAG" -m "$APP_NAME $TAG"

log INFO "pushing the tag $TAG..."
git push origin "$TAG"

# --- GitHub release ----------------------------------------------------------

if gh release view "$TAG" >/dev/null 2>&1; then
	die "the GitHub release $TAG already exists (the tag was pushed)"
fi

log INFO "creating the GitHub release $TAG..."
gh release create "$TAG" \
	--title "$APP_NAME $TAG" \
	--verify-tag \
	--generate-notes \
	--notes "Cross compiled binaries of $APP_NAME $VERSION for every supported platform. Each archive contains the binary, the LICENSE and the README.md." \
	"${ASSETS[@]}"

if REPO_SLUG="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)" && [[ -n "$REPO_SLUG" ]]; then
	log INFO "release: https://github.com/$REPO_SLUG/releases/tag/$TAG"
else
	log INFO "the release $TAG was created (see GitHub Releases)"
fi
