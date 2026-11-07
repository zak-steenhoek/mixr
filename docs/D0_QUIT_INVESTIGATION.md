# D0 quit investigation — 2026-11-06

## Finding and confidence

A foreground-window lookup leak independently survives bf04c6e. Each uncached
lookup creates a capturing closure and passes it to syscall.NewCallback. Go
retains these callbacks forever; the installed Go 1.27.3 runtime has 2,000 slots
and calls runtime.throw("too many callback functions") when exhausted. This is
an unrecoverable process termination, not a blocked COM request. The release
GUI build has no console to show the fatal stderr; mixr's recover handler
cannot catch runtime.throw.

A direct accelerated test of the original bf04c6e foreground function against
the live Windows desktop failed after 36.025 seconds with that fatal error and
a stack through util_windows.go:91. Evidence is retained with the deliverables.
A separate initial run under restricted process visibility also reproduced a
nil-process dereference at util_windows.go:88; process disappearance can produce
the same nil return under normal use. Neither path was fixed by D0 serialization.

This proves the defect and its termination mechanism. It does not prove the
exact cause of every historical exit: neither supplied failed run has a runtime
stderr capture or dump. Callback exhaustion is the strongest explanation of the
repeated lifetimes. Do not present the final unavailable-target warning as a
causal stack trace.

## Logs and timings (local America/Phoenix)

| Run start | Last application message | Observed log span | Discoveries | Redundant COM warnings |
| --- | --- | --- | --- | --- |
| Nov 4 20:06:45.419 | 20:22:27.000 | 15m 41.581s | 42 | 13 |
| Nov 4 20:31:00.058 | 20:46:31.161 | 15m 31.103s | 35 | 6 |
| Nov 5 20:36:32.525 | 20:52:29.649 | 15m 57.124s | 183 | 98 |
| Nov 5 22:37:03.020 | 22:52:47.149 | 15m 44.129s | 50 | 20 |
| Nov 6 13:19:09.448 | 13:35:05.038 | 15m 55.590s | 360 | 209 |
| Nov 6 14:22:36.259 | 14:38:10.867 | 15m 34.608s | 24 | 0 |
| Nov 6 14:39:41.588 | 14:55:38.199 | 15m 56.611s | 22 | 0 |

Archive filenames mark the next startup/rotation time, not the archived run's
start or exact termination time. These spans end at the last log entry, so they
are lower bounds on process lifetimes. At one lookup per 500 ms, 2,000 callback
slots imply about 1,000 seconds (16m 40s); tray/COM callbacks occupy some slots,
button/slider activity can add lookups, and the last log precedes termination.
The earlier builds' metadata says 2554efc, rather than their current worktree
HEAD e53da9c; their exact source/config provenance is not recoverable from this
metadata alone. The foreground leak is present at bf04c6e and earlier source.

The new runs show no COM timeout, fatal logger message, orderly shutdown, or
serial disconnect. The newest run still accepts audio commands at 14:55:21.320.
Discord sessions expire at 14:52:13.693, discovery succeeds 59 ms later, and many
commands succeed afterward. Foreground unavailable/recovered transitions recur
throughout successful operation; the final warning at 14:55:38.199 is incidental
to the evidence, not proof of a fault. D0 materially reduces discoveries and
removes redundant COM warnings without changing the roughly 16-minute pattern.

A read-only Application log query for events 1000/1001/1002 between 14:15 and
15:05 returned no matches; no mixr-release/mixr process was present at inspection.
A historical hang followed by manual termination cannot be ruled out. A runtime
fatal exit need not produce Windows Error Reporting records.

## Version and configuration

fix/d0-runtime-stability remains at bf04c6eaa496284f5b194533c912a45d85b0804e.
Its config.yaml has a pre-existing local modification and was left untouched.
The tested executable is releases/d0-runtime-stability-amd64-bf04c6e/mixr-release.exe
in that worktree. SHA256:
DB7968A16A3CA33EB3BDB0765244935D99402AB85562364068689D3D3F5A5847.
Both startup logs and go version -m agree on bf04c6e, amd64, release, Go 1.27.3.
VCS metadata marks the build dirty, so metadata alone does not certify a pristine
source tree; the known config modification explains a possible source of that flag.

The absolute loaded config path is inside that release directory. COM5, 9600
baud, four legacy analog channels plus A4 microphone, five momentary mute
controls and 500 ms polling are confirmed. Four analog mappings is correct for
this firmware: A4 is handled separately. A3 is discord.exe in the actual runtime
configuration; the fifth mute target remains deej.current. This was preserved.

## Scoped change

One process-wide non-capturing EnumChildWindows callback replaces the per-poll
closure. HWND and LPARAM use native-width integers. One mutex protects the
synchronous enumeration context and foreground cache; returned slices are copies.
Vanished parent/child processes are handled without a nil dereference, and a
vanished parent clears stale cached targets. The existing D0 COM owner, refresh
policy, audio controls, firmware and source configuration are unchanged.

Regression tests exercise 10,000 deterministic lookups through the actual native
callback ABI, container children, vanished child and parent processes, cache
isolation and concurrent consumers. An opt-in live foreground test exercises
2,500 actual desktop lookups beyond the former failure threshold.

The release bundle includes a diagnostic launcher that redirects fatal stderr,
records PID, executable hash, start/exit time and exit code, and samples thread,
handle and private-memory counts every 15 seconds. These process logs are not
part of run-log pruning. An alive sample proves process existence, not audio
responsiveness; a hung process requires a dump while it remains alive. No registry
or WER configuration was changed.

## Validation and remaining work

Validation results and final binary identity are recorded with the delivered
release. Required physical validation remains: 60–120-minute five-channel Nano
COM5 soak; every button and analog control; focus/app recreation; playback and
microphone endpoint changes; USB disconnect/reconnect; LED and audible isolation;
resource trend review; and 386 physical parity. Accelerated read-only foreground
lookups are not a substitute for that soak. No hardware-affecting application
was launched by this investigation. No merge, push, tag or original-log removal.

Go callback lifetime contract: https://go.dev/src/syscall/syscall_windows.go
Installed runtime limit/throw: GOROOT/src/runtime/zcallback_windows.go and
GOROOT/src/runtime/syscall_windows.go. Other issues observed during inspection,
including COM timeout/lifetime edge cases, were not broadened into this fix.

Automated results on 2026-11-06: full go test -count=1 -buildvcs=false ./...
passed on 386 and amd64. Both architectures built with the established dev and
release scripts and all three diagnostic CLIs. The known module stat-cache
access warning was non-fatal. Vet on both architectures reported only the
pre-existing util.go:43 unbuffered signal.Notify channel. Race instrumentation
was unavailable (CGO disabled). The fixed live amd64 test completed 2,500 actual
foreground lookups in 44.29 seconds; the deterministic 10,000-call and other
foreground tests pass on both architectures. The opt-in live test is skipped
by default; the full suite ran before adding that opt-in test, and the updated
util suite passed again on both architectures afterward.

The diagnostic launcher was verified with a separate immediately exiting GUI
probe: stderr was retained and exit code 2 was correctly recorded and propagated.
The actual application was not launched. This probe exposed and eliminated a
Start-Process fast-exit-code race by owning the native Process handle at creation.
