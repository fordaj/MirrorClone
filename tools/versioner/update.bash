#!/usr/bin/env bash
set -euo pipefail

# --- Environment -------------------------------------------------------------

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"

(return 0 2>/dev/null) || source "$PROJECT_ROOT/scripts/entry.sh"

DRY_RUN=0
REPO="${VERSIONER_REPO:-fordaj/Versioner}"
TAG="${VERSIONER_TAG:-}"

usage() {
    cat <<'EOF'
Usage: tools/versioner/update.bash [--dry-run] [--tag TAG] [--repo REPO]

Download the latest Versioner release from GitHub via gh, extract each
platform binary into tools/versioner/<tag>/<platform>/ (platform is the
bundled <os>-<arch> layout, e.g. darwin-amd64 -> macos-x86_64), and delete old
version folders. Only the binary is kept.

Flags:
  --dry-run, -n   Resolve tag + list planned assets, change nothing.
  --tag TAG       Pin a specific release tag (default: latest release).
  --repo REPO     Repository slug (default: fordaj/Versioner).
  --help, -h      Show this help.

Env:
  VERSIONER_REPO  Repository slug (overridden by --repo).
  VERSIONER_TAG   Pinned tag (overridden by --tag).
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
    --dry-run | -n)
        DRY_RUN=1
        shift
        ;;
    --help | -h)
        usage
        exit 0
        ;;
    --tag)
        [[ $# -ge 2 ]] || {
            log ERROR "--tag requires a value"
            usage >&2
            exit 2
        }
        TAG="$2"
        shift 2
        ;;
    --tag=*)
        TAG="${1#--tag=}"
        shift
        ;;
    --repo)
        [[ $# -ge 2 ]] || {
            log ERROR "--repo requires a value"
            usage >&2
            exit 2
        }
        REPO="$2"
        shift 2
        ;;
    --repo=*)
        REPO="${1#--repo=}"
        shift
        ;;
    *)
        log ERROR "unknown argument: $1"
        usage >&2
        exit 2
        ;;
    esac
done

[[ -n "$REPO" ]] || {
    log ERROR "repository must not be empty"
    exit 2
}

die() {
    log ERROR "$*" || true
    exit 1
}
need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1 (https://cli.github.com/)"
}

need_cmd gh

if [[ "$DRY_RUN" -ne 1 ]]; then
    if command -v 7zz >/dev/null 2>&1; then
        SEVENZIP=7zz
    elif command -v 7z >/dev/null 2>&1; then
        SEVENZIP=7z
    else
        die "required command not found: 7z/7zz (brew install p7zip / apt-get install p7zip-full)"
    fi
fi

VERSIONER_ROOT="$SCRIPT_DIR"
[[ -d "$VERSIONER_ROOT" ]] || die "versioner bundle directory not found: $VERSIONER_ROOT"
log INFO "versioner bundle directory: $VERSIONER_ROOT"
log INFO "repository: $REPO"

if [[ -z "$TAG" ]]; then
    TAG="$(gh release view --repo "$REPO" --json tagName -q .tagName)" ||
        die "could not determine latest release from $REPO (is gh authenticated and the repo visible?)"
fi
[[ -n "$TAG" ]] || die "could not determine release tag from $REPO"
if ! [[ "$TAG" =~ ^v[0-9]+(\.[0-9]+)+$ ]]; then
    die "invalid tag '$TAG' (expected vX.Y.Z...)"
fi
log INFO "release tag: $TAG"

# parse_stem_to_platform <stem> sets PLATFORM (e.g.
# Versioner_v0.0.3.0.5_darwin-arm64 -> macos-arm64) or returns 1 for
# unknown values. Sets globals to avoid subshell-die pitfalls. Asset
# names append _<os>-<arch> after the tag, and arch may itself contain
# "_" (x86_64), so the suffix is matched as a whole instead of splitting
# on the last "_" (which would yield "x86_64" alone).
parse_stem_to_platform() {
    local stem="$1"
    case "$stem" in
    *_macos-x86_64 | *_darwin-amd64) PLATFORM="macos-x86_64" ;;
    *_macos-arm64 | *_darwin-arm64) PLATFORM="macos-arm64" ;;
    *_linux-x86_64 | *_linux-amd64) PLATFORM="linux-x86_64" ;;
    *_linux-arm64) PLATFORM="linux-arm64" ;;
    *_windows-x86_64 | *_windows-amd64) PLATFORM="windows-x86_64" ;;
    *_windows-arm64) PLATFORM="windows-arm64" ;;
    *) return 1 ;;
    esac
}

if [[ "$DRY_RUN" -eq 1 ]]; then
    log INFO "DRY RUN: no download, extraction, or deletion will happen"
    log INFO "release $TAG assets:"
    assets_list=$(gh release view "$TAG" --repo "$REPO" --json assets -q '.assets[].name' ||
        die "could not list assets for $TAG in $REPO")
    [[ -n "$assets_list" ]] || die "no assets found for $TAG in $REPO"
    asset_count=0
    while IFS= read -r asset; do
        [[ -n "$asset" ]] || continue
        case "$asset" in
        *.7z)
            asset_count=$((asset_count + 1))
            stem="${asset%.7z}"
            if parse_stem_to_platform "$stem"; then
                log INFO "  $asset -> $TAG/$PLATFORM"
            else
                log INFO "  $asset -> unknown platform (would fail)"
            fi
            ;;
        checksums.txt)
            log INFO "  $asset (checksums)"
            ;;
        *)
            log INFO "  $asset (ignored)"
            ;;
        esac
    done <<<"$assets_list"
    ((asset_count > 0)) || die "no .7z assets found for $TAG in $REPO"
    shopt -s nullglob
    old_found=0
    for dir in "$VERSIONER_ROOT"/v*/; do
        [[ -d "$dir" ]] || continue
        dir_name="$(basename "$dir")"
        if [[ "$dir_name" != "$TAG" ]]; then
            log INFO "would remove old $dir_name"
            old_found=1
        fi
    done
    if [[ "$old_found" -eq 0 ]]; then
        log INFO "no old version folders to remove"
    fi
    log INFO "dry run complete: would update $VERSIONER_ROOT/$TAG"
    exit 0
