#!/usr/bin/env bash
set -euo pipefail

# --- Source Scripts --- #
(return 0 2>/dev/null) && sourced=true || sourced=false
if ! "${sourced}"; then
    source scripts/entry.sh
fi

# --- Args --- #
HOST="github.com"
while [[ $# -gt 0 ]]; do
    case "$1" in
    --host)
        HOST=$2
        shift 2
        ;;
    --tag)
        TAG=$2
        shift 2
        ;;
    --owner)
        OWNER=$2
        shift 2
        ;;
    --repo)
        REPO=$2
        shift 2
        ;;
    -h | --help)
        usage
        exit 0
        ;;
    *)
        echo "Unknown argument: $1" >&2
        usage
        exit 1
        ;;
    esac
done

# ----- Validate Inputs ----- #
UPLOADING_TO_GIT="false"
if [[ -n "${TAG:-}" || -n "${OWNER:-}" || -n "${REPO:-}" ]]; then
    UPLOADING_TO_GIT="true"
    if [[ -z "${TAG:-}" || -z "${OWNER:-}" || -z "${REPO:-}" ]]; then
        echo "Error: TAG, OWNER, and REPO must either all be provided or all be omitted" >&2
        usage
        exit 1
    fi
fi

# ----- Verify release files all exist ----- #
case "$PROJECT_MODE" in
ESP32_S3_EMULATED) FILES=("${ESP32_S3_EMULATED_RELEASE_FILES[@]}") ;;
ESP32_S3) FILES=("${ESP32_S3_RELEASE_FILES[@]}") ;;
host) FILES=("${HOST_RELEASE_FILES[@]}") ;;
esac
for file in "${FILES[@]}"; do
    if [[ ! -f "$file" ]]; then
        log $LOG_LEVEL_ERROR "File $file does not exist!"
        exit 1
    fi
done

# --- Fetch all git branches --- #
if [[ "${UPLOADING_TO_GIT:-}" == "true" ]]; then
    log $LOG_LEVEL_INFO "Fetching all branches..." &
    git fetch --all
fi

# ----- Prepare Temp Dir ----- #
TEMP="temp/${PROJECT_MODE}_Release"
log $LOG_LEVEL_TRACE "Using temp dir: $TEMP"
rm -rf "$TEMP"
mkdir -p "$TEMP"
log $LOG_LEVEL_TRACE "Determining zip file name" &
ZIP_FILE_NAME="${REPO_NAME}_${PROJECT_MODE}_v$VERSION_MAJOR.$VERSION_MINOR.$VERSION_REVISION.zip" # AirFrame_<MODE>_v0.0.0.zip
# Add OS and architecture suffix if host
if [[ "${PROJECT_MODE:-}" == "host" ]]; then
    case "$(uname -s):$(uname -m)" in
    Darwin:x86_64) ZIP_FILE_NAME="${ZIP_FILE_NAME%.zip}_MacOS_x64.zip" ;;
    MINGW*:x86_64 | MSYS*:x86_64 | CYGWIN*:x86_64) ZIP_FILE_NAME="${ZIP_FILE_NAME%.zip}_Windows_x64.zip" ;;
    Linux:aarch64 | Linux:x86_64 | Darwin:arm64 | *)
        echo "Unsupported platform: $(uname -s) $(uname -m)"
        exit 1
        ;;
    esac
fi
ZIP_FILE="$TEMP/$ZIP_FILE_NAME"

# ----- Choose 7Zip exe ----- #
SEVENZIP_EXE=""
case "$(uname -s):$(uname -m)" in
Darwin:x86_64 | Darwin:arm64) SEVENZIP_EXE="$SEVENZZ_BIN" ;;
MINGW*:x86_64 | MSYS*:x86_64 | CYGWIN*:x86_64) SEVENZIP_EXE="$SEVENZA_EXE" ;;
Linux:aarch64 | Linux:x86_64 | *)
    echo "Unsupported platform: $(uname -s) $(uname -m)"
    exit 1
    ;;
esac
SEVENZIP_EXE="$(realpath "$SEVENZIP_EXE")" # get abs path

# ----- Copy files to temp dir ----- #
case "$PROJECT_MODE" in
ESP32_S3_EMULATED) FILES=("${ESP32_S3_EMULATED_RELEASE_FILES[@]}") ;;
ESP32_S3) FILES=("${ESP32_S3_RELEASE_FILES[@]}") ;;
host) FILES=("${HOST_RELEASE_FILES[@]}") ;;
esac
for file in "${FILES[@]}"; do
    cp "$file" "$TEMP/"
done

# ----- Zip up release ----- #
log $LOG_LEVEL_INFO "Release mode: $PROJECT_MODE" &
(
    cd $TEMP
    "$SEVENZIP_EXE" a "$ZIP_FILE_NAME" "*"
)
if [[ ! -f "$ZIP_FILE" ]]; then
    log $LOG_LEVEL_ERROR "File $ZIP_FILE does not exist!"
    exit 1
else
    log $LOG_LEVEL_DEBUG "Successfully created $ZIP_FILE"
fi

# ----- Upload to Github ----- #
if [[ "${UPLOADING_TO_GIT:-}" == "true" ]]; then
    log $LOG_LEVEL_DEBUG "Uploading $ZIP_FILE to $HOST/$OWNER/$REPO:$TAG"
    export GH_HOST="$HOST"
    export GH_SPINNER_DISABLED=1
    gh release upload $TAG $ZIP_FILE --clobber --repo "$OWNER/$REPO"
    log $LOG_LEVEL_TRACE "Upload complete"
fi
