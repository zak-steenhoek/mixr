# Experimental dual hardware mute integration

## Status and scope

This branch extends the physically validated single-Spotify-mute prototype with
a second independent control for the current Windows default playback endpoint.
The dual-control experiment has now passed the user-reported physical validation
recorded below. The earlier Spotify prototype evidence in
[EXPERIMENTAL_HARDWARE_MUTE.md](EXPERIMENTAL_HARDWARE_MUTE.md) remains separate
historical evidence.

The experiment retains four legacy potentiometers, the 9600-baud serial link,
the Track B1 command model, and documented `IAudioEndpointVolume` mute calls
through `internal/wincom`. It does not add microphone control, selectors,
encoders, a final protocol, or production hardware.

## Controls and observed-state rules

| Hardware | Windows target | Indicator rule |
|---|---|---|
| D2 switch / D3 LED | Every matching configured Spotify application session | On only when all observed sessions are muted; off for all-unmuted, mixed, or unavailable |
| D4 switch / D5 LED | The current default playback endpoint (`master`) | On only when that endpoint reports muted; off when unmuted or unavailable |

A stable transition in either direction requests one toggle. Switch position is
not mute state. Each LED is derived from Windows readback after a command and
during bounded polling. The switch histories and indicator caches are
independent.

Default-endpoint mute and application-session mute are intentionally separate.
If headphones are the Windows default while Spotify is routed to speakers, D4
mutes the headphones only. Spotify continues on the speakers unless D2 mutes
its application sessions. Changing the Windows default causes the established
default-device callback and session refresh path to reacquire `master`; D5
then follows the newly selected endpoint.

## Exact experimental serial messages

All messages are newline-delimited ASCII. This additive namespace remains
experimental.

| Direction | Message | Meaning |
|---|---|---|
| Host → Nano | `mixr:v1:hello` | Begin or restart the handshake |
| Nano → host | `mixr:v1:ready` | Both switch baselines were reset; no toggle was emitted |
| Nano → host | `mixr:v1:event:switch:spotify-mute:0` | D2 debounced open transition |
| Nano → host | `mixr:v1:event:switch:spotify-mute:1` | D2 debounced closed transition |
| Nano → host | `mixr:v1:event:switch:master-mute:0` | D4 debounced open transition |
| Nano → host | `mixr:v1:event:switch:master-mute:1` | D4 debounced closed transition |
| Host → Nano | `mixr:v1:state:indicator:spotify-mute:0` | Turn D3 off |
| Host → Nano | `mixr:v1:state:indicator:spotify-mute:1` | Turn D3 on |
| Host → Nano | `mixr:v1:state:indicator:master-mute:0` | Turn D5 off |
| Host → Nano | `mixr:v1:state:indicator:master-mute:1` | Turn D5 on |

Legacy input remains exactly one line of four pipe-delimited `0..1023`
numbers, for example `0|256|512|1023`. A4 is reserved and does not appear in
that frame.

The host retries `hello` until `ready`, ignores pre-ready switch events,
rejects duplicate stable states per control, and clears both indicator caches
on reconnect. Firmware baselines both switches on every `hello`, arms each
only after its own stable debounce interval, and emits no initial state as an
action.

## Firmware build, upload, wiring, and host setup

Firmware details and exact D2–D5 wiring are in
[`arduino/mixr-hardware-mute-prototype/README.md`](../arduino/mixr-hardware-mute-prototype/README.md).
The known-good reference sketch is not modified.

