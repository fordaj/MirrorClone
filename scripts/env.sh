#!/usr/bin/env bash
set -a

source scripts/.common.env

# OS specific
case "$(uname -s):$(uname -m)" in
Darwin:x86_64) source scripts/.macos-x86_64.env ;;
Darwin:arm64) source scripts/.macos-arm64.env ;;
Linux:x86_64) source scripts/.linux-x86_64.env ;;
Linux:aarch64) source scripts/.linux-arm64.env ;;
MINGW*:x86_64 | MSYS*:x86_64 | CYGWIN*:x86_64) source scripts/.windows-x86_64.env ;;
MINGW*:aarch64 | MSYS*:aarch64 | CYGWIN*:aarch64 | MINGW*:arm64 | MSYS*:arm64 | CYGWIN*:arm64) source scripts/.windows-arm64.env ;;
*)
	echo "Unsupported platform: $OS $ARCH" >&2
	set +a
	return 1 2>/dev/null || exit 1
	;;
esac

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
