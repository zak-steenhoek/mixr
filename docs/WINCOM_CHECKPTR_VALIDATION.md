# WinCOM callback fixture checkptr investigation

## Scope and status — 2026-11-06

Isolated branch `fix/wincom-checkptr-test` starts at main
`edd16d6bced7f188430a9803833c730d78e57369`. Local main, origin/main, and
`git ls-remote origin refs/heads/main` agreed on that commit. T0 remains at
`62a5d7bba51eadbc0fefef3f87e8e0ffcc41cb06` in its original, clean worktree.
No production source, dependencies, firmware, released binaries, or runtime
logs were changed. No merge, tag, or push was performed.

**The original activation checkptr failure is corrected. The full race suite
still fails: a separate stack-address lifetime problem was exposed by the
Go-backed native fixtures. Investigation stopped before broadening into
production changes, as requested. This is not a full race qualification pass.**

Related context: [Windows audio hardening](WINDOWS_AUDIO_HARDENING.md) and
[D0 callback regression guard](D0_CALLBACK_REGRESSION_GUARD.md).

## Original cause and correction

The original focused race command reproduced the fatal checkptr error at
`fakeActivate`'s `*(*uintptr)(unsafe.Pointer(out))` assignment. The callback
received the address of Go output storage as `uintptr`, then reconstructed a
pointer with no pointer operand establishing its provenance. Checkptr rejected
that conversion before the synthetic activation result was written. The
fabricated result `0x1234` was therefore not the cause of this particular
fatal error, but was also inappropriate as a Go pointer value.

The fixture now receives `out *unsafe.Pointer`, writes through that pointer,
and returns a real allocated `IAudioSessionManager2` object address. The other
fake-vtable output callbacks use typed pointer arguments for the same reason:
native-width endpoint address storage, `*int32` count/BOOL, and `*float32`
volume. Native `CoTaskMemAlloc` storage for the endpoint ID is unchanged.

Go's installed `syscall.NewCallback` documentation allows arguments no larger
than a uintptr; its runtime callback ABI translation supports these pointer
arguments. Pointer sizes and argument positions are unchanged on 386/amd64.
This is neither a callback calling-convention mismatch nor evidence of an
incorrect production activation signature. No checkptr directive, skip,
disabled instrumentation, or relaxed assertion was introduced.

The activation test retains context-by-value, activation-parameter identity,
and output-pointer assertions. It additionally checks device and IID identity.
The returned object remains rooted by a Go pointer instead of an integer.
Tests remain sequential, as required by their shared fixture state.

## Validation environment and results

Native Windows amd64 host: display version 25H2, build 26200.9711;
Go `go1.27.3 windows/amd64`; MSYS2 UCRT64 GCC 16.2.0 at
`C:\msys64\ucrt64\bin\gcc.exe`. The 386 suite executed on the Windows host.
Race commands used these process-local settings:

```powershell
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '1'
$env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
go test -race -count=1 -buildvcs=false ./internal/wincom -run '^TestActivateUsesNativeSignature$' -v
go test -race -count=1 -buildvcs=false ./...
```

| Check | Result |
| --- | --- |
| Original focused amd64 race test | Fatal checkptr error reproduced |
| Revised focused amd64 race test | Pass |
| Revised complete amd64 race suite | Fail: DeviceID and mute fixture assertions, below |
| Complete ordinary amd64 suite | Pass |
| Complete ordinary 386 suite | Pass |
| Negative control: wrong activation context | Expected assertion failure: `0x18`, want `0x17` |
| Negative control: write to a different output slot | Expected assertion failure: nil result, want object address |

Ordinary suites used `CGO_ENABLED=0`, each `GOARCH`, and
`go test -count=1 -buildvcs=false ./...`. Negative controls used temporary Go
`-overlay` replacements of the production source, with race/checkptr enabled;
no tracked production file was edited. One changed `uintptr(ctx)` to
`uintptr(ctx)+1`; the other replaced `outValue.Pointer()` with a separate
allocated pointer slot. Both failed at the intended assertions rather than
crashing. The overlays were removed afterward.

No `WARNING: DATA RACE` was emitted. The full race invocation ran the remaining
packages, but the failing WinCOM tests return early, so this does not establish
that all paths are race-free. Physical Windows/Nano regression and live COM
reentrancy were not tested; production behavior was not changed or certified.

## Additional blocker: output address survives a moving Go stack

After the checkptr correction, the DeviceID test reports a nil result and the
mute test reports false instead of true. A second WinCOM-only race run
reproduced both failures. A temporary diagnostic overlay printing the caller's
mute output address before/after the syscall and the callback's write produced:

```text
caller before 0xc0000a5cb8
callback before 0xc0000a5cb8
callback after 0xc0000a5cb8 -1
caller after 0xc0000ffcb8 0
```

This directly demonstrates stale stack output storage in the mute case: the
native callback receives the old address while the resumed caller uses a
relocated stack. The DeviceID symptom is consistent with the same problem,
but was not independently address-traced. The installed runtime's
`cgocallbackg` comments explicitly allow the Go stack to move during a callback.
Disassembly also confirms that `SimpleAudioVolumeMute` keeps its output on the
Go stack and reloads that local after `syscall.Syscall` returns.

The observed failure occurs with reentrant Go test callbacks. It exposes a
pointer-lifetime hazard in exercising the existing production wrappers this
way; it does not prove that a real Windows COM implementation reenters Go on
these calls or that deployed D0 encounters it. Production changes, stack
warming, forced stack sizing, and native shims that bypass instrumented writes
were not used to turn this into a pass. A separate scope decision is needed
before changing the production output-storage lifetime or redesigning this
fixture boundary. Original activation checkptr failure and subsequent stack
lifetime failure must not be conflated.

## Preserved complete race output

The full post-correction race invocation exited 1 with the following complete
Go output (also retained locally in `.validation/full-race.log`):

```text
?   	github.com/zak-steenhoek/mixr/cmd/windows-audio-control	[no test files]
?   	github.com/zak-steenhoek/mixr/cmd/windows-microphone-control	[no test files]
ok  	github.com/zak-steenhoek/mixr/cmd/windows-routing-spike	1.344s
--- FAIL: TestDeviceIDUsesNativePointerWidth (0.00s)
    wincom_windows_test.go:83: IMMDevice::GetId: success returned a nil endpoint ID
--- FAIL: TestMuteOperationsUseWin32BOOL (0.00s)
    wincom_windows_test.go:166: simple mute = false, want true
FAIL
FAIL	github.com/zak-steenhoek/mixr/internal/wincom	0.331s
ok  	github.com/zak-steenhoek/mixr/pkg/mixr	5.447s
?   	github.com/zak-steenhoek/mixr/pkg/mixr/cmd	[no test files]
?   	github.com/zak-steenhoek/mixr/pkg/mixr/icon	[no test files]
ok  	github.com/zak-steenhoek/mixr/pkg/mixr/util	1.692s
ok  	github.com/zak-steenhoek/mixr/tools/icon-generator	2.643s
FAIL
```

Local ignored `.validation/*.log` captures also retain the original complete
fatal stack dump, focused pass, ordinary suites, both negative controls,
WinCOM-only follow-up, disassembly, and diagnostic address trace. Temporary
overlay source/JSON files were removed; those diagnostics are not a permanent
test or production change.
