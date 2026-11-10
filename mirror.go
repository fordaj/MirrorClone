package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CloneRequest describes a single clone operation.
type CloneRequest struct {
	// Name is the name used for the mirror cache entry. It defaults to the
	// repository name derived from URL.
	Name string
	// URL is the repository to clone.
	URL string
	// Dest is the directory the working copy is written to.
	Dest string
	// Ref is an optional branch or commit that is checked out after cloning.
	Ref string
}

// Submodule is a single entry of a .gitmodules file.
type Submodule struct {
	// Name is the submodule name as written in .gitmodules.
	Name string
	// Path is the path of the submodule inside the superproject.
	Path string
	// URL is the raw URL as written in .gitmodules.
	URL string
	// SHA is the commit recorded in the superproject index.
	SHA string
}

// MirrorCloner clones repositories through a local mirror cache. Every
// repository (and every submodule) is mirrored once and then cloned from that
// mirror, which keeps repeated clones fast and cheap.
type MirrorCloner struct {
	git     *Git
	log     *Logger
	cache   string
	workers chan struct{}
}

// NewMirrorCloner returns a cloner using cache as the mirror cache
// directory. jobs limits how many submodules are cloned in parallel.
func NewMirrorCloner(git *Git, log *Logger, cache string, jobs int) *MirrorCloner {
	if jobs < 1 {
		jobs = 1
	}
	return &MirrorCloner{
		git:     git,
		log:     log,
		cache:   cache,
		workers: make(chan struct{}, jobs),
	}
}

// Clone mirrors req.URL (refreshing an existing mirror if needed), clones it
// into req.Dest and clones all submodules recursively.
func (m *MirrorCloner) Clone(ctx context.Context, req CloneRequest) error {
	if strings.TrimSpace(req.URL) == "" {
		return errors.New("no repository url given")
	}
	if req.Dest == "" {
		return errors.New("no destination directory given")
	}

	name := req.Name
	if name == "" {
		name = repoNameFromURL(req.URL)
	}

	if err := os.MkdirAll(filepath.Dir(req.Dest), 0o755); err != nil {
		return fmt.Errorf("creating parent directory of %s: %w", req.Dest, err)
	}
	if err := os.MkdirAll(req.Dest, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", req.Dest, err)
	}

	mirror, err := m.ensureMirror(ctx, name, req.URL)
	if err != nil {
		return err
	}

	if err := m.cloneFromMirror(ctx, mirror, req.URL, req.Dest); err != nil {
		return err
	}

	if req.Ref != "" {
		if err := m.checkout(ctx, req.Dest, req.Ref, mirror); err != nil {
			return err
		}
	}

	return m.cloneSubmodules(ctx, req.URL, req.Dest)
}

// ensureMirror returns the path of a mirror of url, creating or refreshing it
// as needed.
func (m *MirrorCloner) ensureMirror(ctx context.Context, name, url string) (string, error) {
	if err := os.MkdirAll(m.cache, 0o755); err != nil {
		return "", fmt.Errorf("creating mirror cache %s: %w", m.cache, err)
	}

	safe := sanitizeName(name)
	primary := filepath.Join(m.cache, safe+".git")
	// A second repository with the same name (common for submodules) must
	// not steal the cache entry of the first one, so a URL specific name is
	// used as fallback.
	disambiguated := filepath.Join(m.cache, safe+"-"+shortHash(url)+".git")

	for _, candidate := range []string{primary, disambiguated} {
		if !isDir(candidate) {
			continue
		}
		origin, err := m.git.Run(ctx, candidate, "remote", "get-url", "origin")
		if err != nil {
			m.log.Warnf("removing unusable mirror %s", candidate)
			if rmErr := os.RemoveAll(candidate); rmErr != nil {
				return "", fmt.Errorf("removing broken mirror %s: %w", candidate, rmErr)
			}
			continue
		}
		if !sameURL(origin, url) {
			continue
		}

		m.log.Infof("Fetching latest in %s", candidate)
		if _, err := m.git.Run(ctx, candidate, "fetch", "--prune", "origin"); err != nil {
			return "", fmt.Errorf("fetching mirror %s: %w", candidate, err)
		}
		return candidate, nil
	}

	target := primary
	if isDir(primary) {
		target = disambiguated
	}

	m.log.Infof("Mirror cloning %s", target)
	if _, err := m.git.Run(ctx, "", "clone", "--mirror", url, target); err != nil {
		return "", fmt.Errorf("mirroring %s: %w", url, err)
	}
	return target, nil
}

