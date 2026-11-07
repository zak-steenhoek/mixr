# Windows amd64 race testing

Run this setup after any active hardware soak, in a separate validation
worktree. Compiler installation is a deliberate manual operation; the
validation script only checks prerequisites and never installs them.

1. Install [MSYS2](https://www.msys2.org/) and follow its update instructions.
2. Open **MSYS2 UCRT64**, then install GCC:

   ```sh
   pacman -S mingw-w64-ucrt-x86_64-gcc
   ```

   Use the UCRT64 compiler, not MSYS `/usr/bin/gcc` or a 32-bit MinGW compiler.
   [MSYS2 environments](https://www.msys2.org/docs/environments/) describes
   the UCRT64 toolchain and runtime.
3. Open a fresh PowerShell in the worktree. Adjust the installation path:

   ```powershell
   $env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
   $env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
   $env:CGO_ENABLED = '1'
   $env:GOARCH = 'amd64'
   & $env:CC --version
   & $env:CC -dumpmachine
   & $env:CC --print-file-name libsynchronization.a
   .\pkg\mixr\scripts\windows\validate.ps1 -Mode race
   ```

   Expect target `x86_64-w64-mingw32` and a full existing path to
   `libsynchronization.a`. Go's Windows race runtime requires MinGW-w64
   runtime libraries v8 or later; the library check is the documented
   compatibility probe. The wrapper checks both, records compiler/version,
   forces `GOOS=windows`, amd64 and CGO=1, and runs:

   ```powershell
   go test -race -count=1 -buildvcs=false ./...
   ```

All `$env:` assignments above affect only that PowerShell process and its
children. Close it afterward to discard them; no permanent PATH edit is needed.
The wrapper restores its own changes on exit. `CC` must name a compiler
executable, optionally an absolute path, without embedded command-line flags.

[Go's race requirements](https://go.dev/doc/articles/race_detector#requirements)
list Windows **amd64**, not Windows 386. The detector has no supported Windows
386 runtime; enabling CGO does not add one. Continue ordinary 386 tests/builds
with `-Mode full`; do not interpret an unsupported race target as a product bug.

`WARNING: DATA RACE` identifies conflicting accesses and stacks showing where
goroutines were created. Retain the command output, commit and reproduction;
inspect synchronization/ownership around both accesses. A race report fails
the run. The wrapper overrides caller `GORACE` settings to keep failure exits
and output visible. Compiler/linker/setup errors are prerequisite failures,
not detected data races. A passing run covers only executed paths and does not
replace physical audio/USB validation or prove the absence of races.

Use race testing for concurrency, callback ownership, refresh/shutdown changes
and milestones. Go documents substantial runtime/memory overhead; ordinary
iterations should use quick tests, then full checks at review/integration.
T0 did not install GCC or execute this suite; previous D0 race deferrals remain
unchanged historical evidence.
