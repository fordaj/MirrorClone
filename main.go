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
//   -ldflags "-X main.toolVersion=<tag>"
// and defaults to "dev" for local, unstamped builds.
var toolVersion = "dev"

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

	// Ensure destination directory exists
	os.MkdirAll(destDir, 0755)

	fmt.Printf("Cloning %s into %s\n", repoURL, destDir)

	// Clone or fetch mirror
	mirrorPath := filepath.Join(mirrorDir, repoName+".git")
	var mirrorRepo *git.Repository

	if _, err := os.Stat(mirrorPath); !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Fetching latest in %s\n", mirrorPath)
		var openErr error
		mirrorRepo, openErr = git.PlainOpen(mirrorPath)
		if openErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to open mirror: %v\n", openErr)
			os.Exit(1)
		}
		fetchErr := mirrorRepo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
		})
		if fetchErr != nil && fetchErr.Error() != "git: no changes to fetch" {
			fmt.Fprintf(os.Stderr, "Warning: mirror fetch had issues: %v\n", fetchErr)
		}
	} else {
		fmt.Printf("Mirror cloning %s\n", repoURL)
		var cloneErr error
		mirrorRepo, cloneErr = git.PlainInit(mirrorPath, true)
		if cloneErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to init mirror: %v\n", cloneErr)
			os.Exit(1)
		}
		// Add remote and fetch
		_, err := mirrorRepo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{repoURL},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to add remote: %v\n", err)
			os.Exit(1)
		}
		err = mirrorRepo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to fetch mirror: %v\n", err)
			os.Exit(1)
		}
	}

	// Clone from mirror to destination
	cloneOpts := &git.CloneOptions{
		URL:      mirrorPath,
		Progress: os.Stdout,
		SingleBranch: true,
		NoCheckout: false,
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
	cmd = exec.Command("git", "submodule", "status", workDir)
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
		subModuleName := filepath.Base(subURL)

		// Determine mirror path for the submodule
		smMirrorPath := filepath.Join(mirrorDir, subModuleName+".git")

		// Check if mirror exists
		smExists := false
		if _, err := os.Stat(smMirrorPath); err == nil {
			smExists = true
		}

		if smExists {
			fmt.Printf("Using existing submodule mirror %s\n", smMirrorPath)
			// Fetch updates
			smMirrorRepo, err := git.PlainOpen(smMirrorPath)
			if err != nil {
				return fmt.Errorf("failed to open submodule mirror: %v", err)
			}
			err = smMirrorRepo.Fetch(&git.FetchOptions{
				RemoteName: "origin",
			})
			if err != nil && err.Error() != "git: no changes to fetch" {
				fmt.Fprintf(os.Stderr, "Warning: submodule mirror fetch had issues: %v\n", err)
			}
		} else {
			fmt.Printf("Mirror cloning submodule %s\n", subURL)
			_, err = git.PlainInit(smMirrorPath, true)
			if err != nil {
				return fmt.Errorf("failed to init submodule mirror: %v", err)
			}
			// Add remote and fetch
			smMirrorRepo, err := git.PlainOpen(smMirrorPath)
			if err != nil {
				return fmt.Errorf("failed to open submodule mirror: %v", err)
			}
			_, err = smMirrorRepo.CreateRemote(&config.RemoteConfig{
				Name: "origin",
				URLs: []string{subURL},
			})
			if err != nil {
				return fmt.Errorf("failed to add submodule remote: %v", err)
			}
			err = smMirrorRepo.Fetch(&git.FetchOptions{
				RemoteName: "origin",
			})
			if err != nil && err.Error() != "git: no changes to fetch" {
				fmt.Fprintf(os.Stderr, "Warning: submodule mirror fetch had issues: %v\n", err)
			}
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