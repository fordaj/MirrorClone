package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test when git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
}

// testGit runs a git command and fails the test when it does not succeed.
func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=MirrorClone Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=MirrorClone Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_ALLOW_PROTOCOL=file",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s) failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// newSourceRepo creates a working repository containing a single file.
func newSourceRepo(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	testGit(t, dir, "init", "--initial-branch=main")
	testGit(t, dir, "config", "user.email", "test@example.com")
	testGit(t, dir, "config", "user.name", "MirrorClone Test")
	writeTestFile(t, filepath.Join(dir, name), content)
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

// newBareRepo pushes src into a bare repository at barePath.
func newBareRepo(t *testing.T, src, barePath string) string {
	t.Helper()
	testGit(t, src, "init", "--bare", "--initial-branch=main", barePath)
	testGit(t, src, "push", barePath, "HEAD:refs/heads/main")
	return barePath
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// newTestCloner returns a cloner writing to a temporary mirror cache.
func newTestCloner(t *testing.T, cache string) (*MirrorCloner, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	log := NewLogger(buf, buf, LevelDebug)
	return NewMirrorCloner(NewGit(log), log, cache, 4), buf
}

func TestCloneRepository(t *testing.T) {
	requireGit(t)

	base := t.TempDir()
	src := newSourceRepo(t, filepath.Join(base, "src"), "file.txt", "hello")
	bare := newBareRepo(t, src, filepath.Join(base, "src.git"))

	cache := filepath.Join(base, "cache")
	dest := filepath.Join(base, "dest")

	cloner, _ := newTestCloner(t, cache)
	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: dest}); err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	if got := readTestFile(t, filepath.Join(dest, "file.txt")); got != "hello" {
		t.Errorf("cloned file = %q, want %q", got, "hello")
	}
	if !isDir(filepath.Join(dest, ".git")) {
		t.Error("the clone has no .git directory")
	}

	mirror := filepath.Join(cache, "src.git")
	if !isDir(mirror) {
		t.Fatalf("expected a mirror at %s, cache contains %v", mirror, listDir(t, cache))
	}
}

func TestCloneRefreshesExistingMirror(t *testing.T) {
	requireGit(t)

	base := t.TempDir()
	src := newSourceRepo(t, filepath.Join(base, "src"), "file.txt", "one")
	bare := newBareRepo(t, src, filepath.Join(base, "src.git"))

	cache := filepath.Join(base, "cache")
	cloner, logs := newTestCloner(t, cache)

	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: filepath.Join(base, "dest1")}); err != nil {
		t.Fatalf("first clone failed: %v", err)
	}

	// A new commit has to show up in the second clone.
	writeTestFile(t, filepath.Join(src, "file.txt"), "two")
	testGit(t, src, "commit", "-am", "second commit")
	testGit(t, src, "push", bare, "HEAD:refs/heads/main")

	logs.Reset()
	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: filepath.Join(base, "dest2")}); err != nil {
		t.Fatalf("second clone failed: %v", err)
	}

	if got := readTestFile(t, filepath.Join(base, "dest2", "file.txt")); got != "two" {
		t.Errorf("cloned file = %q, want %q", got, "two")
	}
	if !strings.Contains(logs.String(), "Fetching latest") {
		t.Errorf("expected the mirror to be refreshed, log was:\n%s", logs.String())
	}
}

func TestCloneChecksOutBranch(t *testing.T) {
	requireGit(t)

	base := t.TempDir()
	src := newSourceRepo(t, filepath.Join(base, "src"), "file.txt", "main")
	bare := newBareRepo(t, src, filepath.Join(base, "src.git"))

	testGit(t, src, "checkout", "-b", "feature")
	writeTestFile(t, filepath.Join(src, "file.txt"), "feature")
	testGit(t, src, "commit", "-am", "feature commit")
	testGit(t, src, "push", bare, "HEAD:refs/heads/feature")
	testGit(t, src, "checkout", "main")

	dest := filepath.Join(base, "dest")
	cloner, _ := newTestCloner(t, filepath.Join(base, "cache"))
	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: dest, Ref: "feature"}); err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	if got := readTestFile(t, filepath.Join(dest, "file.txt")); got != "feature" {
		t.Errorf("checked out file = %q, want %q", got, "feature")
	}
}

