package mirrorclone

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// RepoNameFromURL extracts the repository name from a git URL/path.
func RepoNameFromURL(url string) string {
	u := strings.TrimRight(url, "/")
	base := path.Base(u)
	base = strings.TrimSuffix(base, ".git")
	if base == "." || base == "/" || base == "" {
		return "repo"
	}
	return base
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = ioMultiWriter(os.Stderr, &stderr)
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func runGitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// small helper to duplicate stderr to buffer without extra deps in callers
type multiWriter struct {
	a, b *bytes.Buffer
	w    *os.File
}

func ioMultiWriter(file *os.File, buf *bytes.Buffer) *mw {
	return &mw{file: file, buf: buf}
}

type mw struct {
	file *os.File
	buf  *bytes.Buffer
}

func (m *mw) Write(p []byte) (n int, err error) {
	n, err = m.file.Write(p)
	if n > 0 {
		_, _ = m.buf.Write(p[:n])
	}
	return n, err
}

// MirrorClone clones or updates a mirror for repoURL under mirrorDir and clones into cloneDir.
// If ref is non-empty, it is checked out in the cloned repository.
func MirrorClone(repoName, repoURL, mirrorDir, cloneDir, ref string) error {
	mirrorRepo := filepath.Join(mirrorDir, repoName+".git")

	if info, err := os.Stat(mirrorRepo); err == nil && info.IsDir() {
		fmt.Printf("Fetching latest in %s\n", filepath.ToSlash(mirrorRepo))
		if err := runGit("", "--git-dir", mirrorRepo, "remote", "update", "--prune"); err != nil {
			// fallback to fetch origin
			if err2 := runGit("", "--git-dir", mirrorRepo, "fetch", "--prune", "origin"); err2 != nil {
				return err
			}
		}
	} else {
		fmt.Printf("Mirror cloning %s\n", filepath.ToSlash(mirrorRepo))
		if err := runGit("", "clone", "--mirror", repoURL, mirrorRepo); err != nil {
			return err
		}
	}

	fmt.Printf("Cloning %s from %s\n", filepath.ToSlash(cloneDir), filepath.ToSlash(mirrorRepo))
	if err := runGit("", "clone", "--reference", mirrorRepo, repoURL, cloneDir); err != nil {
		return err
	}

	if strings.TrimSpace(ref) != "" {
		fmt.Printf("Checking out %s\n", ref)
		if err := runGit(cloneDir, "checkout", ref); err != nil {
			return err
		}
	}

	return cloneSubmodules(cloneDir, mirrorDir)
}

type submodule struct {
	path string
	url  string
	sha  string
}

func cloneSubmodules(repoPath, mirrorDir string) error {
	subs, err := listSubmodules(repoPath)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(subs))
	for _, sm := range subs {
		sm := sm
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := filepath.Base(sm.path)
			dest := filepath.Join(repoPath, sm.path)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				errCh <- fmt.Errorf("submodule %s: %w", sm.path, err)
				return
			}
			if err := MirrorClone(name, sm.url, mirrorDir, dest, sm.sha); err != nil {
				errCh <- fmt.Errorf("submodule %s: %w", sm.path, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			return e
		}
	}
	return nil
}

func listSubmodules(repoPath string) ([]submodule, error) {
	gitmodules := filepath.Join(repoPath, ".gitmodules")
	if _, err := os.Stat(gitmodules); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out, err := runGitOutput(repoPath, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		// no submodules configured
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	subs := make([]submodule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0] // submodule.<name>.path
		smPath := strings.TrimSpace(parts[1])
		name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".path")
		urlKey := fmt.Sprintf("submodule.%s.url", name)
		urlOut, err := runGitOutput(repoPath, "config", "-f", ".gitmodules", "--get", urlKey)
		if err != nil {
			return nil, err
		}
		smURL := strings.TrimSpace(urlOut)
		sha, err := submoduleSHA(repoPath, smPath)
		if err != nil {
			return nil, err
		}
		subs = append(subs, submodule{path: filepath.FromSlash(smPath), url: smURL, sha: sha})
	}
	return subs, nil
}

func submoduleSHA(repoPath, smPath string) (string, error) {
	out, err := runGitOutput(repoPath, "ls-files", "-s", "--", smPath)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) >= 2 {
		return fields[1], nil
	}
	return "", nil
}
