package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Command is a self management command of the tool. The empty command is
// the default one, which clones a repository.
type Command string

// The self management commands of the tool.
const (
	// CommandClone clones a repository through the mirror cache.
	CommandClone Command = ""
	// CommandInstall installs the tool for the user.
	CommandInstall Command = "install"
	// CommandUninstall removes the tool of the user.
	CommandUninstall Command = "uninstall"
	// CommandUpdate updates the tool to the latest stable release.
	CommandUpdate Command = "update"
)

// commandByName returns the command with the given name.
func commandByName(name string) (Command, bool) {
	switch Command(name) {
	case CommandInstall, CommandUninstall, CommandUpdate:
		return Command(name), true
	default:
		return CommandClone, false
	}
}

// Options holds the parsed command line options.
type Options struct {
	// Command is the self management command to run. It is empty when a
	// repository is cloned.
	Command Command
	// Repository is the repository that is cloned.
	Repository string
	// Directory is the destination of the working copy. It defaults to the
	// repository name inside the current working directory.
	Directory string
	// Branch is an optional branch or commit checked out after cloning.
	Branch string
	// MirrorCacheDir is the directory holding the mirror cache.
	MirrorCacheDir string
	// Jobs limits how many submodules are cloned in parallel.
	Jobs int
	// LogLevel is the minimum severity that is printed.
	LogLevel Level
	// ShowVersion requests the version output.
	ShowVersion bool
	// ShowHelp requests the usage output.
	ShowHelp bool
}

const usageText = `mirror-clone - clone repositories through a local mirror cache

Usage:
  mirror-clone [options] <repository> [directory]
  mirror-clone install
  mirror-clone uninstall
  mirror-clone update

Commands:
  install     Install mirror-clone into the executable directory of the user.
  uninstall   Remove the installed mirror-clone.
  update      Update mirror-clone to the latest stable release.

Arguments:
  repository            Repository to clone (URL or path).
  directory             Optional destination directory (defaults to the
                        repository name in the current directory).

Options:
  -b, --branch NAME      Checkout NAME after cloning instead of the remote HEAD.
      --mirror-cache-dir DIR
                        Directory used for the mirror cache
                        (default: %s).
  -j, --jobs N          Maximum number of submodules cloned in parallel
                        (default: number of CPUs).
      --log-level LEVEL  Minimum severity printed: error, warn, info, debug
                        (default: info).
  -q, --quiet            Same as --log-level error.
  -v, --version          Print the version and exit.
  -h, --help             Print this help and exit.

Examples:
  mirror-clone git@github.com:fordaj/AirFrame.git
  mirror-clone --branch main git@github.com:fordaj/AirFrame.git /tmp/AirFrame
  mirror-clone install
`

// defaultCacheDirName is the name of the mirror cache directory inside the
// home directory of the user.
const defaultCacheDirName = ".mirror-clones"

// parseArgs parses the command line. Flags may appear before or after the
// positional arguments, "--" ends the flag parsing.
func parseArgs(args []string) (Options, error) {
	opts := Options{LogLevel: LevelInfo}

	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)

		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			next := func(flag string) (string, error) {
				if hasValue {
					return value, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("flag --%s requires a value", flag)
				}
				i++
				return args[i], nil
			}

			switch name {
			case "branch":
				v, err := next(name)
				if err != nil {
					return opts, err
				}
				opts.Branch = v
			case "mirror-cache-dir":
				v, err := next(name)
				if err != nil {
					return opts, err
				}
				opts.MirrorCacheDir = v
			case "jobs":
				v, err := next(name)
				if err != nil {
					return opts, err
				}
				jobs, err := strconv.Atoi(v)
				if err != nil {
					return opts, fmt.Errorf("invalid value %q for --jobs: %w", v, err)
				}
				if jobs < 1 {
					return opts, fmt.Errorf("invalid value %q for --jobs: must be at least 1", v)
				}
				opts.Jobs = jobs
			case "log-level":
				v, err := next(name)
				if err != nil {
					return opts, err
				}
				opts.LogLevel = ParseLevel(v)
			case "quiet":
				opts.LogLevel = LevelError
			case "version":
				opts.ShowVersion = true
			case "help":
				opts.ShowHelp = true
			default:
				return opts, fmt.Errorf("unknown flag: --%s", name)
			}

		case len(arg) > 1 && strings.HasPrefix(arg, "-"):
			short := arg[1:2]
			rest := arg[2:]
			rest = strings.TrimPrefix(rest, "=")

			switch short {
			case "b":
				if rest == "" {
					if i+1 >= len(args) {
						return opts, fmt.Errorf("flag -b requires a value")
					}
					i++
					rest = args[i]
				}
				opts.Branch = rest
			case "j":
				if rest == "" {
					if i+1 >= len(args) {
						return opts, fmt.Errorf("flag -j requires a value")
					}
					i++
					rest = args[i]
				}
				jobs, err := strconv.Atoi(rest)
				if err != nil {
					return opts, fmt.Errorf("invalid value %q for -j: %w", rest, err)
				}
				if jobs < 1 {
					return opts, fmt.Errorf("invalid value %q for -j: must be at least 1", rest)
				}
				opts.Jobs = jobs
			case "q":
				opts.LogLevel = LevelError
			case "v":
				opts.ShowVersion = true
			case "h":
				opts.ShowHelp = true
			default:
				return opts, fmt.Errorf("unknown flag: -%s", short)
			}

		default:
			positional = append(positional, arg)
		}
	}

	// The first positional argument is the command, when it is one of the
	// self management commands.
	if len(positional) > 0 {
		if command, ok := commandByName(positional[0]); ok {
			opts.Command = command
			positional = positional[1:]
			if len(positional) > 0 {
				return opts, fmt.Errorf("the %s command does not take arguments", command)
			}
			return opts, nil
		}
	}

	switch len(positional) {
	case 0:
		if opts.ShowHelp || opts.ShowVersion {
			return opts, nil
		}
		return opts, fmt.Errorf("no repository given")
	case 1:
		opts.Repository = positional[0]
	default:
		opts.Repository = positional[0]
		opts.Directory = positional[1]
		if len(positional) > 2 {
			return opts, fmt.Errorf("too many arguments: %s", strings.Join(positional[2:], " "))
		}
	}

	return opts, nil
}
