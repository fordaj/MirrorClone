//go:build !windows

package main

import "os"

// removeExecutable removes the executable at path.
//
// On Unix systems a running executable can be removed, the file is only
// unlinked and stays usable until the process ends.
func removeExecutable(path string) error {
	return os.Remove(path)
}