// cloneFromMirror clones url into dest, borrowing the objects of the mirror
// so no objects have to be downloaded twice.
func (m *MirrorCloner) cloneFromMirror(ctx context.Context, mirror, url, dest string) error {
	if isDir(dest) && !isEmptyDir(dest) {
		if !isGitRepo(ctx, m.git, dest) {
			return fmt.Errorf("destination %s already exists and is not a git repository", dest)
		}
		origin, err := m.git.Run(ctx, dest, "remote", "get-url", "origin")
		if err != nil {
			return fmt.Errorf("reading origin of %s: %w", dest, err)
		}
		if !sameURL(origin, url) {
			return fmt.Errorf("destination %s already contains a clone of %s", dest, origin)
		}
		m.log.Infof("Updating existing clone %s", dest)
		if _, err := m.git.Run(ctx, dest, "fetch", "--prune", "origin"); err != nil {
			return fmt.Errorf("updating %s: %w", dest, err)
		}
		m.fastForward(ctx, dest)
		return nil
	}

	m.log.Infof("Cloning %s from %s", dest, mirror)
	// --reference makes git use the objects of the mirror instead of
	// downloading them again, --dissociate copies the borrowed objects
	// into the new repository so the clone stays usable when the mirror
	// cache is removed.
	if _, err := m.git.Run(ctx, "", "clone", "--reference", mirror, "--dissociate", url, dest); err != nil {
		return fmt.Errorf("cloning %s: %w", url, err)
	}
	return nil
}

