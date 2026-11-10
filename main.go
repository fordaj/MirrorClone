package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// toolVersion is the released version. It is injected at build time with
//
//	-ldflags "-X main.toolVersion=<tag>"
//
// and defaults to "dev" for local, unstamped builds.
var toolVersion = "dev"

// mirrorFetchRefSpec mirrors the source's branches into the mirror's own
// refs/heads/* so the mirror can be cloned from like a regular repository.
//
// go-git's default refspec ("+refs/heads/*:refs/remotes/origin/*") stores
// the branches under refs/remotes/origin/* and leaves the mirror's HEAD
// pointing at a branch that does not exist, which makes a clone from the
// mirror fail with: couldn't find remote ref "HEAD".
const mirrorFetchRefSpec = "+refs/heads/*:refs/heads/*"

// syncMirror makes sure the bare mirror at mirrorPath holds the refs of
// repoURL and that the mirror's HEAD points at the source's default
// branch, so that a clone from the mirror resolves HEAD.
func syncMirror(mirrorPath string, repoURL string) (*git.Repository, error) {
	repo, err := git.PlainOpen(mirrorPath)
	if err != nil {
		// No mirror yet: create a bare one.
		repo, err = git.PlainInit(mirrorPath, true)
		if err != nil {
			return nil, err
		}
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		remote, err = repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{repoURL},
			Fetch: []config.RefSpec{mirrorFetchRefSpec},
		})
		if err != nil {
			return nil, err
		}
	} else {
		// Existing mirror: switch it to the mirror refspec.
		remote.Config().Fetch = []config.RefSpec{mirrorFetchRefSpec}
	}

	if err := remote.Fetch(&git.FetchOptions{}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil, err
	}

	// Point the mirror's HEAD at the source's default branch.
	refs, err := remote.List(&git.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD && ref.Type() == plumbing.SymbolicReference {
			if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref.Target())); err != nil {
				return nil, err
			}
			break
		}
	}

	// Drop the refs/remotes/origin/* layout left behind by older versions
	// so the mirror only holds its own refs/heads/* plus tags.
	if err := pruneRemoteRefs(repo); err != nil {
		return nil, err
	}

	return repo, nil
}

// pruneRemoteRefs removes the refs/remotes/* refs of a mirror.
func pruneRemoteRefs(repo *git.Repository) error {
	iter, err := repo.References()
	if err != nil {
		return err
	}
	var stale []plumbing.ReferenceName
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsRemote() {
			stale = append(stale, ref.Name())
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, name := range stale {
		if err := repo.Storer.RemoveReference(name); err != nil && err != plumbing.ErrReferenceNotFound {
			return err
		}
	}
	return nil
}

// rel returns the relative path of p to the current working directory.
func rel(path string) string {
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(absCwd, p)
	if err != nil {
		return path
	}
	return rel
}

type CloneOptions struct {
	Repository string
	Directory  string
	Branch     string
}

// main entry point for the mirror-clone tool
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: mirror-clone [--branch NAME] [--version] REPO [DEST_DIR]\n")
		os.Exit(1)
	}

	// Check for --help/-h early
	if os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Fprintf(os.Stderr, "Usage: mirror-clone [--branch NAME] [--version] REPO [DEST_DIR]\n")
		os.Exit(0)
	}

	// Check for --version early
	if os.Args[1] == "--version" {
		fmt.Println(toolVersion)
		os.Exit(0)
	}

	// Simple argument parsing
	repoURL := ""
	destDir := ""
	branch := ""

	args := os.Args[1:]
	i := 0
	for i < len(args) {
		if (args[i] == "--branch" || args[i] == "-b") && i+1 < len(args) {
			branch = args[i+1]
			i += 2
		} else if args[i] == "--help" || args[i] == "-h" {
			fmt.Fprintf(os.Stderr, "Usage: mirror-clone [--branch NAME] [--version] REPO [DEST_DIR]\n")
			os.Exit(0)
		} else if repoURL == "" {
			repoURL = args[i]
			i++
		} else if destDir == "" {
			destDir = args[i]
			i++
		} else {
			fmt.Fprintf(os.Stderr, "Error: unexpected argument: %s\n", args[i])
			os.Exit(1)
		}
	}

	if repoURL == "" {
		fmt.Fprintf(os.Stderr, "Error: repository URL is required\n")
		os.Exit(1)
	}

	// Get repo name from URL
	repoName := strings.TrimSuffix(filepath.Base(repoURL), ".git")

	// Set default destination directory
	if destDir == "" {
		destDir = repoName
	}

	// Mirror cache directory
	mirrorDir := filepath.Join(os.Getenv("HOME"), ".mirror-clones")

	// Ensure destination and mirror directories exist
	os.MkdirAll(destDir, 0755)
	os.MkdirAll(mirrorDir, 0755)

	// Cloning into a non-empty directory fails with a cryptic
	// "repository already exists" error; say what actually went wrong.
	if entries, err := os.ReadDir(destDir); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "Error: destination directory %s already exists and is not empty\n", destDir)
		os.Exit(1)
	}

	fmt.Printf("Cloning %s into %s\n", repoURL, destDir)

	// Clone or fetch mirror
	mirrorPath := filepath.Join(mirrorDir, repoName+".git")
	if _, err := os.Stat(mirrorPath); !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Fetching latest in %s\n", mirrorPath)
	} else {
		fmt.Printf("Mirror cloning %s\n", repoURL)
	}
	mirrorRepo, syncErr := syncMirror(mirrorPath, repoURL)
	if syncErr != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to update mirror: %v\n", syncErr)
		os.Exit(1)
	}

	// Clone from mirror to destination
	cloneOpts := &git.CloneOptions{
		URL:          mirrorPath,
		Progress:     os.Stdout,
		SingleBranch: true,
		NoCheckout:   false,
	}

	// Only specify reference name if a branch was explicitly provided
	if branch != "" {
		refName := plumbing.NewBranchReferenceName(branch)
		cloneOpts.ReferenceName = refName
	}

	_, err := git.PlainClone(destDir, false, cloneOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to clone from mirror: %v\n", err)
		os.Exit(1)
	}

	// Handle submodules using git CLI
	// destDir is the directory where the repo was cloned (may already include repo name)
	err = recursiveSubmodules(destDir, mirrorDir, mirrorRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: submodule handling failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Cloned %s successfully\n", destDir)
}

