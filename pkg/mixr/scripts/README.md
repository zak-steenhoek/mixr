## Developer scripts

This document lists the various scripts in the project and their purposes.

> Note: All scripts are meant to be run from the root of the repository, i.e. from the _root_ `deej` directory: `.\pkg\deej\scripts\...\whatever.bat`. They're not guaranteed to work correctly if run from another directory.

### Windows

- [`build-dev.bat`](./windows/build-dev.bat): Builds `mixr-dev.exe` with a console window for development
- [`build-release.bat`](./windows/build-release.bat): Builds `mixr-release.exe` as a tray application without a console window
- [`build-all.bat`](./windows/build-all.bat): Helper script to build all variants
- [`make-icon.bat`](./windows/make-icon.bat): Converts a .ico file to an icon byte array in a Go file. Used by our systray library. You shouldn't need to run this unless you change the deej logo
- [`make-rsrc.bat`](./windows/make-rsrc.bat): Generates architecture-tagged `rsrc_windows_386.syso` and `rsrc_windows_amd64.syso` files inside `cmd` alongside `main.go`. The Go linker selects the matching mixr application manifest and icon resource for each Windows target.
- [`prepare-release.bat`](./windows/prepare-release.bat): Tags, builds and renames the release binaries in preparation for a GitHub release. Usage: `prepare-release.bat vX.Y.Z` (binaries will be under `releases\vX.Y.Z\`)

### Linux

- [`build-dev.sh`](./linux/build-dev.sh): Builds `mixr-dev` for development
- [`build-release.sh`](./linux/build-release.sh): Builds `mixr-release` for releases
- [`build-all.sh`](./linux/build-all.sh): Helper script to build all variants
