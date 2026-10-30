# Relative volume and mute software foundation

## Scope and status

Track B1 implements host-side relative-volume commands, mute commands, state
queries, and architecture-safe Windows Core Audio mute bindings. It does not
add encoder/button serial messages, firmware, indicators, routing policy, OLED,
RGB, GPIO, or output-selector behavior. The original potentiometer path and
pipe-delimited serial protocol are unchanged.

This is software-foundation evidence, not physical encoder/button validation.
The final six-channel hardware requirements in `REQUIREMENTS.md` remain
`Planned` until the corresponding hardware and bidirectional protocol exist and
are physically validated.

## Command model and behavior

`pkg/mixr.AudioCommand` is independent of serial transport and represents:

- signed relative-volume movement;
- explicit mute and unmute;
- mute toggle;
- current-volume query; and
- current-mute query.

Targets use the established exact, case-insensitive mapping keys. `master`
selects the existing master session for the Windows default playback endpoint,
`deej.current` resolves the foreground process using the existing transform,
and application names such as `chrome.exe` select every matching Core Audio
session across active render endpoints.

Relative movement is `Steps * Increment`, where `Steps` is signed and the
configurable scalar `Increment` must be greater than zero and at most one. Each
matching session is handled independently: mixr reads that session's actual
current volume immediately before the change, adds the relative movement,
clamps the result to `0.0..1.0`, writes it, and reads it back. It does not derive
a later command from a cached desired value. Matching sessions with different
starting volumes therefore remain different while moving by the same amount,
subject to clamping.

Mute operations also act on every matching session. A query reports each
session plus one deterministic aggregate state: `all-muted`, `all-unmuted`, or
`mixed`. Explicit mute/unmute sets all sessions to the requested state and then
reads them back. Toggle first reads every session without mutating any; if all
are muted it unmutes all, otherwise (including a mixed state) it mutes all.
External Windows changes are observed on the next command/query because mute
and volume state are read from Core Audio rather than a cache. This track does
not add asynchronous session-volume notifications.

## Windows ABI boundary

`internal/wincom` now owns `ISimpleAudioVolume` and `IAudioEndpointVolume`
mute calls. `GetMute` receives into a 32-bit integer matching Windows `BOOL`,
and `SetMute` passes a 32-bit `0` or `1`. Production code does not call the
pinned `go-wca` mute methods. Fake-vtable tests cover both application-session
and endpoint get/set argument shapes on 386 and amd64.

## Software-only Windows exercise

Build the diagnostic from the repository root:

```powershell
go build -o windows-audio-control.exe .\cmd\windows-audio-control
```

Read-only examples:

```powershell
.\windows-audio-control.exe current-volume --target master
.\windows-audio-control.exe current-mute --target spotify.exe
.\windows-audio-control.exe current-mute --target deej.current
```

Mutation examples (use an audible test source and restore its state):

```powershell
.\windows-audio-control.exe relative-volume --target master --steps 1 --increment 0.02
.\windows-audio-control.exe mute --target spotify.exe
.\windows-audio-control.exe unmute --target spotify.exe
.\windows-audio-control.exe toggle-mute --target spotify.exe
```

## Automated and live validation

Validation ran on Windows on 2026-10-29. The required matrix used
`go test -count=1 -buildvcs=false ./...`, the existing development build script,
the routing-spike build, and the new audio-control build.

| Check | 386 | amd64 |
|---|---:|---:|
| All Go tests | Pass | Pass |
| mixr development build | Pass | Pass |
| routing-spike build | Pass | Pass |
| audio-control build | Pass | Pass |
| Relative calculation/clamping/fresh-state unit tests | Pass | Pass |
| Multi-session mute transition/query unit tests | Pass | Pass |
| Fake-vtable 32-bit `BOOL` mute ABI tests | Pass | Pass |
| Live `master` current-volume query | Pass: `0.5000` | Pass: `0.5000` |
| Live two-session `spotify.exe` mute query | Pass: both unmuted | Pass: both unmuted |

The command-model tests and fake COM vtables are simulated: they do not change
Windows audio and do not prove physical behavior. The live checks ran outside
the tool sandbox against the actual Windows audio service and prove read-only
default-endpoint volume, per-session mute readback, multi-session matching, and
both architecture paths. No live volume or mute mutation was performed, so
audible set/toggle behavior remains part of manual validation.

`go vet -unsafeptr=false ./...` reports only the pre-existing unbuffered
`signal.Notify` channel diagnostic on both architectures. Raw COM-boundary
`unsafe.Pointer` diagnostics remain expected as described in
`WINDOWS_AUDIO_HARDENING.md`.

## Manual Windows command validation

Record the Windows build, mixr commit, architecture, default endpoint, target
application/PIDs, starting states, commands, readbacks, audible result, and any
untested case.

- [ ] On amd64, query master volume/mute, make one small positive and negative
  relative move, and confirm Windows reports the clamped/read-back values.
- [ ] Change master volume externally, issue another relative move, and confirm
  it starts from the external value and affects only the current default
  playback endpoint.
- [ ] For an audible application with two matching sessions, give the sessions
  different volumes, move them relatively, and confirm each starts from its own
  value.
- [ ] Exercise explicit mute and unmute for master and an application target;
  confirm the printed readback and audible state agree.
- [ ] Create a mixed application mute state in Windows Volume Mixer; confirm
  query reports `mixed`, the first toggle mutes all, and the second unmutes all.
- [ ] Change mute externally and confirm the next query reports the change.
- [ ] Repeat a read/query and mutation smoke test with the 386 build if x86
  remains supported.
- [ ] Change the default playback endpoint and confirm `master` follows the new
  default after rediscovery, without changing a non-default endpoint.

## Nano R4 four-potentiometer regression plan

Use the established four-pot configuration, Nano R4 on the configured 9600-baud
COM port, and audible sources. Run amd64 first and repeat a 386 smoke test if x86
distribution remains required.

- [ ] Start mixr and confirm the unchanged legacy `0..1023` pipe-delimited
  stream detects all four potentiometers.
- [ ] Move the master pot through low, middle, and high positions; confirm only
  the Windows default playback endpoint follows.
- [ ] Confirm Chrome and Spotify pots independently control every matching
  session for their configured executable.
- [ ] Confirm the fourth `deej.current` pot follows the foreground application.
- [ ] Fully exit and reopen Chrome, Spotify, and the foreground test app; confirm
  sessions are reacquired without restarting mixr.
- [ ] Change the default playback endpoint and confirm the master pot follows
  after refresh.
- [ ] Run all four controls for at least ten minutes and record crashes, serial
  disconnects, stalled callbacks, or unhealthy resource growth.

## Remaining risks

- Live volume and mute mutation, audible behavior, external-change interaction,
  and default-device change behavior still require the manual checks above.
- Nano R4 regression is pending; automated builds do not constitute a physical
  regression pass.
- State is authoritative at command time, but Core Audio provides no atomic
  read-modify-write across an external concurrent change. A change racing in
  the small interval between read and write may be superseded by the command.
- The software-only executor uses an explicit refresh to discover recreated
  sessions; the existing tray session map retains its established refresh
  behavior. Asynchronous session/mute notifications are not added here.
- Mute command support is Windows-only. Linux session types return an explicit
  unsupported error so cross-platform interfaces continue to compile.
- Existing COM apartment/lifetime limitations documented in
  `WINDOWS_AUDIO_HARDENING.md` are not broadened or resolved by this track.
