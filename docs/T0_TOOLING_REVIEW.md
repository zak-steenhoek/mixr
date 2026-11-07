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
