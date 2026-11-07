# WinCOM pointer lifetime follow-up

## Scope and conclusion — 2026-11-06

Branch `fix/wincom-pointer-lifetime`, isolated worktree
`.worktrees/wincom-pointer-lifetime`, starts at `97a9f25`. Main remains
`edd16d6`; the original fix branch remains `97a9f25`; T0 remains `62a5d7b`.
No merge, push, tag, dependency, firmware, configuration, protocol, release
artifact, or existing runtime-log change is part of this work.

The activation checkptr fatal was test-only pointer provenance misuse, corrected
by the parent commit. The later DeviceID/mute failures are **not additional
checkptr fatals or data-race reports**: they expose a production output-address
lifetime hazard under synchronous reentry into Go. This follow-up corrects that
hazard at the WinCOM syscall boundary. It does not establish that deployed Core
Audio has triggered it, nor certify all native calls in the pinned dependencies.

Prior evidence is preserved in [the initial report](WINCOM_CHECKPTR_VALIDATION.md).

## Failure mechanisms

1. Original activation fixture: receiving Go output storage as `uintptr` and
   reconstructing a pointer lost its Go allocation provenance. Checkptr rejected
   the conversion. The fabricated `0x1234` result was also unsuitable as a Go
   pointer but was not the location of that fatal. The parent fix uses typed
   pointer arguments and a real, rooted object; those changes remain intact.
2. DeviceID and mute: the wrappers' output locals could remain on a Go stack.
   Pointer-to-uintptr conversion directly inside `syscall.Syscall` preserves
   liveness, but its `uintptrkeepalive` contract does not force heap allocation.
   Windows runtime callback dispatch can grow the goroutine stack before native
   arguments become typed Go pointers. Go updates its stack references, but the
   native argument copy still contains the old address. The callback can write
   the old storage, and the wrapper resumes reading its relocated, zero storage:
   nil ID or false mute. `runtime.KeepAlive` does not fix relocation.
3. Count and scalar volume have the same output-storage pattern, even where
   ordinary tests happened to pass. The new address-stability negative control
   demonstrates it. Inspection also found this exact pattern in the pinned
   `ISimpleAudioVolume.GetMasterVolume`, used by `wcaSession.CurrentVolume`.
   Routing that getter through WinCOM is necessary for the same guarantee.

The earlier mute address trace independently demonstrated the stale native
address. This follow-up reproduces DeviceID/mute failures from the untouched
starting revision and demonstrates both stack movement and output-address
movement in the targeted negative control. No asynchronous retention or
concurrent writer is needed to cause these failures.

## Correction and guarantees

`nativeCall` and `nativeCall6` are small forwarding wrappers over the existing
Windows syscalls, marked with the compiler's documented `//go:uintptrescapes`.
Every direct pointer conversion remains in their argument lists. The compiler
therefore places the referenced Go storage on the heap and keeps it alive
through the call. This covers input objects, GUIDs, and output slots without
stack warming, forced allocation tricks, runtime internals, or disabled checks.
The existing explicit KeepAlive calls are retained. Escape-analysis output
confirms heap placement of ID, count, both mute outputs, and both scalar outputs.

Activation's existing reflected output uses `reflect.Value.Pointer`, whose
implementation forces the referenced storage to escape; it is passed directly
and `out` remains alive through the call. Its real-object output test and
address-stability test pass. No intermediate saved uintptr is introduced.

`SimpleAudioVolumeLevel` mirrors the existing endpoint scalar getter using the
same protected boundary. `wcaSession.CurrentVolume` now calls it. DeviceID's
fixture now uses `**uint16` for LPWSTR output instead of `*uintptr`.

Native ABI remains unchanged: native-width interface/GUID/output pointers,
context and BOOL values by value, signed 32-bit count/BOOL storage, FLOAT scalar
storage/bits, identical argument counts and vtable slots, and signed HRESULT
failure detection. Setters and unregister use the same forwarding boundary;
no new native calls or audio mutations are added.

These are synchronous caller-owned argument buffers. The compiler contract
guarantees their heap lifetime for the invocation, not permission for C to
retain them afterward. Real COM interface pointers and CoTaskMem strings refer
to native allocations, not Go allocations; native output pointers do not create
Go-pointer-to-Go-pointer retention. GetId's native string is copied then freed
with CoTaskMemFree, including the existing error cleanup. Returned interfaces
retain their existing caller release responsibility. No ownership or apartment
transition changes: production interfaces remain on the existing locked STA
executor, and notification objects remain owned by the finder until unregister
and release. Notification callbacks' IID/output addresses originate in native
COM calls, unlike the original synthetic fixture's Go output address.

