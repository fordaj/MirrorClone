# mirror-clone
 
Supports cloning repos and submodules using a cache of mirrors for performance.
 
## Installation

Build from source with Go (1.22+):

```bash
./build.bash
# binaries are written to ./dist/
```

Or build a single binary:

```bash
go build -o mirror-clone ./cmd/mirror-clone
```
 
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