package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fordaj/MirrorClone/internal/mirrorclone"
)

func main() {
	var branch string
	var mirrorCacheDir string
	flag.StringVar(&branch, "branch", "", "Checkout <NAME> branch or ref after cloning instead of the remote's HEAD.")
	flag.StringVar(&branch, "b", "", "Checkout <NAME> branch or ref after cloning instead of the remote's HEAD.")
	flag.StringVar(&mirrorCacheDir, "mirror-cache-dir", "", "Directory used to store bare mirror caches (default: ~/.mirror-clones).")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: mirror-clone [--branch NAME] [--mirror-cache-dir DIR] <repository> [directory]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nClone a repository into a new directory using a local mirror cache.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "A mirror is maintained per repository so subsequent clones/fetches are faster.\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 || len(args) > 2 {
		flag.Usage()
		os.Exit(2)
	}

	repoURL := args[0]
	repoName := mirrorclone.RepoNameFromURL(repoURL)

	var destDir string
	if len(args) == 2 {
		destDir = args[1]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		destDir = filepath.Join(cwd, repoName)
	}

	if mirrorCacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		mirrorCacheDir = filepath.Join(home, ".mirror-clones")
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create destination directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(mirrorCacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create mirror cache directory: %v\n", err)
		os.Exit(1)
	}

	if err := mirrorclone.MirrorClone(repoName, repoURL, mirrorCacheDir, destDir, branch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Cloned %s\n", filepath.ToSlash(destDir))
	_ = strings.TrimSpace("") // keep strings import if needed
}
