## Developer scripts

This document lists the various scripts in the project and their purposes.

> Note: All scripts are meant to be run from the root of the mixr repository: `.\pkg\mixr\scripts\...\whatever.bat`. They're not guaranteed to work correctly if run from another directory.

### Windows

- [`validate.ps1`](./windows/validate.ps1): Repeatable quick/full/race checks with isolated outputs and timestamped results. See the [validation workflow](../../../docs/VALIDATION_WORKFLOW.md) and [race setup](../../../docs/WINDOWS_RACE_TESTING.md). `-Plan` previews commands without Go/GCC execution.
- [`test-validation-tooling.ps1`](./windows/test-validation-tooling.ps1): Lightweight wrapper regression checks using fake Go/GCC commands; no application, diagnostic, or real compiler execution.
- [`build-dev.bat`](./windows/build-dev.bat): Builds `mixr-dev.exe` with a console window for development. Optional `MIXR_BUILD_OUTPUT_DIR` redirects output to an existing directory.
- [`build-release.bat`](./windows/build-release.bat): Builds `mixr-release.exe` as a tray application without a console window; supports the same output override.
- [`build-all.bat`](./windows/build-all.bat): Helper script to build all variants
- [`generate-branding.bat`](./windows/generate-branding.bat): Regenerates the multi-resolution ICO and Go tray/menu icon bytes from the editable SVG and existing menu icons, then regenerates the architecture-tagged 386/amd64 Windows resources. Outputs are deterministic.
- [`make-rsrc.bat`](./windows/make-rsrc.bat): Compatibility wrapper for `generate-branding.bat`.
- [`prepare-release.bat`](./windows/prepare-release.bat): Tags, builds and renames the release binaries in preparation for a GitHub release. Usage: `prepare-release.bat vX.Y.Z` (binaries will be under `releases\vX.Y.Z\`)

### Linux

- [`build-dev.sh`](./linux/build-dev.sh): Builds `mixr-dev` for development
- [`build-release.sh`](./linux/build-release.sh): Builds `mixr-release` for releases
- [`build-all.sh`](./linux/build-all.sh): Helper script to build all variants
