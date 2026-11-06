# Track D0 runtime stability fix

## Status and scope

This isolated fix branch starts at Track D0 commit
`e53da9ce2a22eeeb2f75989509639dbae1e7f3b5`. It contains the refresh
containment and Windows COM ownership correction for the repeatable
five-channel release-build termination. It does not change firmware, pins,
serial framing, mappings, routing, log retention, or configuration.

Automated checks are software evidence only. The 60-minute Windows/Nano soak,
physical endpoint changes, USB reconnect, resource observation, and crash-dump
exercise below remain pending user validation. This branch is isolated,
unmerged, untagged, and unpushed.

## Preserved failure evidence and confidence

Five captured runs stopped abruptly 15:31-15:57 after startup. Every log ended
after successful audio-session discovery and contained no graceful shutdown,
Go panic, serial-disconnect boundary, or recorded Windows Application Error.
The evidence bundle and its SHA-256 manifest are external, read-only inputs; no
evidence file was changed by this branch.

The evidence establishes with **high confidence** that normal unavailable
five-channel targets, particularly `deej.current`, triggered a full-discovery
storm. Each control previously had its own five-second recovery budget, and no
coordinator covered the complete clear/discover/add transaction. The logs show
completed discoveries only 14-32 ms apart.

The immediate native failure mechanism remains **moderate-to-high confidence**:
retained Core Audio interfaces could be used and released concurrently and on
different OS threads from the apartment that acquired them. No crash dump
exists, so this report does not claim a particular faulting instruction.
`CoInitializeEx` returning `E_FALSE` was a marker of repeated initialization,
not itself a fatal error.

## Refresh coordination and recovery behavior

Before this fix, indicator polling retried an unavailable target, then could
force full discovery independently for each control every five seconds.
Refresh callers could overlap while sessions were cleared, released,
rediscovered, and republished. A caller could retain a session slice after the
map lock was released and use it while another goroutine released the same COM
interfaces.

After this fix:

- one coordinator elects a single refresh transaction and makes overlapping
  callers wait for and share its result;
- one global five-second cooldown covers automated refresh reasons;
- the 45-second ordinary freshness check remains the automatic application and
  foreground-session recovery path;
- routine indicator polling never requests discovery solely because Chrome,
  Spotify, `deej.current`, master, or microphone is unavailable, and the
  corresponding LED remains off;
- one explicit button press may request one cooldown-bounded recovery refresh
  and retry the command once;
- stale endpoint/session errors request the same globally bounded recovery;
- manual tray refresh remains an explicit cooldown bypass;
- configuration reload and slider recovery use the coordinator; and
- the session-map lock remains held across command/session use, while snapshot
  replacement releases old sessions only after all users have left the map.

The refresh keeps the previous usable snapshot if discovery fails. Snapshot
discovery, publication, and old-session release are one serialized operation;
serial writes and indicator callbacks occur outside the session-map lock.

## Windows COM ownership

Refresh synchronization alone would not make retained COM interfaces
thread-correct. The Windows finder therefore runs behind one dedicated
goroutine locked to one OS thread. That owner:

1. initializes one STA apartment once;
2. creates and retains the device enumerator there;
3. performs discovery and every retained session get/set operation there;
4. releases sessions and unregisters/releases the finder there; and
5. calls `CoUninitialize` once during orderly shutdown.

Requests use a bounded buffered queue and a 30-second request/response timeout.
Session proxies cache only their non-COM key and forward all interface work to
the owner. The default-device callback uses synchronized finder pointers and
an atomic stale flag; it does not invoke retained Core Audio interfaces.

## Diagnostics

Debug diagnostics record refresh request ID, reason, forced/manual status,
coalescing, start, completion, time since the previous completion, failures,
and cooldown suppression. Routine cooldown suppression is globally limited to
one debug record per 30 seconds. Valid ADC frames and ordinary successful
indicator polls remain quiet. Unavailable-target warnings retain the existing
30-second limiter and recovery transition.

## Automated verification

