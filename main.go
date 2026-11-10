// mirror-clone clones repositories through a local mirror cache.
//
// Every repository is mirrored once into the mirror cache and cloned from that
// mirror afterwards. Submodules are cloned recursively through the same cache,
// so repeated clones of a project and its dependencies only transfer new
// objects.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run contains the whole program logic and returns the exit code.
func run(args []string) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mirror-clone: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'mirror-clone --help' for usage.\n")
		return 2
	}

	if opts.ShowHelp {
		fmt.Printf(usageText, defaultCacheDir())
		return 0
	}
	if opts.ShowVersion {
		fmt.Printf("mirror-clone %s\n", versionString())
		return 0
	}

	log := DefaultLogger(opts.LogLevel)

	cacheDir := opts.MirrorCacheDir
	if cacheDir == "" {
		cacheDir = os.Getenv("MIRROR_CLONE_MIRROR_CACHE_DIR")
	}
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}

	repoName := repoNameFromURL(opts.Repository)
	if repoName == "" {
		log.Errorf("cannot derive a repository name from %q", opts.Repository)
		return 1
	}

	destination := opts.Directory
	if destination == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Errorf("cannot determine the current directory: %v", err)
			return 1
		}
		destination = filepath.Join(cwd, repoName)
	}

	jobs := opts.Jobs
	if jobs < 1 {
		jobs = runtime.NumCPU()
		if jobs > 8 {
			jobs = 8
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cloner := NewMirrorCloner(NewGit(log), log, cacheDir, jobs)
	if err := cloner.Clone(ctx, CloneRequest{
		Name: repoName,
		URL:  opts.Repository,
		Dest: destination,
		Ref:  opts.Branch,
	}); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Errorf("interrupted")
			return 130
		}
		log.Errorf("%v", err)
		return 1
	}

	log.Infof("Cloned %s", filepath.ToSlash(destination))
	return 0
}

// defaultCacheDir returns the default mirror cache directory.
func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultCacheDirName
	}
	return filepath.Join(home, defaultCacheDirName)
}
