package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// appName is the name of the executable of the tool.
const appName = "mirror-clone"

// installDir returns the directory mirror-clone is installed into on the
// given platform. home is the home directory of the user and localAppData the
// value of %LOCALAPPDATA%, which is only used on Windows.
//
// The directory belongs to the user, so no administrator privileges are
// needed to install the tool.
func installDir(goos, home, localAppData string) (string, error) {
	switch goos {
	case "linux", "darwin":
		if home == "" {
			return "", fmt.Errorf("the home directory of the user is unknown")
		}
		return filepath.Join(home, ".local", "bin"), nil
	case "windows":
		if localAppData == "" {
			return "", fmt.Errorf("the %%LOCALAPPDATA%% environment variable is not set")
		}
		return filepath.Join(localAppData, "Programs", appName), nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", goos)
	}
}

// defaultInstallDir returns the installation directory of the platform the
// tool runs on.
func defaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the home directory: %w", err)
	}
	return installDir(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"))
}

// executableName returns the file name of the executable on the given
// platform.
func executableName(goos string) string {
	if goos == "windows" {
		return appName + ".exe"
	}
	return appName
}

// installPath returns the path of the installed executable for the given
// platform.
func installPath(goos, dir string) string {
	return filepath.Join(dir, executableName(goos))
}

// install copies the executable at source into dir and returns the path of
// the installed file. The directory is created when it does not exist.
func install(source, dir, name string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("the location of the running executable is unknown")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}

	target := filepath.Join(dir, name)
	if err := copyExecutable(source, target); err != nil {
		return "", err
	}
	return target, nil
}

// copyExecutable copies the file at src to dst. The copy is written to a
// temporary file first and moved into place afterwards, so a failed copy
// never leaves a broken executable behind.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+appName+"-*")
	if err != nil {
		return fmt.Errorf("creating a temporary file in %s: %w", filepath.Dir(dst), err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpName, err)
	}
	// The permissions are set explicitly, because the mode of an existing
	// file is not changed when it is created.
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("setting the permissions of %s: %w", tmpName, err)
	}
	if err := replaceFile(dst, tmpName); err != nil {
		return err
	}
	return nil
}

// replaceFile replaces the file at target with the file at replacement.
//
// The file is replaced in one step, which is possible as long as it is not
// locked by a running process. Windows locks a running executable, so the
// old file is moved aside there and removed afterwards. When the
// replacement fails, the old file is put back.
func replaceFile(target, replacement string) error {
	err := os.Rename(replacement, target)
	if err == nil {
		return nil
	}

	backup := target + ".old"
	_ = os.Remove(backup)

	moved := false
	if _, statErr := os.Stat(target); statErr == nil {
		if moveErr := os.Rename(target, backup); moveErr != nil {
			// A backup of an earlier update is still in use, so the old
			// file is moved aside under another name.
			backup = fmt.Sprintf("%s.%d.old", target, os.Getpid())
			if moveErr := os.Rename(target, backup); moveErr != nil {
				return fmt.Errorf("replacing %s: %w", target, err)
			}
		}
		moved = true
	}

	if renameErr := os.Rename(replacement, target); renameErr != nil {
		if moved {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("replacing %s: %w", target, renameErr)
	}

	if moved {
		// The old file can only be removed when no process uses it any more.
		// On Windows a running executable stays locked until the process
		// ends, so a leftover file is not an error.
		_ = os.Remove(backup)
	}
	return nil
}

// uninstall removes the installed executable at path. It reports whether the
// file was there and got removed.
func uninstall(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}
	if err := removeExecutable(path); err != nil {
		return false, fmt.Errorf("removing %s: %w", path, err)
	}
	// A file that was moved aside by an earlier update is removed as well.
	_ = os.Remove(path + ".old")
	return true, nil
}

// removeDirIfEmpty removes dir when it does not contain any file. A
// directory with other files in it is kept, because it might belong to
// another program.
func removeDirIfEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return false, nil
	}
	if err := os.Remove(dir); err != nil {
		return false, fmt.Errorf("removing %s: %w", dir, err)
	}
	return true, nil
}

// pathContains reports whether dir is part of the PATH. The comparison of
// the entries is case insensitive on Windows, where the PATH is case
// insensitive too.
func pathContains(goos, path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	want := filepath.Clean(dir)
	for _, entry := range strings.Split(path, string(os.PathListSeparator)) {
		if entry == "" {
			continue
		}
		candidate := filepath.Clean(entry)
		if candidate == want {
			return true
		}
		if goos == "windows" && strings.EqualFold(candidate, want) {
			return true
		}
	}
	return false
}

// pathInstructions returns the steps that put dir on the PATH. The shell
// configuration of the user is never changed automatically.
func pathInstructions(goos, dir string) []string {
	if goos == "windows" {
		return []string{
			fmt.Sprintf("Add %s to the PATH of your user account.", dir),
			fmt.Sprintf("setx PATH \"%%PATH%%;%s\"", dir),
		}
	}
	return []string{
		fmt.Sprintf("Add %s to the PATH of your shell configuration:", dir),
		fmt.Sprintf("export PATH=\"$PATH:%s\"", dir),
	}
}

// runInstall installs the running executable into the directory of the user.
func runInstall(log *Logger) int {
	dir, err := defaultInstallDir()
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	source, err := os.Executable()
	if err != nil {
		log.Errorf("cannot determine the location of the running executable: %v", err)
		return 1
	}

	target, err := install(source, dir, executableName(runtime.GOOS))
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	log.Infof("Installed %s", target)

	if !pathContains(runtime.GOOS, os.Getenv("PATH"), dir) {
		log.Warnf("%s is not on your PATH.", dir)
		for _, line := range pathInstructions(runtime.GOOS, dir) {
			log.Warnf("%s", line)
		}
	}
	return 0
}

// runUninstall removes the installed executable of the user.
func runUninstall(log *Logger) int {
	dir, err := defaultInstallDir()
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	target := installPath(runtime.GOOS, dir)
	removed, err := uninstall(target)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	if !removed {
		log.Infof("%s is not installed in %s", appName, dir)
		return 0
	}
	log.Infof("Removed %s", target)

	// The directory is only removed when nothing else is in it.
	if gone, err := removeDirIfEmpty(dir); err == nil && gone {
		log.Infof("Removed %s", dir)
	}
	return 0
}
