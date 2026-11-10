# mirror-clone
 
Supports cloning repos and submodules using a cache of mirrors for performance.
 
## Installation
 
```bash
pip install git+ssh://git@github.com/fordaj/MirrorClone.git
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