The synthetic callbacks consume output pointers synchronously; they do not
retain them beyond the invocation. Typed arguments preserve ABI widths and Go
provenance. Their Go-allocated fake interfaces and returned activation object
are fixtures, not a model of actual COM allocation/refcount implementation.
This is a Windows syscall boundary, not a generated cgo function: enabling CGO
for race testing does not independently validate every cgo pointer rule or
foreign ownership obligation.

References: [compiler escape directive](https://pkg.go.dev/cmd/compile#hdr-Compiler_Directives),
[unsafe pointer rules](https://pkg.go.dev/unsafe#Pointer),
[Windows syscall runtime](https://go.dev/src/runtime/syscall_windows.go), and
[cgo pointer rules](https://pkg.go.dev/cmd/cgo#hdr-Passing_pointers).
The installed Go sources were also inspected, including dll_windows.go,
cgocall.go, callbackWrap and compiler escape analysis.

## Regression and validation

`TestNativeOutputsSurviveCallbackStackGrowth` runs each ABI fixture from a fresh
goroutine. A non-inlined recursive callback helper forces stack growth and
checks that a stack marker really moved. It snapshots the output address as an
integer, then compares that snapshot with the typed pointer after growth. The
integer is never converted back or dereferenced. This distinction matters:
typed Go pointers can follow stack relocation and still return correct values,
masking a stale address retained by native code. Existing value/identity/ABI
assertions remain. Shared fixtures remain sequential and channel completion
synchronizes the worker and test goroutines.

Environment: native Windows 25H2, build 26200.9711; Go 1.27.3 windows/amd64;
MSYS2 UCRT64 GCC 16.2.0 at `C:\msys64\ucrt64\bin\gcc.exe`. Both architectures'
ordinary tests execute on this Windows host.

| Check | Result |
| --- | --- |
| Starting revision DeviceID/mute, amd64 race | Reproduced nil ID and false mute failures |
| Final focused WinCOM, amd64 race | Pass, including forced-growth regression |
| Focused DeviceID/mute/growth, explicit checkptr=1 plus race | Pass |
| Final full Windows amd64 race suite | Pass; no data-race warnings |
| Full ordinary Windows 386 and amd64 suites | Both pass |
| D0 registration, transition reuse, 10,000-lookup guards | Pass on both architectures and amd64 race |
| Dev/release scripts, routing/audio/microphone CLI builds | All pass on both architectures |
| No-escape directive negative control | Expected address-stability failures for ID, count, volume and mute |
| Wrong activation context negative control | Expected 0x18 versus 0x17 assertion failure |
| Wrong activation output slot negative control | Expected nil versus real object assertion failure |

Race settings are process-local: `GOARCH=amd64`, `CGO_ENABLED=1`,
`CC=C:\msys64\ucrt64\bin\gcc.exe`, with its directory prepended to PATH.
Commands:

```powershell
go test -race -count=1 -buildvcs=false ./internal/wincom -v
go test -race -count=1 -buildvcs=false '-gcflags=all=-d=checkptr=1' ./internal/wincom -run 'Test(DeviceID|Mute|NativeOutputs)' -v
go test -race -count=1 -buildvcs=false ./...
```

The ordinary/build matrix follows AGENTS.md with `CGO_ENABLED=0`. Build scripts
retain the known non-fatal module-cache stat permission messages. Full local
captures, including failures and negative controls, remain in ignored
`.validation/*.log`. Negative controls use temporary absolute-path Go overlays,
removed afterward; no tracked source was left mutated. An initial relative
overlay argument was parsed incorrectly by PowerShell and is not negative-control
evidence; corrected absolute-path runs supply the results above. A first growth
test checked only values and passed old code; the final address assertion
specifically closes that gap.

## Limits and recommended read-only qualification

No live COM, USB/Nano, endpoint switching, microphone hardware or audio soak test
was performed here. The synthetic native boundary proves this lifetime
regression and ABI assertions, not actual Core Audio reentry behavior, arbitrary
third-party COM implementations, or memory safety of every pinned wrapper.
No physical regression or integration qualification is claimed. No substantial
COM redesign was needed for the demonstrated defect.

Before integration, run a focused read-only test on the actual Windows audio
setup with both architectures. Use the newly built audio-control CLI's
`current-volume --target master`, `current-mute --target master`, and the same
operations for a running application's exact session key; use microphone-control
`current` and routing-spike `list`/`sessions` to exercise discovery/IDs. Repeat
reads while observing endpoint/session changes made by the user. Compare the
read values with Windows, record Windows/build, architecture, devices, active
applications, procedures/results and untested cases, and verify clean shutdown.
Do not use the diagnostics' mutation commands for this read-only check. Follow
the existing physical regression requirements before merge/tag qualification.
