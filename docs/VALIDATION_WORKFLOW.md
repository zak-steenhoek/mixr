# Windows developer validation

Use Windows PowerShell 5.1 or PowerShell 7 with Go and Git on PATH. Run from
the repository root (the script also resolves its own root from other working
directories):

```powershell
.\pkg\mixr\scripts\windows\validate.ps1 -Mode quick
.\pkg\mixr\scripts\windows\validate.ps1 -Mode full
.\pkg\mixr\scripts\windows\validate.ps1 -Mode race
```

`quick` runs `go test -count=1 -buildvcs=false ./...` on Windows amd64.
`full` runs those tests, the existing development/release batch builds, and
the routing, audio-control, and microphone-control diagnostic builds for
386, then amd64. Build linker flags and embedded commit/version/build type
are unchanged. Diagnostics are **built, never launched**. `race` runs only
amd64 race tests; see [Windows race setup](WINDOWS_RACE_TESTING.md).

Quick/full explicitly set `GOOS=windows`, `CGO_ENABLED=0` and the selected
architecture, matching the current pure-Go Windows builds. Race sets CGO to 1.
All changed environment variables and the working directory are restored on
success or failure. No machine/user environment settings are changed. Optional
live foreground lookups are disabled for these runs; qualify them separately
using the [callback guard procedure](D0_CALLBACK_REGRESSION_GUARD.md).
`GOTOOLCHAIN=local` prevents automatic toolchain installation. Normal Go module
resolution and build caches still apply; prepare dependencies before offline
validation. No compiler or package installation command is run by the script.

## Results and artifact safety

Each invocation creates a unique, ignored `validation-results/<timestamp>-<id>-<mode>/`
directory. `summary.txt` records timestamps with timezone, Windows/PowerShell,
Go version, commit/tag, tracked dirty state, commands, exit codes and final
PASS/FAIL. Command output is captured separately. Failure stops the suite and
throws an error (nonzero when called with `powershell.exe -File` or `pwsh -File`).
Do not treat partial results as a matrix pass. Known standalone vet findings
are documented in [Windows audio hardening](WINDOWS_AUDIO_HARDENING.md);
the script does not suppress vet failures or retry with weaker flags.

```powershell
# Inspect all commands without Go/GCC/build execution or prerequisite checks:
.\pkg\mixr\scripts\windows\validate.ps1 -Mode full -Plan
# Keep both architectures' application and diagnostic binaries for later review:
.\pkg\mixr\scripts\windows\validate.ps1 -Mode full -KeepArtifacts
```

Builds go to `artifacts/386/` and `artifacts/amd64/` within that invocation.
By default only the five known executable names per architecture are removed
at the end, including after failure. Summaries/output and version metadata are
always retained. `-KeepArtifacts` preserves every generated binary as well.
Previously retained runs, root diagnostic binaries, release bundles and runtime
logs are never cleaned or overwritten. No tag, release package, configuration,
audio, serial or COM operation is performed by the validation script.

The batch builders accept optional `MIXR_BUILD_OUTPUT_DIR` (an existing directory)
for isolated output. Without it their established root executable paths are
unchanged. The wrapper sets this variable only for its child builds. Do not
use `prepare-release.bat` for validation: it deletes/recreates a supplied tag
and overwrites that release directory. Do not invoke a root batch build beside
a running root executable; use an isolated worktree and this wrapper.

## Configuration and physical validation

The wrapper never starts the application and never copies `config.yaml`.
At runtime `config.yaml` is relative to the **process working directory**, not
automatically the executable directory. For later manual validation, launch
from a dedicated directory containing its own config and executable; configure
a shortcut's **Start in** directory accordingly. Runtime `logs/` and preferences
also belong to that working directory. Do not start a second controller while
another instance owns the hardware. Do not point manual runs at soak logs or
configuration.

Build/test success does not establish hardware qualification. Follow the
applicable milestone procedure and [C2 checklist](RELIABILITY_DIAGNOSTICS.md)
for physical regression. Record commit, Windows/build, architecture, hardware,
procedure, observations and untested cases. During a hardware soak, use only
`-Plan` and tooling self-tests; defer real Go tests/builds and compiler setup.

## Track T0 audit and review scope

- `AGENTS.md` contained the repeated two-architecture command block; it now
  delegates to this entry point. Historical report commands remain evidence.
- The old loop inherited GOOS/CGO, overwrote each application's previous
  architecture, and removed GOARCH instead of restoring the caller's setting.
- `build-all.bat` does not stop if its first child fails; the wrapper calls each
  builder separately and checks the result. No release/branding scripts are
  needed for automated validation.
- Temporary diagnostic executables previously accumulated at the root; release
  cleanup risks are avoided by unique output directories and bounded cleanup.
- README placement instructions did not explain runtime working-directory
  configuration lookup. This guide and README now make that explicit.
- Tests remain beside their Go packages: `pkg/mixr` behavior, `util` foreground
  callbacks, `internal/wincom` ABI, and routing CLI parsing. No test moves or
  stylistic consolidation are needed. Tooling tests use fake commands only.
- Milestone reports, upstream documentation and decisions retain their original
  context. For new work, start at AGENTS, this guide, and the applicable report;
  do not replay historical command sequences as current setup instructions.

T0 execution evidence and physical qualification limits are in [T0 tooling review](T0_TOOLING_REVIEW.md).

To check the wrapper itself without real Go/GCC execution:

```powershell
.\pkg\mixr\scripts\windows\test-validation-tooling.ps1
```

Its stub artifacts and captures are confined to ignored `validation-results/`.
PASS in those captures refers to orchestration checks, not product validation.
