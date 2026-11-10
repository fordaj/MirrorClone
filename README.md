# mirror-clone

Clones repositories and their submodules through a local cache of mirrors, so
repeated clones only transfer the objects that are actually new.

The tool is written in Go and ships as a single, dependency free binary for
Linux, macOS and Windows (x86_64 and arm64).

## How it works

1. The repository is mirrored once into the mirror cache
   (`~/.mirror-clones` by default).
2. The working copy is cloned from that mirror, so the objects are not
   downloaded twice.
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

| Option                   | Description                                                        |
| ------------------------ | ------------------------------------------------------------------ |
| `-b`, `--branch NAME`    | Check out `NAME` after cloning instead of the remote HEAD.          |
| `--mirror-cache-dir DIR` | Directory of the mirror cache (default: `~/.mirror-clones`).         |
| `-j`, `--jobs N`         | Maximum number of submodules cloned in parallel.                    |
| `--log-level LEVEL`      | Minimum severity printed: `error`, `warn`, `info`, `debug`.         |
| `-q`, `--quiet`          | Same as `--log-level error`.                                        |
| `-v`, `--version`        | Print the version and exit.                                         |
| `-h`, `--help`           | Print the usage and exit.                                           |

The cache directory can also be set with the `MIRROR_CLONE_MIRROR_CACHE_DIR`
environment variable.

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

## Testing

```bash
./test.bash                 # gofmt, go vet, go build, go test, smoke test, shellcheck
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
