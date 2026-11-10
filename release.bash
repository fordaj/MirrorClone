#!/usr/bin/env bash
set -euo pipefail

# ======================================================================
# release.bash — build, archive and publish mirror-clone on GitHub.
# ======================================================================
#
# The release version comes from the bundled Versioner, the archive is
# created with the bundled 7-Zip and every message is printed by the
# bundled Logamuffin. scripts/env.sh resolves those binaries (newest
# bundle in tools/, falling back to PATH); scripts/entry.sh provides the
# log() helper.
#
# Usage:
#   ./release.bash           # tag + push + GitHub release
#   ./release.bash --dry-run # build + version only, touch nothing remote
#
# Prerequisites: go, git, and (for the real release) gh authenticated via
# `gh auth login`. Working tree must be clean and the current branch
# pushed to origin.
# ======================================================================

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# --- Usage --- #
usage() {
    cat <<'EOF'
Usage: ./release.bash [--dry-run]

Build and release mirror-clone to GitHub.

Flags:
  --dry-run, -n   Build + compute the version, but do not tag/push/release.
  --help, -h      Show this help.

Env:
  VERSIONER_CONFIG_FILE   Versioner config file (default: .versioner.json).
                           Only passed to versioner when the file exists.
EOF
}

# --- Repository root --- #
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)" || {
    printf 'release.bash: error: not inside a git repository\n' >&2
    exit 1
}
cd "$REPO_ROOT"

# entry.sh defines log() (backed by Logamuffin) and sources env.sh,
# which resolves the bundled tool binaries.
source "$REPO_ROOT/scripts/entry.sh"

die() {
    log ERROR "$*"
    exit 1
}

# --- Args --- #
DRY_RUN=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run|-n) DRY_RUN=1; shift ;;
        --help|-h) usage; exit 0 ;;
        *) log ERROR "unknown argument: $1"; usage >&2; exit 2 ;;
    esac
done

# --- Verify prerequisites --- #
need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

need_tool() {
    local bin="$1" name="$2"
    [[ -n "$bin" && -x "$bin" ]] || die "$name binary not found or not executable: ${bin:-<unset>} (expected under ${TOOLS_DIR:-tools}/)"
}

for cmd in git go; do need_cmd "$cmd"; done
if [[ "$DRY_RUN" -ne 1 ]]; then
    need_cmd gh
    gh auth status >/dev/null 2>&1 || die "gh not authenticated (run: gh auth login)"
fi

need_tool "${VERSIONER_BIN:-}" "versioner"
need_tool "${LOGAMUFFIN_BIN:-}" "logamuffin"
need_tool "${SEVENZZ_BIN:-}" "7-Zip (7zz)"

# --- Repository state --- #
RANCH="$(git rev-parse --abbrev-ref HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
log INFO "repo: $REPO_ROOT"
log INFO "branch: $RANCH  HEAD: $HEAD_SHORT"

