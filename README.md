# mirror-clone
 
Supports cloning repos and submodules using a cache of mirrors for performance.
 
## Installation
 
```bash
pip install git+ssh://git@github.com/fordaj/mirror-clone.git
```
 
## Usage
 
```bash
mirror-clone "[--branch, --help, .etc]" "repo name" "[dest dir]"
```
 
### Clone into current dir
 
```bash
mirror-clone "AirFrame"
```
 
### Clone into empty non-CWD dir
 
```bash
mirror-clone "AirFrame" "/c/Path/To/Dir/AirFrame"
```
 
### Clone into specific branch
 
```bash
mirror-clone --branch "main" "AirFrame" "/c/Path/To/Dir/AirFrame"
```