On 2026-11-06, the implementation and focused regressions passed the following
for both `GOARCH=386` and `GOARCH=amd64`:

- `go test -count=1 -buildvcs=false ./...`;
- `pkg/mixr/scripts/windows/build-dev.bat`;
- `pkg/mixr/scripts/windows/build-release.bat`;
- Windows routing-spike build;
- Windows audio-control build; and
- Windows microphone-control build.

Both build scripts emitted the previously documented non-fatal denial while Go
attempted to update its module stat cache, then returned success and produced
the requested executables. `go vet -unsafeptr=false ./...` on both
architectures reported only the documented pre-existing unbuffered
`signal.Notify` diagnostic in `pkg/mixr/util/util.go`.

Focused deterministic tests cover sustained missing foreground/Chrome/Spotify
polling, one global refresh budget, coalesced simultaneous refreshes, command
versus release exclusion, one button recovery attempt, application recovery,
foreground indication, default playback and recording recovery, manual
refresh, existing momentary/latching deduplication, legacy A0-A4 behavior, and
serial reconnect/indicator synchronization. No test uses a real-time
five-second cooldown wait.

The race detector could not run in this Windows toolchain:

```text
go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
```

## Manual 60-minute validation checklist

Record date/time, final commit, executable architecture, Windows edition/build,
Go version, Nano R4 and Arduino core, COM port, firmware hash, default playback
and recording endpoints, starting/ending process thread count, handle count,
private memory, discovery count, every observation, and every untested case.

1. Run the amd64 release build for at least 60 minutes under ordinary
   five-channel use. Operate every mute button and analog control occasionally.
2. Keep Spotify and Chrome closed long enough to span several ordinary
   freshness intervals, then open, play audio, restart, and play again. Confirm
   the controls and LEDs recover without a discovery loop.
3. Repeatedly switch foreground focus among two active audio applications and
   applications without audio sessions. Confirm unavailable focus leaves the
   foreground LED off and a newly appearing session recovers.
4. Press every mute button while its target is both available and unavailable.
   Confirm one action per distinct press, no release/hold action, and at most
   one bounded recovery discovery.
5. Change the default playback endpoint and confirm master volume/mute and its
   LED retarget without affecting application isolation.
6. Disable/re-enable the default microphone, and change it if a second capture
   endpoint is available. Confirm A4, microphone mute, and its LED fail safe and
   recover.
7. Disconnect/reconnect USB normally and once while each button is held.
   Confirm handshake baselining, no unintended toggle, analog recovery, and
   five-LED synchronization.
8. Sample process thread count, handle count, and private memory at startup and
   at least every 15 minutes. Record the values and investigate sustained
   monotonic growth.
9. Inspect the run log and count full discoveries. Confirm they remain bounded
   by explicit/manual events and ordinary freshness rather than five-second
   per-control polling.
10. Repeat a shorter parity smoke test with the 386 release build.

If mixr exits unexpectedly, preserve the current and archived run logs before
the next launch. Configure Windows Error Reporting LocalDumps for
`mixr-release.exe` with `DumpType=2` (full dump), an explicit writable dump
folder, and a bounded `DumpCount`, or run Sysinternals ProcDump with its
unhandled-exception capture option. Record the exact tool/configuration and
collect the dump, executable hash, Windows event records, and matching log as
one evidence set. Do not infer a native faulting instruction without the dump.

## Remaining risks and unverified cases

- The five captured exits lack a crash dump, so the historical native fault is
  not proven even though both known trigger and leading lifetime mechanism are
  corrected.
- The 60-minute physical soak, endpoint/USB exercises, resource measurements,
  and 386 physical parity check are pending user confirmation.
- Race instrumentation was unavailable because CGO is disabled.
- Core Audio callbacks are still delivered by Windows-owned threads; callback
  state is synchronized, but extended callback behavior would require a fresh
  ownership audit.
- Polling remains the indicator mechanism, so external-state indication latency
  remains bounded by the configured poll interval.
- Foreground resolution retains its documented process-ownership limitations.