if [[ -n "$(git status --porcelain)" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
        log WARNING "working tree not clean (allowed in --dry-run)"
    else
        die "working tree not clean (commit or stash first)"
    fi
fi

git fetch origin --tags 2>/dev/null || die "git fetch failed"

if [[ "$RANCH" != "HEAD" ]]; then
    git rev-parse --verify --quiet "refs/remotes/origin/$RANCH" >/dev/null ||
        die "branch $RANCH does not exist on origin (push it first)"
    [[ "$(git rev-list --count "origin/$RANCH..HEAD")" == "0" ]] ||
        die "branch $RANCH has unpushed commits (run: git push origin $RANCH)"
fi

# --- Compute version with versioner --- #
log INFO "computing version with versioner..."
VERSIONER_ARGS=(--format text --quiet)
if [[ -n "${VERSIONER_CONFIG_FILE:-}" && -f "${VERSIONER_CONFIG_FILE}" ]]; then
    VERSIONER_ARGS+=(--config "$VERSIONER_CONFIG_FILE")
fi
VERSIONER_OUT="$("$VERSIONER_BIN" "${VERSIONER_ARGS[@]}")" || die "versioner failed"
VERSION="$(printf '%s\n' "$VERSIONER_OUT" | sed -n 's/^VERSION=//p' | tail -n 1)"
TAG="$(printf '%s\n' "$VERSIONER_OUT" | sed -n 's/^TAG=//p' | tail -n 1)"
[[ -n "$VERSION" ]] || die "could not parse VERSION from versioner output"
[[ -n "$TAG" ]] || die "could not parse TAG from versioner output"
log INFO "VERSION=$VERSION  TAG=$TAG"

# --- Build mirror-clone binary --- #
log INFO "building mirror-clone binary..."
BIN_NAME="mirror-clone"
if [[ "$(go env GOOS)" == "windows" ]]; then
    BIN_NAME="mirror-clone.exe"
fi

STAGE_DIR="$REPO_ROOT/temp/release"
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"
go build -o "$STAGE_DIR/$BIN_NAME" . || die "go build failed"
chmod +x "$STAGE_DIR/$BIN_NAME"
for doc in README.md LICENSE; do
    [[ -f "$doc" ]] || die "missing $doc"
    cp -p "$doc" "$STAGE_DIR/"
done

# --- Create .7z archive --- #
ARTIFACT_DIR="$REPO_ROOT/dist"
ARCHIVE_NAME="mirror-clone_${TAG}_${BIN_NAME%.exe}.7z"
ARCHIVE_PATH="$ARTIFACT_DIR/$ARCHIVE_NAME"
mkdir -p "$ARTIFACT_DIR"
rm -f "$ARCHIVE_PATH"
log INFO "creating $ARCHIVE_NAME with 7-Zip..."
( cd "$STAGE_DIR" && "$SEVENZZ_BIN" a -t7z "$ARCHIVE_PATH" "$BIN_NAME" README.md LICENSE ) ||
    die "7-Zip failed to create $ARCHIVE_NAME"
[[ -f "$ARCHIVE_PATH" ]] || die "failed to create $ARCHIVE_PATH"
log INFO "created $ARCHIVE_NAME"

# --- Generate checksums --- #
log INFO "generating checksums..."
CHECKSUM_FILE="$ARTIFACT_DIR/checksums.txt"
rm -f "$CHECKSUM_FILE"
if command -v shasum >/dev/null 2>&1; then
    ( cd "$ARTIFACT_DIR" && shasum -a 256 "$ARCHIVE_NAME" > checksums.txt )
elif command -v sha256sum >/dev/null 2>&1; then
    ( cd "$ARTIFACT_DIR" && sha256sum "$ARCHIVE_NAME" > checksums.txt )
else
    # Fall back to 7-Zip's hash command.
    ARCHIVE_HASH="$("$SEVENZZ_BIN" h -scrcSHA256 "$ARCHIVE_PATH" |
        sed -n 's/^SHA256 for data:[[:space:]]*//p' | tail -n 1)"
    [[ -n "$ARCHIVE_HASH" ]] || die "could not compute checksum for $ARCHIVE_NAME"
    ( cd "$ARTIFACT_DIR" && printf '%s  %s\n' "$ARCHIVE_HASH" "$ARCHIVE_NAME" > checksums.txt )
fi
[[ -s "$CHECKSUM_FILE" ]] || die "failed to write $CHECKSUM_FILE"
while IFS= read -r line; do
    log INFO "checksum: $line"
done < "$CHECKSUM_FILE"

# --- Tag checks --- #
if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists locally"
fi
if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists on origin"
fi

# --- Tag + GitHub release --- #
if [[ "$DRY_RUN" -eq 1 ]]; then
    log INFO "DRY RUN: not tagging, pushing or creating a release"
    log INFO "would publish $ARCHIVE_NAME (version $VERSION)"
    exit 0
fi

log INFO "creating annotated tag $TAG..."
git tag -a "$TAG" -m "mirror-clone $TAG"

log INFO "pushing tag $TAG..."
git push origin "$TAG"

if gh release view "$TAG" >/dev/null 2>&1; then
    die "release $TAG exists"
fi

log INFO "creating GitHub release $TAG..."
gh release create "$TAG" \
    --title "mirror-clone $TAG" \
    --generate-notes "dist/$ARCHIVE_NAME" "dist/checksums.txt"

REPO_SLUG="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || true)"
if [[ -n "$REPO_SLUG" ]]; then
    log INFO "release: https://github.com/$REPO_SLUG/releases/tag/$TAG"
else
    log INFO "release $TAG created"
fi
