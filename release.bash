#!/usr/bin/env bash
set -euo pipefail

# ======================================================================
# release.bash — build, archive and publish mirror-clone on GitHub.
# ======================================================================
#
# The release version comes from the bundled Versioner, every release
# platform is cross-compiled with the version stamped into the binary
# (make cross), each platform is packed into its own .7z with the
# bundled 7-Zip, and all messages are printed by the bundled Logamuffin.
# scripts/env.sh resolves those binaries (newest bundle in tools/,
# falling back to PATH); scripts/entry.sh provides the log() helper.
#
# Usage:
#   ./release.bash           # tag + push + GitHub release
#   ./release.bash --dry-run # build + version only, touch nothing remote
#
# Prerequisites: go, git, make, and (for the real release) gh
# authenticated via `gh auth login`. Working tree must be clean and the
# current branch pushed to origin.
# ======================================================================

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# --- Usage --- #
usage() {
    cat <<'EOF'
Usage: ./release.bash [--dry-run]

Cross-compile mirror-clone for every release platform, pack each platform
into MirrorClone_<tag>_<platform>.7z (binary + README.md + LICENSE), create
an annotated tag, push it, and create a GitHub release with the .7z
archives + checksums.

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

for cmd in git go make; do need_cmd "$cmd"; done
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
        git status --short | while IFS= read -r line; do log WARNING "  $line"; done
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
if ! [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    die "computed tag looks invalid: '$TAG' (expected vX.Y.Z.W.V)"
fi
log INFO "VERSION=$VERSION  TAG=$TAG"

# --- Tag checks --- #
if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists locally"
fi
if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists on origin"
fi

# --- Cross-compile release binaries --- #
log INFO "cross-compiling release binaries (VERSION=$TAG) via make cross..."
VERSION="$TAG" make cross || die "make cross failed"

# The release platforms live in the Makefile; ask it for the list so the
# script and the build stay in sync.
PLATFORMS=""
while IFS= read -r platform; do
    if [[ -n "$platform" ]]; then
        PLATFORMS="$PLATFORMS $platform"
    fi
done < <(make -s platforms)
[[ -n "$PLATFORMS" ]] || die "make platforms returned no platforms"

for doc in README.md LICENSE; do
    [[ -f "$doc" ]] || die "missing $doc (needed for the .7z archives)"
done

# --- Pack each platform into its own .7z --- #
log INFO "packing .7z archives with 7-Zip..."
ARTIFACT_DIR="$REPO_ROOT/dist"
ARCHIVES=()
for platform in $PLATFORMS; do
    dir="$ARTIFACT_DIR/$platform"
    [[ -d "$dir" ]] || die "no build output for platform $platform (expected $dir)"

    bin=""
    for cand in "$dir/mirror-clone" "$dir/mirror-clone.exe"; do
        if [[ -f "$cand" ]]; then
            bin="$cand"
        fi
    done
    [[ -n "$bin" ]] || die "no mirror-clone binary in $dir"
    [[ -s "$bin" ]] || die "built binary is empty: $bin"
    bin_name="$(basename "$bin")"

    # Stage the user docs next to the binary so they land at the
    # archive root.
    for doc in README.md LICENSE; do
        cp -p "$doc" "$dir/"
    done

    archive="$ARTIFACT_DIR/MirrorClone_${TAG}_${platform}.7z"
    rm -f "$archive"
    ( cd "$dir" && "$SEVENZZ_BIN" a -t7z "$archive" "$bin_name" README.md LICENSE >/dev/null ) ||
        die "7-Zip failed to create $(basename "$archive")"
    [[ -s "$archive" ]] || die "failed to create $archive"
    log INFO "  $(basename "$archive")"
    ARCHIVES+=("$archive")
done

# The native binary must report the release tag (guards the ldflags
# wiring in the Makefile).
NATIVE_PLATFORM="${RECIPE_SERVER_PLATFORM:-}"
NATIVE_BIN=""
if [[ -n "$NATIVE_PLATFORM" ]]; then
    if [[ "$(go env GOOS)" == "windows" ]]; then
        NATIVE_BIN="$ARTIFACT_DIR/$NATIVE_PLATFORM/mirror-clone.exe"
    else
        NATIVE_BIN="$ARTIFACT_DIR/$NATIVE_PLATFORM/mirror-clone"
    fi
fi
if [[ -n "$NATIVE_BIN" && -x "$NATIVE_BIN" ]]; then
    NATIVE_VER="$("$NATIVE_BIN" --version)"
    log INFO "native binary ($NATIVE_PLATFORM) reports: $NATIVE_VER"
    case "$NATIVE_VER" in
        *"$TAG"*) ;;
        *) die "native binary version '$NATIVE_VER' does not contain $TAG (ldflags missing?)" ;;
    esac
fi

# --- Generate checksums --- #
log INFO "generating checksums..."
CHECKSUM_FILE="$ARTIFACT_DIR/checksums.txt"
rm -f "$CHECKSUM_FILE"
ARCHIVE_NAMES=()
for archive in "${ARCHIVES[@]}"; do
    ARCHIVE_NAMES+=("$(basename "$archive")")
done
if command -v shasum >/dev/null 2>&1; then
    ( cd "$ARTIFACT_DIR" && shasum -a 256 "${ARCHIVE_NAMES[@]}" > checksums.txt )
elif command -v sha256sum >/dev/null 2>&1; then
    ( cd "$ARTIFACT_DIR" && sha256sum "${ARCHIVE_NAMES[@]}" > checksums.txt )
else
    # Fall back to 7-Zip's hash command.
    : > "$CHECKSUM_FILE"
    for archive in "${ARCHIVES[@]}"; do
        name="$(basename "$archive")"
        hash="$("$SEVENZZ_BIN" h -scrcSHA256 "$archive" |
            sed -n 's/^SHA256 for data:[[:space:]]*//p' | tail -n 1)"
        [[ -n "$hash" ]] || die "could not compute checksum for $name"
        printf '%s  %s\n' "$hash" "$name" >> "$CHECKSUM_FILE"
    done
fi
[[ -s "$CHECKSUM_FILE" ]] || die "failed to write $CHECKSUM_FILE"
while IFS= read -r line; do
    log INFO "checksum: $line"
done < "$CHECKSUM_FILE"

# --- Tag + GitHub release --- #
if [[ "$DRY_RUN" -eq 1 ]]; then
    log INFO "DRY RUN: not tagging, pushing or creating a release"
    log INFO "would publish ${#ARCHIVES[@]} archive(s) for version $VERSION"
    exit 0
fi

log INFO "creating annotated tag $TAG..."
git tag -a "$TAG" -m "mirror-clone $TAG"

log INFO "pushing tag $TAG..."
git push origin "$TAG"

if gh release view "$TAG" >/dev/null 2>&1; then
    die "release $TAG exists (upload the assets manually with: gh release upload $TAG ${ARCHIVE_NAMES[*]} --clobber)"
fi

log INFO "creating GitHub release $TAG..."
gh release create "$TAG" \
    --title "mirror-clone $TAG" \
    --generate-notes "${ARCHIVES[@]}" "dist/checksums.txt"

REPO_SLUG="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || true)"
if [[ -n "$REPO_SLUG" ]]; then
    log INFO "release: https://github.com/$REPO_SLUG/releases/tag/$TAG"
else
    log INFO "release $TAG created"
fi