// fastForward moves the working copy of an existing clone to the fetched
// default branch. Local changes are never discarded: when the branch cannot
// be fast forwarded a warning is logged instead.
func (m *MirrorCloner) fastForward(ctx context.Context, dest string) {
	head, err := m.git.Run(ctx, dest, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil || head == "" {
		branch, err := m.git.Run(ctx, dest, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || branch == "" || branch == "HEAD" {
			return
		}
		head = "origin/" + branch
	}
	if _, err := m.git.Run(ctx, dest, "merge", "--ff-only", head); err != nil {
		m.log.Warnf("could not fast-forward %s to %s: %v", dest, head, err)
	}
}

// checkout checks out ref in dest. The commit is looked up in the mirror as
// well, so refs that are not reachable from any remote branch (a submodule
// pinned to an old commit) still work.
func (m *MirrorCloner) checkout(ctx context.Context, dest, ref, mirror string) error {
	m.log.Infof("Checking out %s", ref)
	if _, err := m.git.Run(ctx, dest, "checkout", ref); err != nil {
		m.log.Debugf("checkout of %s failed, fetching refs from the mirror", ref)
		if _, fetchErr := m.git.Run(ctx, dest, "fetch", "--no-tags", mirror, "+refs/*:refs/remotes/mirror/*"); fetchErr != nil {
			return fmt.Errorf("checking out %s: %w", ref, err)
		}
		if _, err := m.git.Run(ctx, dest, "checkout", ref); err != nil {
			return fmt.Errorf("checking out %s: %w", ref, err)
		}
	}
	return nil
}

// cloneSubmodules clones all submodules of the repository in repoPath. The
// superproject URL is used to resolve relative submodule URLs.
func (m *MirrorCloner) cloneSubmodules(ctx context.Context, parentURL, repoPath string) error {
	submodules, err := m.readSubmodules(ctx, repoPath)
	if err != nil {
		return err
	}
	if len(submodules) == 0 {
		return nil
	}

	m.log.Infof("Cloning %d submodule(s)", len(submodules))

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, submodule := range submodules {
		wg.Add(1)
		go func(sub Submodule) {
			defer wg.Done()

			select {
			case m.workers <- struct{}{}:
				defer func() { <-m.workers }()
			case <-ctx.Done():
				return
			}

			if err := m.cloneSubmodule(ctx, parentURL, repoPath, sub); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(submodule)
	}
	wg.Wait()

	return errors.Join(errs...)
}

// cloneSubmodule clones a single submodule into the superproject.
func (m *MirrorCloner) cloneSubmodule(ctx context.Context, parentURL, repoPath string, sub Submodule) error {
	url, err := resolveSubmoduleURL(parentURL, sub.URL)
	if err != nil {
		return fmt.Errorf("submodule %s: %w", sub.Path, err)
	}

	name := repoNameFromURL(url)
	if name == "" {
		name = filepath.Base(filepath.FromSlash(sub.Path))
	}

	dest := filepath.Join(repoPath, filepath.FromSlash(sub.Path))
	m.log.Infof("Submodule %s -> %s", sub.Path, url)

	return m.Clone(ctx, CloneRequest{
		Name: name,
		URL:  url,
		Dest: dest,
		Ref:  sub.SHA,
	})
}

// readSubmodules returns the submodules declared in .gitmodules together with
// the commit recorded in the index of repoPath.
func (m *MirrorCloner) readSubmodules(ctx context.Context, repoPath string) ([]Submodule, error) {
	gitmodules := filepath.Join(repoPath, ".gitmodules")
	if _, err := os.Stat(gitmodules); err != nil {
		return nil, nil
	}

	out, err := m.git.Run(ctx, repoPath, "config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.(path|url)$`)
	if err != nil {
		// git config exits with 1 when nothing matches, which simply means
		// the repository has no submodules.
		return nil, nil
	}

	type entry struct {
		path string
		url  string
	}
	entries := map[string]*entry{}
	var order []string

	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		key = strings.TrimPrefix(key, "submodule.")
		idx := strings.LastIndex(key, ".")
		if idx <= 0 {
			continue
		}
		name, field := key[:idx], key[idx+1:]
		switch field {
		case "path", "url":
		default:
			continue
		}
		current, ok := entries[name]
		if !ok {
			current = &entry{}
			entries[name] = current
			order = append(order, name)
		}
		if field == "path" {
			current.path = value
		} else {
			current.url = value
		}
	}

	if len(order) == 0 {
		return nil, nil
	}

	shas, err := m.gitlinkSHAs(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	submodules := make([]Submodule, 0, len(order))
	for _, name := range order {
		current := entries[name]
		if current.path == "" || current.url == "" {
			m.log.Warnf("ignoring incomplete submodule %q", name)
			continue
		}
		submodules = append(submodules, Submodule{
			Name: name,
			Path: current.path,
			URL:  current.url,
			SHA:  shas[current.path],
		})
	}
	return submodules, nil
}

// gitlinkSHAs maps submodule paths to the commit recorded in the index.
func (m *MirrorCloner) gitlinkSHAs(ctx context.Context, repoPath string) (map[string]string, error) {
	out, err := m.git.Run(ctx, repoPath, "ls-files", "--stage")
	if err != nil {
		return nil, fmt.Errorf("reading index of %s: %w", repoPath, err)
	}

	shas := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		meta, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 3 || fields[0] != "160000" {
			continue
		}
		shas[path] = fields[1]
	}
	return shas, nil
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isEmptyDir reports whether path is a directory without entries.
func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

// isGitRepo reports whether dir contains a git repository.
func isGitRepo(ctx context.Context, git *Git, dir string) bool {
	return git.RunIn(ctx, dir, "rev-parse", "--git-dir")
}
