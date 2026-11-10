#!/usr/bin/env bash
set -a

ENV_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck disable=SC1091
source "$ENV_DIR/.common.env"

# --- Resolve the newest bundled tool version ---------------------------
# .common.env pins one version directory per tool (e.g.
# tools/versioner/v0.0.3.0.5). When the working tree carries a newer
# v* directory, prefer it, so updating a tool bundle does not require
# editing every platform env file.
resolve_version_dir() {
    local tool_dir="$1" fallback="$2" newest=""
    [[ -d "$tool_dir" ]] || {
        printf '%s' "$fallback"
        return 0
    }
    # The version directories are plain "v<number>" names, so a glob is
    # safe here.
    # shellcheck disable=SC2010
    newest="$(ls -1 "$tool_dir" 2>/dev/null | grep -E '^v[0-9]' | sort -V 2>/dev/null | tail -n 1 || true)"
    if [[ -n "$newest" && -d "$tool_dir/$newest" ]]; then
        printf '%s' "$tool_dir/$newest"
    else
        printf '%s' "$fallback"
    fi
}

VERSIONER_VERSION_DIR="$(resolve_version_dir "$VERSIONER_DIR" "$VERSIONER_VERSION_DIR")"
LOGAMUFFIN_VERSION_DIR="$(resolve_version_dir "$LOGAMUFFIN_DIR" "$LOGAMUFFIN_VERSION_DIR")"
RUNNER_SPAWNER_VERSION_DIR="$(resolve_version_dir "$RUNNER_SPAWNER_DIR" "$RUNNER_SPAWNER_VERSION_DIR")"
SEVENZIP_VERSION_DIR="$(resolve_version_dir "$SEVENZIP_DIR" "$SEVENZIP_VERSION_DIR")"

# OS specific
# shellcheck disable=SC1091
case "$(uname -s):$(uname -m)" in
Darwin:x86_64) source "$ENV_DIR/.macos-x86_64.env" ;;
Darwin:arm64) source "$ENV_DIR/.macos-arm64.env" ;;
Linux:x86_64) source "$ENV_DIR/.linux-x86_64.env" ;;
Linux:aarch64) source "$ENV_DIR/.linux-arm64.env" ;;
MINGW*:x86_64 | MSYS*:x86_64 | CYGWIN*:x86_64) source "$ENV_DIR/.windows-x86_64.env" ;;
MINGW*:aarch64 | MSYS*:aarch64 | CYGWIN*:aarch64 | MINGW*:arm64 | MSYS*:arm64 | CYGWIN*:arm64) source "$ENV_DIR/.windows-arm64.env" ;;
*)
    echo "Unsupported platform: $(uname -s) $(uname -m)" >&2
    set +a
    # shellcheck disable=SC2317
    return 1 2>/dev/null || exit 1
    ;;
esac

# --- Anchor tool paths to the repository root -------------------------
# The env files describe the bundles with paths relative to the
# repository root ("tools/versioner/v0.0.3.0.5/..."). Make them
# absolute so they keep resolving from any working directory.
REPO_ROOT_DIR="$(cd -- "$ENV_DIR/.." && pwd)"
absolutize_bin() {
    local var="$1" value="${!1:-}"
    case "$value" in
    "" | /*) return 0 ;;
    esac
    printf -v "$var" '%s' "$REPO_ROOT_DIR/$value"
}

absolutize_bin VERSIONER_BIN
absolutize_bin LOGAMUFFIN_BIN
absolutize_bin RUNNER_SPAWNER_BIN
absolutize_bin SEVENZZ_BIN

# --- Fall back to PATH for missing bundles -----------------------------
# Not every bundle ships every platform (the versioner bundle has no
# windows-arm64 build), so fall back to the same tool on PATH when the
# bundled binary is not present in the working tree.
fallback_to_path() {
    local var="$1" cmd="$2" value="${!1:-}"
    if [[ -n "$value" && -x "$value" ]]; then
        return 0
    fi
    if command -v "$cmd" >/dev/null 2>&1; then
        printf -v "$var" '%s' "$(command -v "$cmd")"
    fi
}

fallback_to_path VERSIONER_BIN versioner
fallback_to_path LOGAMUFFIN_BIN logamuffin
fallback_to_path RUNNER_SPAWNER_BIN runner-spawner

if [[ -z "${SEVENZZ_BIN:-}" || ! -x "${SEVENZZ_BIN:-}" ]]; then
    for sevenzip_cmd in 7zz 7z; do
        if command -v "$sevenzip_cmd" >/dev/null 2>&1; then
            SEVENZZ_BIN="$(command -v "$sevenzip_cmd")"
            break
        fi
    done
fi

# source "project.env"
# case "$(uname -s):$(uname -m)" in
# Darwin:x86_64) source project.mac-x64.env ;;
# MINGW*:x86_64 | MSYS*:x86_64 | CYGWIN*:x86_64) source project.win-x64.env ;;
# Darwin:arm64 | Linux:x86_64 | Linux:aarch64 | *)
#     echo "Unsupported platform: $(uname -s) $(uname -m)"
#     exit 1
#     ;;
# esac

set +a