func TestCloneSubmodules(t *testing.T) {
	requireGit(t)

	base := t.TempDir()

	libSrc := newSourceRepo(t, filepath.Join(base, "lib-src"), "lib.txt", "lib")
	libBare := newBareRepo(t, libSrc, filepath.Join(base, "lib.git"))

	projSrc := newSourceRepo(t, filepath.Join(base, "proj-src"), "proj.txt", "proj")
	projBare := newBareRepo(t, projSrc, filepath.Join(base, "proj.git"))

	// The submodule uses a relative url, which has to be resolved against
	// the url of the superproject.
	testGit(t, projSrc, "-c", "protocol.file.allow=always", "submodule", "add", "../lib.git", "lib")
	testGit(t, projSrc, "commit", "-m", "add submodule")
	testGit(t, projSrc, "push", projBare, "HEAD:refs/heads/main")

	cache := filepath.Join(base, "cache")
	dest := filepath.Join(base, "dest")

	cloner, _ := newTestCloner(t, cache)
	if err := cloner.Clone(context.Background(), CloneRequest{URL: projBare, Dest: dest}); err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	if got := readTestFile(t, filepath.Join(dest, "proj.txt")); got != "proj" {
		t.Errorf("superproject file = %q, want %q", got, "proj")
	}
	if got := readTestFile(t, filepath.Join(dest, "lib", "lib.txt")); got != "lib" {
		t.Errorf("submodule file = %q, want %q", got, "lib")
	}
	if !isDir(filepath.Join(dest, "lib", ".git")) {
		t.Error("the submodule was not cloned into a git repository")
	}
	if !isDir(filepath.Join(cache, "lib.git")) {
		t.Errorf("expected a mirror for the submodule, cache contains %v", listDir(t, cache))
	}

	// The relative url of the submodule has to be resolved against the url
	// of the superproject.
	origin := strings.TrimSpace(testGit(t, filepath.Join(dest, "lib"), "remote", "get-url", "origin"))
	if !sameURL(origin, libBare) {
		t.Errorf("submodule origin = %q, want %q", origin, libBare)
	}
}

func TestCloneRefusesForeignDestination(t *testing.T) {
	requireGit(t)

	base := t.TempDir()
	first := newSourceRepo(t, filepath.Join(base, "first"), "file.txt", "first")
	firstBare := newBareRepo(t, first, filepath.Join(base, "first.git"))
	second := newSourceRepo(t, filepath.Join(base, "second"), "file.txt", "second")
	secondBare := newBareRepo(t, second, filepath.Join(base, "second.git"))

	dest := filepath.Join(base, "dest")
	cloner, _ := newTestCloner(t, filepath.Join(base, "cache"))

	if err := cloner.Clone(context.Background(), CloneRequest{URL: firstBare, Dest: dest}); err != nil {
		t.Fatalf("first clone failed: %v", err)
	}
	if err := cloner.Clone(context.Background(), CloneRequest{URL: secondBare, Dest: dest}); err == nil {
		t.Fatal("expected an error when cloning a different repository into an existing clone")
	}
}

func TestCloneUpdatesExistingDestination(t *testing.T) {
	requireGit(t)

	base := t.TempDir()
	src := newSourceRepo(t, filepath.Join(base, "src"), "file.txt", "one")
	bare := newBareRepo(t, src, filepath.Join(base, "src.git"))

	dest := filepath.Join(base, "dest")
	cache := filepath.Join(base, "cache")
	cloner, _ := newTestCloner(t, cache)

	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: dest}); err != nil {
		t.Fatalf("first clone failed: %v", err)
	}

	writeTestFile(t, filepath.Join(src, "file.txt"), "two")
	testGit(t, src, "commit", "-am", "second commit")
	testGit(t, src, "push", bare, "HEAD:refs/heads/main")

	if err := cloner.Clone(context.Background(), CloneRequest{URL: bare, Dest: dest}); err != nil {
		t.Fatalf("second clone failed: %v", err)
	}
	if got := readTestFile(t, filepath.Join(dest, "file.txt")); got != "two" {
		t.Errorf("cloned file = %q, want %q", got, "two")
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
