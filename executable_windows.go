//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// The Windows error codes of a file that is used by another process.
const (
	errorSharingViolation = syscall.Errno(32)
	errorLockViolation    = syscall.Errno(33)
)

// removeExecutable removes the executable at path.
//
// Windows locks a running executable, so the file cannot be deleted while
// the program is still running. A running executable can be renamed, so it
// is moved aside and the leftover file is reported to the user.
func removeExecutable(path string) error {
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if !isFileLocked(err) {
		return err
	}

	aside := path + ".old"
	_ = os.Remove(aside)
	if renameErr := os.Rename(path, aside); renameErr != nil {
		return err
	}
	return fmt.Errorf("the file is in use by a running %s, it was moved to %s and has to be removed by hand once the program is closed", appName, aside)
}

// isFileLocked reports whether err was caused by a file that is locked by
// another process.
func isFileLocked(err error) bool {
	if err == nil {
		return false
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errorSharingViolation || errno == errorLockViolation
}
