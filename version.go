package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// Build information. The values are overridden at build time with
//
//	-ldflags "-X main.version=... -X main.commit=... -X main.date=..."
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	mode    = "release"
)

// versionString returns the human readable version of the tool.
func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s, %s/%s)", version, commit, date, mode, runtime.GOOS, runtime.GOARCH)
}

// compareVersions compares two versions and returns -1 when a is older than
// b, 1 when a is newer than b and 0 when both are the same.
//
// A leading "v" is ignored and the parts are compared as numbers, so
// "v0.0.1.0.10" is newer than "0.0.1.0.9". Parts that are not numbers end
// the comparison, so a pre release suffix never sorts above the release it
// belongs to.
func compareVersions(a, b string) int {
	partsA := versionParts(a)
	partsB := versionParts(b)

	count := len(partsA)
	if len(partsB) > count {
		count = len(partsB)
	}

	for i := 0; i < count; i++ {
		x := 0
		if i < len(partsA) {
			x = partsA[i]
		}
		y := 0
		if i < len(partsB) {
			y = partsB[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// versionParts splits a version into its numeric parts.
func versionParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	var parts []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		parts = append(parts, n)
	}
	return parts
}

// releaseVersion returns the version of a release tag without the "v"
// prefix, so it can be used in the names of the release archives.
func releaseVersion(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}
