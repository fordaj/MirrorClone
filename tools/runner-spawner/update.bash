#!/usr/bin/env bash
set -euo pipefail

# --- Environment -------------------------------------------------------------

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"

(return 0 2>/dev/null) || source "$PROJECT_ROOT/scripts/entry.sh"

REPO="${RUNNER_SPAWNER_TOOL_REPO:-fordaj/runner-spawner}"
TAG="${RUNNER_SPAWNER_TAG:-}"
DRY_RUN=0

# --- Helpers -----------------------------------------------------------------

usage() {
    cat <<'EOF'
Usage: update.bash [--dry-run] [--tag TAG] [--repo REPO]

Download and install RunnerSpawner release binaries.

Options:
  -n, --dry-run   Preview without changing anything
  -h, --help      Show this help
  --tag TAG       Release tag (default: latest)
  --repo REPO     GitHub repository (default: fordaj/runner-spawner)
EOF
}

die() {
    log ERROR "$*"
    exit 1
}

need_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

platform_from_asset() {
    case "$1" in
    *_macos-x86_64 | *_darwin-amd64) PLATFORM=macos-x86_64 ;;
    *_macos-arm64 | *_darwin-arm64) PLATFORM=macos-arm64 ;;
    *_linux-x86_64 | *_linux-amd64) PLATFORM=linux-x86_64 ;;
    *_linux-arm64) PLATFORM=linux-arm64 ;;
    *_windows-x86_64 | *_windows-amd64) PLATFORM=windows-x86_64 ;;
    *_windows-arm64) PLATFORM=windows-arm64 ;;
    *) return 1 ;;
    esac
}

# --- Arguments ---------------------------------------------------------------

while (($#)); do
    case "$1" in
    -n | --dry-run)
        DRY_RUN=1
        shift
        ;;
    -h | --help)
        usage
        exit 0
        ;;
    --tag | --repo)
        [[ $# -ge 2 ]] || die "$1 requires a value"
        if [[ "$1" == "--tag" ]]; then
            TAG="$2"
        else
            REPO="$2"
        fi
        shift 2
        ;;
    --tag=*)
        TAG="${1#*=}"
        shift
        ;;
    --repo=*)
        REPO="${1#*=}"
        shift
        ;;
    *) die "Unknown argument: $1" ;;
    esac
done

[[ -n "$REPO" ]] || die "Repository must not be empty"

# --- Dependencies and Release ------------------------------------------------

need_cmd gh

if [[ "$DRY_RUN" -eq 0 ]]; then
    if command -v 7zz >/dev/null 2>&1; then
        SEVENZIP=7zz
    elif command -v 7z >/dev/null 2>&1; then
        SEVENZIP=7z
    else
        die "Missing 7z/7zz (install p7zip)"
    fi
fi

if [[ -z "$TAG" ]]; then
    TAG="$(gh release view --repo "$REPO" --json tagName -q .tagName)" ||
        die "Could not determine latest release"
fi

[[ "$TAG" =~ ^v[0-9]+(\.[0-9]+)+$ ]] ||
    die "Invalid release tag: $TAG"

log INFO "Repository: $REPO"
log INFO "Release: $TAG"

# --- Dry Run -----------------------------------------------------------------

if [[ "$DRY_RUN" -eq 1 ]]; then
    assets="$(gh release view "$TAG" --repo "$REPO" --json assets -q '.assets[].name')" ||
        die "Could not list release assets"

    count=0

    while IFS= read -r asset; do
        case "$asset" in
        *.7z)
            ((count += 1))
            stem="${asset%.7z}"

            if platform_from_asset "$stem"; then
                log INFO "$asset -> $TAG/$PLATFORM"
            else
                log WARNING "$asset -> unknown platform"
            fi
            ;;
        checksums.txt)
            log INFO "$asset (checksums)"
            ;;
        esac
    done <<<"$assets"

    ((count > 0)) || die "No release archives found"

    for dir in "$SCRIPT_DIR"/v*/; do
        [[ -d "$dir" ]] || continue
        [[ "$(basename "$dir")" == "$TAG" ]] ||
            log INFO "Would remove $(basename "$dir")"
    done

    log INFO "Dry run complete"
    exit 0
