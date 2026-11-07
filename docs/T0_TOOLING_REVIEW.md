# Track T0 tooling review

Date: 2026-11-06 (America/Phoenix). Base: verified main `627feec`.
Branch: `tooling/validation-workflow`, isolated `.worktrees/validation-workflow`.
The D0 milestone tag and historical qualification reports were not changed.

## Safe checks performed during the mainline soak

- Parsed the PowerShell entry point; `git diff --check` passed.
- Ran `test-validation-tooling.ps1` with **PowerShell 7.6.5** and **Windows
  PowerShell 5.1**, on Windows kernel/build **10.0.26200**. Both passed.
- Used fake Go and GCC commands (verified first on PATH), real Git metadata,
  and the actual batch builders. No real Go test/build or GCC invocation occurred.
- Checked all three command plans; plans do not invoke Go or GCC and are
  explicitly labeled PLAN ONLY, never validation passes.
- Checked both architecture command sequences, existing test flags, release
  linker flags and embedded version parameters, and isolated filenames.
- Checked ten retained build placeholders, default cleanup, preservation of
  earlier retained outputs, and cleanup after a simulated release-build failure.
- Checked immediate stop on native test and batch failures, nonzero race-test
  failure propagation, missing GCC, incorrect GCC target and absent synchronization
  library guidance. Successful native stderr output is retained without being
  mistaken for an exit-code failure on either PowerShell version.
- Checked working-directory and process environment restoration after success,
  planned runs and failure; timestamped PASS/FAIL/PLAN summaries were produced.

Local evidence is ignored under the tooling worktree's `validation-results/`:
`self-test-pwsh.txt`, `self-test-windows-powershell.txt`, per-run command captures,
summaries, and clearly labeled fake tooling-test artifacts. These are not D0
runtime logs or qualification evidence.

No running-process control, application/diagnostic execution, configuration
change, COM/serial/audio/hardware interaction, dependency change, firmware
change, compiler installation, runtime-log access or release-bundle cleanup
was performed. Main's tracked files and branch were left unchanged.

## Deferred until the soak ends

Run quick and full validation with the real installed Go toolchain; retain full
outputs if needed for review. Set up UCRT64 GCC manually, check compatibility,
then run race validation. These actual execution checks are **not claimed as
passes** by T0. Any physical qualification required by later production changes
remains separate; this documentation/tooling change does not qualify D0 again.

## Compatibility and integration

The existing Windows 386/amd64 pure-Go build path, batch commands, linker flags,
resource selection and version injection are retained. The only batch interface
addition is optional `MIXR_BUILD_OUTPUT_DIR`; default output names remain the
same. The new wrapper targets Windows PowerShell 5.1 and PowerShell 7. Race
testing is Windows amd64 only, with CGO and a compatible MinGW-w64 GCC.

No production Go, tests for application behavior, firmware, configuration,
dependencies or architecture decisions changed. Main was still `627feec` at
review. Documentation entry points (`AGENTS.md`, README and contributing/scripts
guides), `.gitignore`, or batch builder edits made concurrently could need
ordinary conflict resolution. No merge, push or tag was performed.

## Preparation after mainline configuration correction

On 2026-11-06 (America/Phoenix), local `main`, cached `origin/main` and a
read-only remote head query all agreed on
`edd16d6bced7f188430a9803833c730d78e57369`. Starting T0 was
`33301c2415aa806b4ba3d038e15436a0f44c2029`. Main was merged **into T0** with
merge commit `daf3dafbb678c0875c9860816a8ab9be7df642c6`; there were no conflicts
and neither history was rewritten. No final merge into main or push occurred.

The complete diff against current main remains limited to the twelve T0
tooling/documentation files. All Go files (including the corrected
`TestValidatedRepositoryConfigLoads`), module files, firmware and `config.yaml`
are identical to main. Thus the Discord mapping and five-channel/slider-follow
settings are preserved in the isolated branch. That configuration test was
reviewed but **not executed** during this preparation. AGENTS changes remain
confined to its verification entry point; other instructions are preserved.

Preparation checks passed:

- PowerShell parser checks for both tooling scripts, whitespace checks and local
  link checks in the T0 guides/report.
- Tooling self-tests again passed under PowerShell 7 and Windows PowerShell 5.1
  at the synchronized merge commit, using fake Go/GCC only. Captures:
  `validation-results/prep-20261106-182359-pwsh.txt` and
  `validation-results/prep-20261106-182359-powershell51.txt`.
- Separate `-File` invocations with simulated Go failures returned nonzero
  process exit codes under both hosts. Captures:
  `validation-results/prep-child-exit-pwsh.exe.txt` and
  `validation-results/prep-child-exit-powershell.exe.txt`.
- No executable or runtime log is tracked. No directly related tooling defect
  requiring an implementation change was found.

The user reports MSYS2 UCRT64 GCC is now installed; its expected executable
`C:\msys64\ucrt64\bin\gcc.exe` exists. Compiler compatibility execution and
session PATH setup remain deferred, along with all actual quick/full/race
checks. No real compiler was invoked during preparation.

**Stop point:** an extended physical soak is still running. Wait for the user's
explicit confirmation that it has ended before real Go validation. Then run
quick, full (retain artifacts for embedded-identity review), and amd64 race
validation from T0; preserve the reports and stop on any failure or race finding.
Recheck latest main/remote state before integration. Only after all required
checks pass may final integration proceed, followed by the requested merged-main
tests/tooling checks and push verification. No final merge or push is authorized
by the passage of time alone. The running D0 bundle, configuration, diagnostic
launcher, COM ports and original logs remain outside this preparation's writes.
