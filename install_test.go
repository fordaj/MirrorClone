package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeTestExecutable writes a file with the given content and returns its
// path.
func writeTestExecutable(t *testing.T, path, content string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestInstallDir(t *testing.T) {
	cases := []struct {
		name         string
		goos         string
		home         string
		localAppData string
		want         string
		wantErr      bool
	}{
		{
			name: "linux uses the local bin directory",
			goos: "linux",
			home: "/home/user",
			want: "/home/user/.local/bin",
		},
		{
			name: "macos uses the local bin directory",
			goos: "darwin",
			home: "/Users/user",
			want: "/Users/user/.local/bin",
		},
		{
			name:         "windows uses the local app data directory",
			goos:         "windows",
			home:         `C:\Users\user`,
			localAppData: `C:\Users\user\AppData\Local`,
			want:         filepath.Join(`C:\Users\user\AppData\Local`, "Programs", "mirror-clone"),
		},
		{
			name:    "windows without the local app data directory fails",
			goos:    "windows",
			home:    `C:\Users\user`,
			wantErr: true,
		},
		{
			name:    "linux without a home directory fails",
			goos:    "linux",
			wantErr: true,
		},
		{
			name:    "an unsupported platform fails",
			goos:    "plan9",
			home:    "/home/user",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installDir(tc.goos, tc.home, tc.localAppData)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("installDir(%q, %q, %q) did not return an error", tc.goos, tc.home, tc.localAppData)
				}
				return
			}
			if err != nil {
				t.Fatalf("installDir(%q, %q, %q) returned error: %v", tc.goos, tc.home, tc.localAppData, err)
			}
			if got != tc.want {
				t.Errorf("installDir(%q, %q, %q) = %q, want %q", tc.goos, tc.home, tc.localAppData, got, tc.want)
			}
		})
	}
}

func TestExecutableName(t *testing.T) {
	if got := executableName("linux"); got != "mirror-clone" {
		t.Errorf("executableName(%q) = %q, want %q", "linux", got, "mirror-clone")
	}
	if got := executableName("windows"); got != "mirror-clone.exe" {
		t.Errorf("executableName(%q) = %q, want %q", "windows", got, "mirror-clone.exe")
	}
}

func TestInstall(t *testing.T) {
	source := writeTestExecutable(t, filepath.Join(t.TempDir(), "mirror-clone"), "the binary")
	dir := filepath.Join(t.TempDir(), "bin")

	target, err := install(source, dir, "mirror-clone")
	if err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	want := filepath.Join(dir, "mirror-clone")
	if target != want {
		t.Errorf("install returned %q, want %q", target, want)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the binary" {
		t.Errorf("the installed file contains %q, want %q", data, "the binary")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("reading the mode of %s: %v", target, err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("the mode of %s is %v, want %v", target, info.Mode().Perm(), os.FileMode(0o755))
	}
}

func TestInstallCreatesTheDirectory(t *testing.T) {
	source := writeTestExecutable(t, filepath.Join(t.TempDir(), "mirror-clone"), "the binary")
	dir := filepath.Join(t.TempDir(), "missing", "bin")

	if _, err := install(source, dir, "mirror-clone"); err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
}

func TestInstallReplacesTheExistingFile(t *testing.T) {
	source := writeTestExecutable(t, filepath.Join(t.TempDir(), "mirror-clone"), "the new binary")
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	if _, err := install(source, dir, "mirror-clone"); err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the new binary" {
		t.Errorf("the installed file contains %q, want %q", data, "the new binary")
	}
}

func TestInstallWithoutSource(t *testing.T) {
	if _, err := install("", t.TempDir(), "mirror-clone"); err == nil {
		t.Error("install without a source did not return an error")
	}
}

func TestUninstall(t *testing.T) {
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the binary")

	removed, err := uninstall(target)
	if err != nil {
		t.Fatalf("uninstall returned error: %v", err)
	}
	if !removed {
		t.Error("uninstall did not remove the file")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("%s still exists", target)
	}
}

func TestUninstallWithoutInstallation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mirror-clone")

	removed, err := uninstall(target)
	if err != nil {
		t.Fatalf("uninstall returned error: %v", err)
	}
	if removed {
		t.Error("uninstall removed a file that does not exist")
	}
}

