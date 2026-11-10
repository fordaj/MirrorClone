package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitError describes a failed git invocation. It keeps the git output so the
// caller can show the real reason (authentication, bad ref, ...) instead of a
// generic exit status.
type GitError struct {
	Args   []string
	Output string
	Err    error
}

func (e *GitError) Error() string {
	out := strings.TrimSpace(e.Output)
	if out == "" {
		return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("git %s: %v\n%s", strings.Join(e.Args, " "), e.Err, out)
}

// Unwrap exposes the underlying exec error.
func (e *GitError) Unwrap() error { return e.Err }

// Git runs git commands. Every invocation uses "git -C <dir>" so the process
// working directory is never changed and concurrent clones cannot interfere
// with each other.
type Git struct {
	bin string
	log *Logger
}

// NewGit returns a Git runner that logs every command at debug level.
func NewGit(log *Logger) *Git {
	return &Git{bin: "git", log: log}
}

// Run executes a git command inside dir and returns its trimmed stdout.
func (g *Git) Run(ctx context.Context, dir string, args ...string) (string, error) {
	full := make([]string, 0, len(args)+2)
	if dir != "" {
		full = append(full, "-C", dir)
	}
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, g.bin, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// The tool is meant to run unattended (scripts, CI, nested clones), so
	// never block on an interactive credential prompt.
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"GIT_OPTIONAL_LOCKS=0",
	)

	g.log.Debugf("git %s", strings.Join(full, " "))

	if err := cmd.Run(); err != nil {
		return stdout.String(), &GitError{
			Args:   full,
			Output: stderr.String() + stdout.String(),
			Err:    err,
		}
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

// RunIn executes a git command and reports whether it succeeded. It is used
// for probing commands where the output is irrelevant.
func (g *Git) RunIn(ctx context.Context, dir string, args ...string) bool {
	_, err := g.Run(ctx, dir, args...)
	return err == nil
}