fi

# --- Real update: download to a temp dir, stage the bundle, then swap. ---
TMP_DIR=""
STAGE_DIR=""
cleanup() {
    if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
        rm -rf -- "$TMP_DIR"
    fi
    if [[ -n "${STAGE_DIR:-}" && -d "$STAGE_DIR" ]]; then
        rm -rf -- "$STAGE_DIR"
    fi
}
trap cleanup EXIT

TMP_DIR="$(mktemp -d)"
STAGE_DIR="$(mktemp -d "$VERSIONER_ROOT/.tmp.XXXXXX")"

log INFO "downloading $TAG assets from $REPO..."
gh release download "$TAG" --repo "$REPO" --dir "$TMP_DIR" --pattern '*.7z' --pattern 'checksums.txt' --clobber

shopt -s nullglob
archives=("$TMP_DIR"/*.7z)
((${#archives[@]} > 0)) || die "no .7z assets downloaded from $REPO $TAG"
log INFO "downloaded ${#archives[@]} .7z archives"

if [[ -f "$TMP_DIR/checksums.txt" ]]; then
    log INFO "verifying checksums..."
    if command -v shasum >/dev/null 2>&1; then
        (cd "$TMP_DIR" && shasum -a 256 -c checksums.txt) || die "checksum verification failed for $TAG"
    elif command -v sha256sum >/dev/null 2>&1; then
        (cd "$TMP_DIR" && sha256sum -c checksums.txt) || die "checksum verification failed for $TAG"
    else
        die "checksums.txt present but neither shasum nor sha256sum found"
    fi
    log INFO "checksums verified"
else
    log WARNING "no checksums.txt in $TAG release; skipping verification"
fi

for archive in "${archives[@]}"; do
    base="$(basename "$archive")"
    stem="${base%.7z}"
    if ! parse_stem_to_platform "$stem"; then
        die "unknown platform in asset '$base' (expected <os>-<arch> such as darwin-arm64)"
    fi
    if [[ -e "$STAGE_DIR/$PLATFORM" ]]; then
        die "duplicate asset for platform $PLATFORM (from $base)"
    fi
    log INFO "extracting $base -> $TAG/$PLATFORM"
    extract_dir="$TMP_DIR/extract-$PLATFORM"
    mkdir -p "$extract_dir"
    "$SEVENZIP" x -y "$archive" -o"$extract_dir" >/dev/null
    bin_src=""
    for cand in "$extract_dir/versioner" "$extract_dir/versioner.exe"; do
        if [[ -f "$cand" ]]; then
            bin_src="$cand"
        fi
    done
    [[ -n "$bin_src" ]] || die "no versioner binary in $base (expected versioner or versioner.exe at archive root)"
    bin_name="$(basename "$bin_src")"
    target_dir="$STAGE_DIR/$PLATFORM"
    mkdir -p "$target_dir"
    cp -p "$bin_src" "$target_dir/$bin_name"
    chmod +x "$target_dir/$bin_name"
    [[ -s "$target_dir/$bin_name" ]] || die "installed binary is empty: $target_dir/$bin_name"
    log INFO "installed $PLATFORM/$bin_name (from $base)"
    rm -rf -- "$extract_dir"
done

TAG_DIR="$VERSIONER_ROOT/$TAG"
rm -rf -- "$TAG_DIR"
mkdir -p "$VERSIONER_ROOT"
mv "$STAGE_DIR" "$TAG_DIR"
STAGE_DIR=""

for dir in "$VERSIONER_ROOT"/v*/; do
    [[ -d "$dir" ]] || continue
    dir_name="$(basename "$dir")"
    if [[ "$dir_name" != "$TAG" ]]; then
        log INFO "removing old $dir_name"
        rm -rf -- "$dir"
    fi
done

# Best-effort host verification: run the bundled binary for this machine.
host_os_raw="$(uname -s | tr '[:upper:]' '[:lower:]')"
host_arch_raw="$(uname -m | tr '[:upper:]' '[:lower:]')"
case "$host_os_raw" in
darwin*) host_os="macos" ;;
linux*) host_os="linux" ;;
msys* | mingw* | cygwin* | windows* | win*) host_os="windows" ;;
*) host_os="$host_os_raw" ;;
esac
case "$host_arch_raw" in
x86_64 | amd64) host_arch="x86_64" ;;
arm64 | aarch64) host_arch="arm64" ;;
*) host_arch="$host_arch_raw" ;;
esac
host_platform="$host_os-$host_arch"
host_bin=""
for cand in "$TAG_DIR/$host_platform/versioner" "$TAG_DIR/$host_platform/versioner.exe"; do
    if [[ -x "$cand" ]]; then
        host_bin="$cand"
    fi
done
if [[ -n "$host_bin" ]]; then
    host_ver="$("$host_bin" --version 2>&1 || true)"
    log INFO "host binary $host_platform reports: $host_ver"
    case "$host_ver" in
    *"$TAG"*) ;;
    *) log WARNING "host binary version '$host_ver' does not contain $TAG" ;;
    esac
else
    log WARNING "host platform $host_platform not in bundle; skipped version check"
fi

log INFO "done: $TAG_DIR now contains:"
for dir in "$TAG_DIR"/*/; do
    [[ -d "$dir" ]] || continue
    log INFO "  $(basename "$dir")"
done
