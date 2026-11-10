package main

import (
	"fmt"
	"runtime"
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
