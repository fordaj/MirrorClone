# mirror-clone

Clones repositories and their submodules through a local cache of mirrors, so
repeated clones only transfer the objects that are actually new.

The tool is written in Go and ships as a single, dependency free binary for
Linux, macOS and Windows (x86_64 and arm64).

## How it works

1. The repository is mirrored once into the mirror cache
   (`~/.mirror-clones` by default).
2. The working copy is cloned from that mirror, so the objects are not
   downloaded twice. The borrowed objects are copied into the new repository,
   so the clone keeps working when the cache is removed.
3. Every submodule is mirrored and cloned the same way. Relative submodule
   URLs (`../lib.git`) are resolved against the URL of the superproject, just
   like git does it.

The mirror of a repository is refreshed (`git fetch --prune`) whenever it is
used again, so the cache never goes stale.

## Installation

Download the archive for your platform from the
[releases](https://github.com/fordaj/MirrorClone/releases) page and unpack it:

```bash
unzip mirror-clone_<version>_<platform>.zip
```

The archive contains the binary, the `LICENSE` and the `README.md`. Put the
binary somewhere on your `PATH` and you are done.

The binary can also install itself, see [Self management](#self-management).

## Usage

```bash
mirror-clone "[--branch, --help, .etc]" "repo name" "[dest dir]"
```

### Clone into current dir

```bash
mirror-clone "git@github.com:fordaj/AirFrame.git"
```

### Clone into empty non-CWD dir

```bash
mirror-clone "git@github.com:fordaj/AirFrame.git" "/c/Path/To/Dir/AirFrame"
```

### Clone into specific branch

```bash
mirror-clone --branch "main" "git@github.com:fordaj/AirFrame.git" "/c/Path/To/Dir/AirFrame"
```

### Options

| Option                   | Description                                                  |
| ------------------------ | ------------------------------------------------------------ |
| `-b`, `--branch NAME`    | Check out `NAME` after cloning instead of the remote HEAD.   |
| `--mirror-cache-dir DIR` | Directory of the mirror cache (default: `~/.mirror-clones`). |
| `-j`, `--jobs N`         | Maximum number of submodules cloned in parallel.             |
| `--log-level LEVEL`      | Minimum severity printed: `error`, `warn`, `info`, `debug`.  |
| `-q`, `--quiet`          | Same as `--log-level error`.                                 |
| `-v`, `--version`        | Print the version and exit.                                  |
| `-h`, `--help`           | Print the usage and exit.                                    |

The cache directory can also be set with the `MIRROR_CLONE_MIRROR_CACHE_DIR`
environment variable.

## Self management

The tool can install, update and remove itself:

```bash
mirror-clone install
mirror-clone update
mirror-clone uninstall
```

### install

Copies the running executable into the executable directory of the user:

| Platform     | Directory                              |
| ------------ | -------------------------------------- |
| Linux, macOS | `~/.local/bin`                         |
| Windows      | `%LOCALAPPDATA%\Programs\mirror-clone` |

The directory is created when it is missing and the copy is made
executable. No administrator privileges are needed. When the directory is
not on the `PATH`, the command prints the line that has to be added to the
shell configuration; the configuration itself is never changed.

### update

Reads the latest stable release of the repository from the GitHub API,
downloads the archive of the current platform and verifies its SHA256
checksum against the `checksums.txt` of the release. The installed
executable is replaced with the verified one.

An installation that is newer than the latest release is never downgraded.
When the download, the verification or the replacement fails, the installed
executable is kept.

### uninstall

Removes the installed executable. The installation directory is only removed
when it does not contain any other file. The configuration and the mirror
cache of the user are never touched.

## Running the built binary

`run.bash` runs the binary that was built for the current machine. The
platform is resolved with the environment cascade (`scripts/env.sh`), the same
way the bundled tools are resolved, and every argument is passed straight to
the binary:

```bash
./run.bash --branch main git@github.com:fordaj/AirFrame.git
./run.bash --help
DEBUG=true ./run.bash --help        # runs the debug build
```

The binary is looked up in this order:

1. `$MIRROR_CLONE_BIN`
2. `dist/<platform>/mirror-clone[.exe]` (release build)
3. `dist/debug/<platform>/mirror-clone[.exe]` (`DEBUG=true`)

## Building from source

The build scripts use the bundled [logamuffin](https://github.com/fordaj/logamuffin)
for logging and the bundled [versioner](https://github.com/fordaj/Versioner)
to compute the version of the binaries.

```bash
./build.bash                # release build for the host platform
./build.bash --debug        # debug build for the host platform
./build.bash --all          # release build for every platform
./build.bash --platform linux-arm64
```

The binaries are written to `dist/<platform>/mirror-clone[.exe]`, debug
builds go to `dist/debug/<platform>/`.

`DEBUG=true` is the same as `--debug`.

## Formatting

The sources are formatted with [formattington](https://github.com/fordaj/formattington),
which drives the formatters of the project from the configuration in
`.formattington.json`:

| Formatter   | Files                        |
| ----------- | ---------------------------- |
| `prettier`  | Markdown, JSON               |
| `shfmt`     | Shell scripts                |
| `taplo`     | TOML                         |
| `gofmt`     | Go sources                   |
| `goimports` | Go sources and their imports |

```bash
./format.bash                # format every source of the repository
./format.bash --check        # change nothing, fail when a file needs formatting
./format.bash --dry-run      # print the differences, change nothing
./format.bash --tool shfmt   # run a single formatter
```

Formattington is installed with [uv](https://docs.astral.sh/uv/). The
environment is created automatically by `format.bash`, and can be set up by
hand with:

```bash
uv sync
```

`uv.lock` is checked in, so `uv sync` installs exactly the locked versions.
The formatters themselves have to be installed separately:

- `prettier` and `shfmt` come from npm and the Go toolchain
- `taplo` comes from its own releases
- `gofmt` comes with the Go installation
- `goimports` is installed with
  `go install golang.org/x/tools/cmd/goimports@latest`

The formatters run one after another, because `gofmt` and `goimports` write
to the same files and would race against each other when they run
concurrently. `format.bash --parallel` opts out of that.

## Testing

```bash
./test.bash                 # gofmt, go vet, go build, go test, smoke test, shellcheck, formatting
./test.bash --coverage      # also write coverage.out
```

## Releasing

```bash
./release.bash --dry-run    # show the plan, change nothing
./release.bash              # build, package, tag and publish
```

`release.bash` computes the version and the tag with `versioner`, cross
compiles every platform with `build.bash`, packages the binary, the `LICENSE`
and the `README.md` into one archive per platform, creates the annotated tag,
pushes it and uploads the archives to the GitHub release of that tag.

## License

[MIT](LICENSE)