Use the existing COM port, 9600 baud, slider mappings, inversion, and noise
settings. Enable the experiment with:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  poll_interval_ms: 500
```

The target configures only D2/D3. D4/D5 always use the special `master` target.
The polling interval has a 100 ms lower bound. Restart mixr after config changes.

Build and run:

```powershell
$env:GOARCH = 'amd64'
cmd /c "call pkg\deej\scripts\windows\build-dev.bat"
Remove-Item Env:GOARCH
.\mixr-dev.exe --verbose
```

Close Arduino Serial Monitor first because only one process can own the COM
port.

## Manual regression and validation

Record the commit, Windows version/build, architecture, Go version, Nano R4
board/core, Arduino tool version, COM port, default endpoint, Spotify route,
Spotify session/PID count, wiring, procedure, result, and untested cases.

### Startup, debounce, and legacy input

1. Start with both switches stable in arbitrary positions.
2. Confirm one handshake/ready sequence, no startup toggle, and initial D3/D5
   synchronization from Windows.
3. Exercise A0–A3 across their ranges and confirm the existing master, Chrome,
   Spotify, and foreground mappings remain independent.
4. Operate D2 and D4 separately in both directions. Confirm one accepted event
   and one toggle per stable transition, with no repeats while held or bounced.

### Independent state and routing separation

1. With Spotify on the default endpoint, toggle D2 and confirm only Spotify
   sessions and D3 change.
2. Toggle D4 and confirm only the default endpoint and D5 change.
3. Route Spotify to a non-default endpoint (for example speakers while
   headphones remain default).
4. Toggle D4: the default endpoint must mute, D5 must turn on, and Spotify must
   remain audible on its routed endpoint.
5. Toggle D2: Spotify must mute independently and D3 must turn on.
6. Restore all changed mute and routing state.

### Multiple sessions and external changes

1. With multiple Spotify sessions, verify all-muted means D3 on and all-unmuted
   or mixed means D3 off.
2. Create a mixed Spotify state; the next D2 transition must mute all, followed
   by an all-unmute on the next transition.
3. Change Spotify mute externally and confirm D3 updates without a hardware
   event.
4. Change default-endpoint mute externally and confirm D5 updates without a
   hardware event.

### Default-device changes and missing endpoints

1. Note D5 and the mute state of the current default endpoint.
2. Select another Windows default playback endpoint.
3. Allow the device callback/session refresh and polling interval to run.
4. Confirm D4 controls only the new default and D5 reflects its actual state.
5. Temporarily remove/disable all playback endpoints if safe. Confirm D5 turns
   off, D4 causes no crash or unrelated mute, and logs report unavailable state.
6. Restore an endpoint and confirm control/readback recover after rediscovery.

### USB reconnect and application recreation

1. Disconnect USB with both switches stable. Confirm neither Windows mute state
   changes.
2. Reconnect without moving a switch. Confirm both are re-baselined, D3/D5
   resynchronize, and no toggle occurs.
3. Make the first post-reconnect transition on each switch and confirm exactly
   one independent toggle.
4. Exit Spotify fully. D3 must fail safe off while D5 remains functional.
5. Restart Spotify and allow the refresh cooldown plus one poll; confirm D3 and
   D2 recover.

### Stability

Run both switches, both external mute paths, all four pots, a default-device
change, Spotify recreation, and USB reconnect for at least ten minutes. Confirm
no disconnect loop, repeated toggle, stalled pot, crash, stale LED, or unhealthy
resource growth. Indicator messages should occur only for initial sync or
actual state changes, not every poll.

## User-observed physical validation

The user reported the following results on 2026-10-30 using the dual-mute
experiment, Nano R4 hardware, both switch/LED pairs, and the four established
potentiometers. These are hands-on observations, not automated or instrumented
measurements.

| Validation item | Result | User-observed behavior |
|---|---|---|
| D2/D3 Spotify mute | Pass | D2 controlled Spotify mute and D3 remained synchronized with the observed Spotify mute state. |
| D4/D5 default-endpoint mute | Pass | D4 controlled the current Windows default playback endpoint and D5 remained synchronized with its observed mute state. |
| Independent mute domains | Pass | Spotify application mute and default-endpoint mute operated independently. |
| Non-default routed application isolation | Pass | With Spotify on headphones and Chrome routed to UE Boom, muting the default endpoint did not silence Chrome on UE Boom. |
| Default playback device change | Pass | Changing the Windows default playback device retargeted D4/D5 to the newly selected endpoint. |
| USB reconnect safety | Pass | Disconnecting and reconnecting USB resynchronized both indicators without an unintended toggle. |
| Legacy potentiometer compatibility | Pass | All four existing rotary potentiometers continued to function correctly. |
| Practical stability soak | Pass | Extended hands-on use of both mute controls and the volume potentiometers produced no observed crash, instability, stale indicator, or unintended toggle; development console output remained clean. |

The stability result is a practical soak pass. Its exact duration was not
recorded, and it was not an instrumented stress, traffic, memory, or resource
measurement. The exact Windows build, architecture, Go version, Arduino board
core/tool version, COM port, and tested binary metadata were also not supplied;
no values are inferred for those fields.

## Automated validation

Unit tests cover both control IDs, pre-ready rejection, independent stable-state
deduplication, reconnect baseline reset, independent indicator state, external
changes, mixed Spotify sessions, endpoint/session rediscovery, missing endpoint
failure, duplicate indicator write suppression, and legacy frames.

During milestone finalization on 2026-10-30, both `GOARCH=386` and
`GOARCH=amd64` passed again:

- `go test -count=1 -buildvcs=false ./...`;
- `pkg/deej/scripts/windows/build-dev.bat`;
- the Windows routing-spike build; and
- the Track B1 Windows audio-control build.

`go vet -unsafeptr=false ./...` on both architectures reported only the
documented pre-existing unbuffered `signal.Notify` diagnostic in
`pkg/deej/util/util.go`.

Arduino CLI was not installed in the automated validation environment, so the
sketch was source-reviewed but not compiled there. The physical behavior above
was observed by the user on the uploaded Nano R4 firmware.

## Remaining risk

Automated tests simulate audio and serial boundaries and remain distinct from
the user-observed physical evidence. Firmware compilation was not independently
repeated with Arduino CLI in the automated environment. The soak was practical,
not precisely timed or instrumented, and manual edge cases not explicitly
reported above—such as operation with every playback endpoint unavailable—remain
unverified. This milestone does not promote the experimental serial protocol to
a final production interface. Do not merge this experiment to main yet.