// recursiveSubmodules handles submodule cloning recursively using git CLI
func recursiveSubmodules(workDir, mirrorDir string, mirrorRepo *git.Repository) error {
	// git submodule init
	cmd := exec.Command("git", "submodule", "init")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git submodule init failed: %v: %s", err, string(output))
	}
	fmt.Printf("Submodule init: %s", string(output))

	// Get submodule status
	cmd = exec.Command("git", "submodule", "status")
	cmd.Dir = workDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git submodule status failed: %v: %s", err, string(output))
	}

	statusOutput := string(output)
	lines := strings.Split(statusOutput, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: <submodule-path> <sha> <url> (checked out at <sha>)
		// Or: <submodule-path> <sha> <url>
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		subPath := fields[0]
		subSHA := fields[1]
		var subURL string
		if len(fields) > 2 {
			subURL = fields[2]
		} else {
			continue
		}

		// Skip submodules with empty paths
		if subPath == "" {
			continue
		}

		subFullPath := filepath.Join(workDir, subPath)
		subModuleName := strings.TrimSuffix(filepath.Base(subURL), ".git")

		// Determine mirror path for the submodule
		smMirrorPath := filepath.Join(mirrorDir, subModuleName+".git")

		// Mirror the submodule the same way as the top level repo, so the
		// mirror layout (refs/heads/* plus a valid HEAD) is consistent.
		if _, statErr := os.Stat(smMirrorPath); statErr == nil {
			fmt.Printf("Using existing submodule mirror %s\n", smMirrorPath)
		} else {
			fmt.Printf("Mirror cloning submodule %s\n", subURL)
		}
		if _, mirrorErr := syncMirror(smMirrorPath, subURL); mirrorErr != nil {
			return fmt.Errorf("failed to update submodule mirror for %s: %v", subPath, mirrorErr)
		}

		// Update the submodule to the correct SHA
		fmt.Printf("Updating submodule %s to %s\n", subPath, subSHA)
		cmd = exec.Command("git", "submodule", "update", "--init", "--progress", subPath)
		cmd.Dir = workDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git submodule update failed for %s: %v: %s", subPath, err, string(output))
		}
		fmt.Printf("Submodule update: %s", string(output))

		// Recursively handle submodules of this submodule
		err = recursiveSubmodules(subFullPath, mirrorDir, mirrorRepo)
		if err != nil {
			return err
		}
	}

	return nil
}
