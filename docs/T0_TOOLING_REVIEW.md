# Track T0 tooling review

Date: 2026-11-06 (America/Phoenix). Base: verified main `627feec`.
Branch: `tooling/validation-workflow`, isolated `.worktrees/validation-workflow`.
The D0 milestone tag and historical qualification reports were not changed.

## Final qualification — 2026-11-06, America/Phoenix

This section supersedes the preparation stop point below: the user's closeout
request reports the prior soak completed and explicitly authorizes actual
validation followed by gated merges/pushes. Earlier sections remain historical
evidence, including the original failed race investigation.

WinCOM was qualified first using the documented live read-only Core Audio
methods on both architectures, the complete ordinary/build matrix, full amd64
race suite and focused explicit checkptr regressions. The qualified integration
merge is `4baef5dda8c03f1a9f2b7ecc9ec619a07e1f3f30`; documentation commit
`d24bcc2d75c2f0143e8b7d9d730070fa56d94b6f` became main. Both architecture
suites and amd64 race testing passed again on actual main before push. A remote
query confirmed main/origin/main at `d24bcc2` after push. See
[WinCOM qualification](WINCOM_POINTER_LIFETIME.md) for results and native limits.

T0 began at `62a5d7bba51eadbc0fefef3f87e8e0ffcc41cb06`, clean. Merging the
updated main into T0 with `--no-ff` produced
`28799e8deb5274a1c2041a029f9f1cba1879742f`, without conflicts or history rewrite.
The full twelve-file T0 diff was reviewed against the updated main. Production
Go and application tests, firmware, dependencies, config and protocol match
main exactly, including both WinCOM corrections and the D0 config-test fix.
The batch addition only redirects output when `MIXR_BUILD_OUTPUT_DIR` is set.

Actual execution on Windows 25H2 build 26200.9711, Go 1.27.3, PowerShell 7.6.5:

| Gate | Result and retained run under `validation-results/` |
| --- | --- |
| `validate.ps1 -Mode quick` | PASS, `20261106-193238-853-458a1685-quick` |
| `validate.ps1 -Mode full -KeepArtifacts` | PASS, `20261106-193252-166-ec36b552-full` |
| `validate.ps1 -Mode race` | PASS, `20261106-193320-446-5f701f4f-race` |
| Tooling self-tests, Windows PowerShell 5.1 | PASS, `tooling-test-0eaa56577dbb41e3a356a3413fcb7b1f` |
| Tooling self-tests, PowerShell 7.6.5 | PASS, `tooling-test-1db6c236dd974fd58053c0a1c0d056c9` |

The full run executed both 386/amd64 suites and retained all ten real Windows
binaries in its `artifacts/386` and `artifacts/amd64` directories. `go version
-m` confirmed every architecture and all four applications' embedded commit
`28799e8`, version `milestone/five-channel-d0-validated-10-g28799e8`, and dev/release
type. Metadata captures and SHA256/size manifest remain in ignored
`.validation/closeout/`. The race compiler was GCC 16.2.0, target
`x86_64-w64-mingw32`, with a resolved `libsynchronization.a`. CGO and compiler/PATH
settings were process-local. No data-race warning or checkptr fatal appeared.
Known module stat-cache access-denied messages were non-fatal build diagnostics.

Self-tests verified plans, unique output directories, cleanup after success and
failure, preservation of retained artifacts, native/batch/race failure exits,
compiler prerequisite failures, successful stderr capture, and environment/cwd
restoration. Their deliberately failed stub runs are orchestration evidence,
not product failures or real race reports. The initial PowerShell 5.1 invocation
was blocked by script execution policy before tests started; rerunning both
hosts with a process-only `-ExecutionPolicy Bypass` passed. No permanent policy
or environment change was made. All captures are retained locally.

No application/controller was launched by T0. Existing COM5 runtime settings,
logs, crash captures, release bundles, branches/worktrees and milestone tags
were preserved. The repository COM4 example and five-channel D0 settings remain
unchanged. Prior 91m22s soak evidence belongs to release `627feec`, not this
safety correction. Dynamic foreground hardware qualification, endpoint/USB
recovery on the new executable, and a new soak remain unverified; foreground
unavailable warnings, microphone deadband and log-retention follow-ups remain
deferred. No encoder/routing feature work is included.

The qualified branch is ready for the requested `--no-ff` main merge. Repeat
Go/tooling checks on actual merged main, retain its final-identity builds, then
push and verify remote synchronization. Git history and the final closeout
manifest identify that resulting commit; no milestone tag is created.

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
