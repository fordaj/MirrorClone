import threading, argparse
from git import Repo
from git.exc import GitCommandError
from pathlib import Path
from pathlib import PurePosixPath
from concurrent.futures import ThreadPoolExecutor


def rel(path: Path) -> str:
    return path.relative_to(Path.cwd()).as_posix()


def parse_args():
    parser = argparse.ArgumentParser(
        prog="git-clone",
        description=(
            "Clone a repository into a new directory.\n\nA 'mirror' clone is similar to a bare clone, but it also copies all refs (including remote-tracking branches and notes) and sets up remote configuration so that 'git fetch' and 'git push' will update all refs. This is useful for maintaining a full backup or mirror of another repository."
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--branch", "-b", metavar="NAME", help="Checkout <NAME> branch after cloning instead of the remote's HEAD.")
    parser.add_argument("repository", help="The (possibly remote) repository to clone.")
    parser.add_argument("directory", nargs="?", help="Optional directory to clone into (defaults to the repo name).")
    parser.add_argument("--mirror-cache-dir", help="Checkout <NAME> branch after cloning instead of the remote's HEAD.")
    return parser.parse_args()


def mirror_clone(repo_name: str, repo_url: str, mirror_dir: Path, clone_dir: Path = Path.cwd(), sha: str = ""):
    try:
        mirror_repo: Path = mirror_dir / (repo_name + ".git")
        repo_path = clone_dir

        if mirror_repo.exists():
            print(f"Fetching latest in {mirror_repo.as_posix()}")
            Repo(mirror_repo).remotes.origin.fetch(prune=True)
        else:
            print(f"Mirror cloning {mirror_repo.as_posix()}")
            Repo.clone_from(repo_url, mirror_repo, mirror=True)

        print(f"Cloning {repo_path.as_posix()} from {mirror_repo.as_posix()}")
        repo = Repo.clone_from(repo_url, repo_path, reference=mirror_repo)

        if sha:
            print(f"Checking out {sha}")
            repo.git.checkout(sha)

        if repo.submodules:
            with ThreadPoolExecutor(max_workers=len(repo.submodules)) as executor:
                futures = []

                for submodule in repo.submodules:
                    name = Path(submodule.path).name
                    url = submodule.url
                    sub_sha = submodule.hexsha
                    dest = repo_path / Path(submodule.path)

                    futures.append(
                        executor.submit(
                            mirror_clone,
                            name,
                            url,
                            mirror_dir,
                            dest,
                            sub_sha,
                        )
                    )

                for future in futures:
                    future.result()
    except GitCommandError:
        print("\033[31mAn error occurred while running a git command:\033[0m")
        raise

def main():
    args = parse_args()

    url:str = args.repository
    # Get repo name from URL
    REPO_NAME = PurePosixPath(url.rstrip("/")).name
    if REPO_NAME.endswith(".git"):
        REPO_NAME = REPO_NAME[:-4]

    # Parse directory or default to CWD
    if args.directory is not None:
        dest_dir = Path(args.directory)
    else:
        dest_dir = Path.cwd() / REPO_NAME
    dest_dir.mkdir(parents=True, exist_ok=True)
    MIRRORS = Path.home() / ".mirror-clones" # TODO make this an arg
    BRANCH = args.branch or ""

    mirror_clone(REPO_NAME, url, MIRRORS, dest_dir, sha=BRANCH)
    print(f"Cloned {(dest_dir).as_posix()}")


if __name__ == "__main__":
    main()