func TestUninstallKeepsTheDirectoryWithOtherFiles(t *testing.T) {
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the binary")
	writeTestExecutable(t, filepath.Join(dir, "another-program"), "another binary")

	if _, err := uninstall(target); err != nil {
		t.Fatalf("uninstall returned error: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("the directory %s was removed: %v", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "another-program")); err != nil {
		t.Errorf("the other file in %s was removed: %v", dir, err)
	}
}

func TestRemoveDirIfEmpty(t *testing.T) {
	dir := t.TempDir()

	gone, err := removeDirIfEmpty(dir)
	if err != nil {
		t.Fatalf("removeDirIfEmpty returned error: %v", err)
	}
	if !gone {
		t.Error("removeDirIfEmpty did not remove the empty directory")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the directory %s still exists", dir)
	}
}

func TestRemoveDirIfEmptyKeepsTheDirectoryWithOtherFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestExecutable(t, filepath.Join(dir, "another-program"), "another binary")

	gone, err := removeDirIfEmpty(dir)
	if err != nil {
		t.Fatalf("removeDirIfEmpty returned error: %v", err)
	}
	if gone {
		t.Error("removeDirIfEmpty removed a directory with other files in it")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("the directory %s was removed: %v", dir, err)
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")
	replacement := writeTestExecutable(t, filepath.Join(dir, "replacement"), "the new binary")

	if err := replaceFile(target, replacement); err != nil {
		t.Fatalf("replaceFile returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the new binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the new binary")
	}
	if _, err := os.Stat(replacement); !os.IsNotExist(err) {
		t.Errorf("the replacement %s still exists", replacement)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Errorf("the backup %s still exists", target+".old")
	}
}

func TestReplaceFileKeepsTheTargetOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := writeTestExecutable(t, filepath.Join(dir, "mirror-clone"), "the old binary")

	err := replaceFile(target, filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("replaceFile did not return an error")
	}

	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("reading %s: %v", target, readErr)
	}
	if string(data) != "the old binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the old binary")
	}
}

func TestReplaceFileWithoutTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mirror-clone")
	replacement := writeTestExecutable(t, filepath.Join(dir, "replacement"), "the new binary")

	if err := replaceFile(target, replacement); err != nil {
		t.Fatalf("replaceFile returned error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading %s: %v", target, err)
	}
	if string(data) != "the new binary" {
		t.Errorf("%s contains %q, want %q", target, data, "the new binary")
	}
}

func TestPathContains(t *testing.T) {
	sep := string(os.PathListSeparator)
	path := strings.Join([]string{"/usr/bin", "/usr/local/bin", "/home/user/.local/bin"}, sep)

	if !pathContains("linux", path, "/home/user/.local/bin") {
		t.Error("the directory was not found in the PATH")
	}
	if pathContains("linux", path, "/opt/bin") {
		t.Error("a directory that is not in the PATH was found")
	}
	if pathContains("linux", "", "/home/user/.local/bin") {
		t.Error("a directory was found in an empty PATH")
	}
}

func TestPathContainsIsCaseInsensitiveOnWindows(t *testing.T) {
	sep := string(os.PathListSeparator)
	path := strings.Join([]string{"/Windows/System32", "/Users/User/.local/bin"}, sep)

	if !pathContains("windows", path, "/users/user/.local/bin") {
		t.Error("the directory was not found in the PATH")
	}
	if pathContains("linux", path, "/users/user/.local/bin") {
		t.Error("the comparison was case insensitive on a case sensitive platform")
	}
}

func TestPathInstructions(t *testing.T) {
	unix := pathInstructions("linux", "/home/user/.local/bin")
	if len(unix) == 0 {
		t.Fatal("no instructions for linux")
	}
	if !strings.Contains(strings.Join(unix, "\n"), "/home/user/.local/bin") {
		t.Errorf("the instructions do not mention the directory: %v", unix)
	}

	windows := pathInstructions("windows", `C:\Users\user\AppData\Local\Programs\mirror-clone`)
	if len(windows) == 0 {
		t.Fatal("no instructions for windows")
	}
	if !strings.Contains(strings.Join(windows, "\n"), "PATH") {
		t.Errorf("the instructions do not mention the PATH: %v", windows)
	}
}

func TestDefaultInstallDir(t *testing.T) {
	dir, err := defaultInstallDir()
	if err != nil {
		t.Fatalf("defaultInstallDir returned error: %v", err)
	}
	if dir == "" {
		t.Error("defaultInstallDir returned an empty directory")
	}

	want := installPath(runtime.GOOS, dir)
	if !strings.HasSuffix(want, executableName(runtime.GOOS)) {
		t.Errorf("the installation path %q does not end with the executable", want)
	}
}
