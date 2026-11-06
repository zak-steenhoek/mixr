# D0 permanent callback regression guard

## Scope and provenance

This test-only follow-up starts at D0 candidate `5b955d1` on
`track-d/channel-config` (base main `94fba24`). Initial checkout was clean on
main; work is isolated on `track-d/callback-regression-guard` in
`.worktrees/callback-regression-guard`. No production code, dependencies,
configuration, firmware, protocol, audio/COM behavior, or tags change.

The user reported that the preceding fix passed soak and hardware testing.
That is user-reported evidence; hardware/build details and individual checklist
results were not supplied with this request. This task does not independently
certify or repeat that physical validation. Earlier investigation evidence is
in [D0 quit investigation](D0_QUIT_INVESTIGATION.md) and
[D0 runtime stability](D0_RUNTIME_STABILITY.md).

## Guarantees and limits

Go's public syscall API documents permanent callback retention but provides no
supported callback-allocation counter. The guard combines two approaches:

- `TestForegroundCallbackRegistrationStructure` parses production Go source in
  the util package, excluding tests. It requires the shared callback's single
  package variable initializer to call `NewCallback(collectChildWindowProcess)`.
  It rejects every additional selector reference named `NewCallback` or
  `NewCallbackCDecl`, including references saved as aliases and allocations in
  local helpers. It deliberately pins this small lifetime invariant; an
  intentional redesign must update the guard after reviewing callback lifetime.
- `TestForegroundCallbackReuseAcrossTransitions` intercepts child enumeration
  and checks that its native callback pointer and the shared variable equal the
  original registered pointer. The callback is actually invoked through the
  existing native ABI test helper on both architectures. It exercises a forced
  cache hit, invalidation, an application change, missing/erroring parents,
  disappearing children, recovery, concurrent invalidating callers, and context
  cleanup. Cache states are set explicitly; no elapsed-time threshold is used.

Restoring per-lookup registration in this package fails the structural guard
immediately, without consuming the runtime's callback budget. Returning a newly
registered closure to enumeration also fails the pointer guard. The existing
10,000-lookup test is unchanged and remains an additional behavioral safeguard.

This is not a process-wide allocation count or a proof about arbitrary future
code: indirect allocators in other packages, dot-imported allocator names, custom
native registration, or allocations on unexercised paths outside the scanned
package are not covered. Pointer identity alone cannot detect discarded
allocations or runtime deduplication of repeated registration of the same Go
function; the source check supplies that protection for direct selector-based
registration in util. Neither test proves all concurrency interleavings,
physical audio behavior, or unlimited runtime stability. No runtime internals,
memory offsets, allocation-counting dependency, or production hook is used.

## Automated validation — 2026-11-06

Environment: native Windows amd64 host, build `26200.9711`, display version
`25H2`, Go `go1.27.3 windows/amd64`. The registry's ProductName reports
`Windows 10 Pro`; build/version are recorded to avoid relying on that label.
386 tests execute through Windows compatibility support, rather than merely
cross-compiling.

| Check | Windows 386 | Windows amd64 |
| --- | --- | --- |
| Full `go test -count=1 -buildvcs=false ./...` | Pass | Pass |
| Established development build script | Pass | Pass |
| Established release build script | Pass | Pass |
| Routing-spike CLI build | Pass | Pass |
| Audio-control CLI build | Pass | Pass |
| Microphone-control CLI build | Pass | Pass |
| `go vet ./...` | Existing diagnostics | Existing diagnostics |
| `go vet -unsafeptr=false ./...` | Existing signal diagnostic | Existing signal diagnostic |
| Opt-in live desktop test, 2,500 foreground lookups | Pass, 53.24 s | Pass, 42.31 s |

The new guards and preserved 10,000-lookup test execute in the full suite on
both architectures. For the live test, set `MIXR_TEST_LIVE_FOREGROUND=1` and run
`go test -count=1 -buildvcs=false ./pkg/mixr/util -run '^TestForegroundLiveCallbackBudget$' -v`
with each `GOARCH`. The first restricted runs returned zero visible foreground
processes and failed their coverage requirement on both targets. Repeating
with normal desktop/process visibility passed with 2,500 actual lookups each;
no coverage threshold was relaxed.

Negative controls on both architectures confirmed that inserting even one
discarded `NewCallback` call in the lookup fails the structural test, and
passing a freshly registered capturing closure to enumeration fails the
pointer test. Each control ran only its targeted test, failed for the expected
assertion, and was restored byte-for-byte afterward. Both full suites passed
again on the restored source. These temporary mutations are not committed.

Vet's existing unsafe-pointer diagnostics are in `internal/wincom` ABI tests,
`cmd/windows-routing-spike/main_windows.go`, and
`pkg/mixr/session_finder_windows.go`; the filtered run leaves only the existing
`pkg/mixr/util/util.go:43` unbuffered `signal.Notify` channel diagnostic. No new
vet diagnostics originate in the guard. Both application scripts emit the
known non-fatal module stat-cache access denial and exit successfully.

Builds and local validation captures are confined to this isolated worktree.
The completed soak's release executable and runtime logs were never launched,
deleted, rotated, or overwritten. No merge, push, or tag is performed.

## Race testing prerequisite and repeatable command

Windows/amd64 is a supported Go race target; Windows/386 is not. The initial
environment has `CGO_ENABLED=0`, `CC=gcc`, and no compiler on PATH. No suitable
compiler was found in the checked MSYS2, MinGW, Scoop/tools, LLVM, or Cygwin
locations. An explicit full-suite attempt with `GOARCH=amd64`,
`CGO_ENABLED=1`, and `-race` stopped at build time:

```text
# runtime/cgo
cgo: C compiler "gcc" not found: exec: "gcc": executable file not found in %PATH%
```

Race testing **could not run**; there is no race-test pass or race report to
interpret. No compiler was installed and no system configuration changed.
The missing prerequisite is a native x86-64 C compiler incorporating MinGW-w64
runtime libraries version 8 or later, including `libsynchronization.a`.
See [Go race detector requirements](https://go.dev/doc/articles/race_detector#requirements).

Minimal proposed setup, requiring user approval before installation: install
[MSYS2](https://www.msys2.org/), then run
`pacman -S mingw-w64-ucrt-x86_64-gcc` in its UCRT64 terminal. Verify that
`gcc --print-file-name libsynchronization.a` returns a full existing library
path. No permanent PATH change is needed. In a fresh PowerShell at the repository
root, use these process-local settings (adjust the installation path):

```powershell
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '1'
$env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
& $env:CC --print-file-name libsynchronization.a
go test -race -count=1 -buildvcs=false ./...
```

Close that shell afterward to discard its settings. This command remains
unverified here until the prerequisite is installed; it is an optional milestone
check and does not replace the normal two-architecture matrix.
