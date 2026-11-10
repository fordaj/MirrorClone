#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "scripts/entry.sh"

# Resolve version/tag via versioner
if [[ -z "${VERSIONER_BIN:-}" || ! -x "${VERSIONER_BIN:-}" ]]; then
	log ERROR "VERSIONER_BIN is not set or not executable: ${VERSIONER_BIN:-<unset>}"
	exit 1
fi
if ! command -v gh >/dev/null 2>&1; then
	log ERROR "gh CLI is required to create/upload GitHub releases"
	exit 1
fi
if ! command -v zip >/dev/null 2>&1; then
	log ERROR "zip is required to package release artifacts"
	exit 1
fi

log INFO "Computing version with versioner"
eval "$($VERSIONER_BIN --format shell)"
if [[ -z "${TAG:-}" || -z "${VERSION:-}" ]]; then
	log ERROR "Failed to determine TAG/VERSION from versioner"
	exit 1
fi
log INFO "Release tag: ${TAG} (version ${VERSION})"

# Build release artifacts
"$SCRIPT_DIR/build.bash"

DIST_DIR="${SCRIPT_DIR}/dist"
STAGE_DIR="${SCRIPT_DIR}/.release-stage"
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"

shopt -s nullglob
artifacts=("${DIST_DIR}"/mirror-clone-*)
if [[ ${#artifacts[@]} -eq 0 ]]; then
	log ERROR "No build artifacts found in ${DIST_DIR}"
	exit 1
fi

packages=()
for bin in "${artifacts[@]}"; do
	base="$(basename "$bin")"
	pkg_name="${base%.exe}"
	pkg_dir="${STAGE_DIR}/${pkg_name}"
	mkdir -p "$pkg_dir"
	cp "$bin" "$pkg_dir/"
	cp "${SCRIPT_DIR}/LICENSE" "$pkg_dir/"
	cp "${SCRIPT_DIR}/README.md" "$pkg_dir/"
	(
		cd "$STAGE_DIR"
		zip -r "${pkg_name}.zip" "$pkg_name" >/dev/null
	)
	packages+=("${STAGE_DIR}/${pkg_name}.zip")
	log INFO "Packaged ${pkg_name}.zip"
done

# Create tag if needed and upload release assets
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
	log INFO "Tag ${TAG} already exists locally"
else
	log INFO "Creating tag ${TAG}"
	git tag -a "${TAG}" -m "Release ${TAG}"
fi

if git ls-remote --exit-code --tags origin "refs/tags/${TAG}" >/dev/null 2>&1; then
	log INFO "Tag ${TAG} already exists on origin"
else
	log INFO "Pushing tag ${TAG} to origin"
	git push origin "${TAG}"
fi

if gh release view "${TAG}" >/dev/null 2>&1; then
	log INFO "GitHub release ${TAG} exists; uploading assets"
	gh release upload "${TAG}" "${packages[@]}" --clobber
else
	log INFO "Creating GitHub release ${TAG}"
	gh release create "${TAG}" "${packages[@]}" --title "${TAG}" --notes "Release ${TAG}"
fi

log INFO "Release ${TAG} completed"
