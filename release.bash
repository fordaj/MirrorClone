#!/usr/bin/env bash
set -euo pipefail

# ======================================================================
# release.bash — dogfooding release for mirror-clone.
# ======================================================================
#
# Builds and releases mirror-clone cross-platform .exe binaries.
#
# Usage:
#   ./release.bash           # tag + push + GitHub release
#   ./release.bash --dry-run # compute + build only, touch nothing remote
#
# Prerequisites: go, git, 7z (brew install p7zip / apt install
# p7zip-full), and (for the real release) gh authenticated via
# `gh auth login`. Working tree must be clean; HEAD should be pushed.
# ======================================================================

# --- Source environment ---
(return 0 2>/dev/null) && sourced=true || sourced=false
if ! "${sourced}"; then
    # When executed directly, define our own log/die
    log()  { printf 'release.bash: %s\n' "$*" >&2; }
    die()  { printf 'release.bash: error: %s\n' "$*" >&2; exit 1; }
    # Source env.sh for platform variables
    source scripts/env.sh
else
    # When sourced, define minimal functions
    log()  { printf 'release.bash: %s\n' "$*" >&2; }
    die()  { printf 'release.bash: error: %s\n' "$*" >&2; exit 1; }
fi

# --- Args --- #
DRY_RUN=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run|-n) DRY_RUN=1; shift ;;
        --help|-h) usage; exit 0 ;;
        *) echo "release.bash: unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

# --- Verify prerequisites --- #
need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

for cmd in git go; do need_cmd "$cmd"; done
[[ "$DRY_RUN" -ne 1 ]] && { need_cmd gh; gh auth status >/dev/null 2>&1 || die "gh not authenticated"; }

# --- Repository root --- #
REPO_ROOT="$(git rev-parse --show-toplevel)" || die "not inside git repo"
cd "$REPO_ROOT"

RANCH="$(git rev-parse --abbrev-ref HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
log "repo: $REPO_ROOT"
log "branch: $RANCH  HEAD: $HEAD_SHORT"
[[ "$DRY_RUN" -eq 1 ]] && { log "DRY RUN: no tag/push/release"; exit 0; }

# --- Clean working tree --- #
if [[ -n "$(git status --porcelain)" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
        log "warning: working tree not clean (allowed in --dry-run)"
    else
        die "working tree not clean (commit or stash first)"
    fi
fi

git fetch origin --tags 2>/dev/null || die "git fetch failed"

# --- Build mirror-clone binary --- #
log "building mirror-clone binary..."
BIN_NAME="mirror-clone"
[[ "$(go env GOOS)" == "windows" ]] && BIN_NAME="mirror-clone.exe"
go build -o "dist/$BIN_NAME" .
chmod +x "dist/$BIN_NAME"
for doc in README.md LICENSE mirror-clone.example.json; do
    [[ -f "$doc" ]] || die "missing $doc"
    cp -p "$doc" "dist/"
done

# Compute version from git tags
log "computing version..."
GIT_DESC="$(git describe --tags --abbrev=5 2>/dev/null || echo "0.1.0.0.0")"
[[ "$GIT_DESC" != v* ]] && GIT_DESC="v$GIT_DESC"
VERSION="$GIT_DESC"; TAG="$GIT_DESC"
log "VERSION=$VERSION  TAG=$TAG"

# --- Tag checks --- #
if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists locally"
fi
if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
    die "tag $TAG exists on origin"
fi

# --- Create .7z archive --- #
log "creating .7z archive..."
FULL_ARCHIVE="${PWD}/dist/mirror-clone_${TAG}_${BIN_NAME%.exe}.7z"
rm -rf dist/dist 2>/dev/null; rm -f "$FULL_ARCHIVE"
7z a -t7z "$FULL_ARCHIVE" mirror-clone README.md LICENSE mirror-clone.example.json
[[ -f "$FULL_ARCHIVE" ]] || die "failed to create $FULL_ARCHIVE"
log "created $FULL_ARCHIVE"

# --- Generate checksums --- #
log "generating checksums..."
if command -v shasum >/dev/null 2>&1; then
    (cd dist && shasum -a 256 mirror-clone_*.7z > checksums.txt)
elif command -v sha256sum >/dev/null 2>&1; then
    (cd dist && sha256sum mirror-clone_*.7z > checksums.txt)
else
    die "neither shasum nor sha256sum found"
fi
cat dist/checksums.txt | sed 's/^/release.bash:   /'

# --- Tag + GitHub release --- #
[[ "$DRY_RUN" -eq 1 ]] && { log "DRY RUN would tag/push/release"; exit 0; }

log "creating annotated tag $TAG..."
git tag -a "$TAG" -m "mirror-clone $TAG"

log "pushing tag $TAG..."
git push origin "$TAG"

if gh release view "$TAG" >/dev/null 2>&1; then
    die "release $TAG exists"
fi

log "creating GitHub release $TAG..."
gh release create "$TAG" \
    --title "mirror-clone $TAG" \
    --generate-notes "dist/mirror-clone_${TAG}_${BIN_NAME%.exe}.7z" "dist/checksums.txt"

[[ "$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)" ]] && \
    log "release: https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/releases/tag/$TAG" || \
    log "release $TAG created"

# --- Usage --- #
usage() {
    cat <<'EOF'
Usage: ./release.bash [--dry-run]

Build and release mirror-clone cross-platform .exe binary.

Flags:
  --dry-run, -n   Build + compute version, but do not tag/push/release.
  --help, -h      Show this help.

Env:
  MIRRORCLONE_BIN   Path to a prebuilt mirror-clone used for version computation
                    (default: built fresh from source).
EOF
}