fi

# --- Download and Stage ------------------------------------------------------

TMP_DIR="$(mktemp -d)"
STAGE_DIR="$(mktemp -d "$SCRIPT_DIR/.tmp.XXXXXX")"

cleanup() {
    rm -rf -- "$TMP_DIR" "$STAGE_DIR"
}
trap cleanup EXIT

log INFO "Downloading release assets..."

gh release download "$TAG" \
    --repo "$REPO" \
    --dir "$TMP_DIR" \
    --pattern '*.7z' \
    --pattern 'checksums.txt' \
    --clobber

shopt -s nullglob
archives=("$TMP_DIR"/*.7z)

((${#archives[@]} > 0)) || die "No archives downloaded"

# --- Checksum Verification ---------------------------------------------------

if [[ -f "$TMP_DIR/checksums.txt" ]]; then
    need_cmd shasum || need_cmd sha256sum

    log INFO "Verifying checksums..."

    (
        cd "$TMP_DIR"

        while read -r hash file; do
            file="${file#\*}"
            [[ -f "$file" ]] && printf '%s  %s\n' "$hash" "$file"
        done <checksums.txt >checksums.present.txt

        [[ -s checksums.present.txt ]] ||
            {
                echo "No matching checksum entries" >&2
                exit 1
            }

        if command -v shasum >/dev/null 2>&1; then
            shasum -a 256 -c checksums.present.txt
        else
            sha256sum -c checksums.present.txt
        fi
    ) || die "Checksum verification failed"

    log INFO "Checksums verified"
else
    log WARNING "No checksums.txt; skipping verification"
fi

# --- Extract Binaries --------------------------------------------------------

for archive in "${archives[@]}"; do
    name="$(basename "$archive")"

    platform_from_asset "${name%.7z}" ||
        die "Unknown platform in asset: $name"

    target="$STAGE_DIR/$PLATFORM"

    [[ ! -e "$target" ]] || die "Duplicate platform: $PLATFORM"

    extract="$TMP_DIR/extract-$PLATFORM"
    mkdir -p "$extract"

    log INFO "Extracting $name"

    "$SEVENZIP" x -y "$archive" -o"$extract" >/dev/null

    binary="$extract/runner-spawner"
    [[ -f "$binary" ]] || binary="$extract/runner-spawner.exe"

    [[ -f "$binary" ]] || die "Missing binary in $name"

    mkdir -p "$target"
    cp -p "$binary" "$target/"
    chmod +x "$target/$(basename "$binary")"

    log INFO "Installed $PLATFORM"
done

# --- Install -----------------------------------------------------------------

TAG_DIR="$SCRIPT_DIR/$TAG"

rm -rf -- "$TAG_DIR"
mv "$STAGE_DIR" "$TAG_DIR"
STAGE_DIR=""

for dir in "$SCRIPT_DIR"/v*/; do
    [[ -d "$dir" ]] || continue
    [[ "$(basename "$dir")" == "$TAG" ]] && continue

    log INFO "Removing old version $(basename "$dir")"
    rm -rf -- "$dir"
done

# --- Verify Host Binary ------------------------------------------------------

case "$(uname -s):$(uname -m)" in
Darwin:x86_64) PLATFORM=macos-x86_64 ;;
Darwin:arm64) PLATFORM=macos-arm64 ;;
Linux:x86_64) PLATFORM=linux-x86_64 ;;
Linux:aarch64) PLATFORM=linux-arm64 ;;
MINGW*:*) PLATFORM=windows-x86_64 ;;
*) PLATFORM="" ;;
esac

if [[ -n "$PLATFORM" ]]; then
    binary="$TAG_DIR/$PLATFORM/runner-spawner"
    [[ -x "$binary" ]] || binary="$TAG_DIR/$PLATFORM/runner-spawner.exe"

    if [[ -x "$binary" ]]; then
        version="$("$binary" --version 2>&1 || true)"
        log INFO "Host binary: $version"

        [[ "$version" == *"$TAG"* ]] ||
            log WARNING "Binary version does not match $TAG"
    else
        log WARNING "No binary for host platform $PLATFORM"
    fi
fi

log INFO "Successfully installed $TAG"
