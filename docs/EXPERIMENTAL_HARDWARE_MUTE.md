# Experimental Nano R4 Spotify mute integration

## Status and scope

This branch is an isolated proof of concept built on Track B1. It connects one
Nano R4 latching switch to the Track B1 Spotify mute toggle and drives one red
LED from mute state read back from Windows. It does not define the final
bidirectional protocol or implement final buttons, indicators, encoders,
routing selectors, display behavior, channel layout, PCB, or enclosure.

The original potentiometer frame remains unchanged: newline-delimited
`0..1023` values separated by `|` at 9600 baud. The known-good reference sketch
is untouched. Experimental firmware lives in
`arduino/mixr-hardware-mute-prototype/`.

User-reported prerequisites on 2026-10-30 were:

- Track B1 Spotify mute, unmute, toggle, two-session handling, and relative
  volume passed audible Windows testing.
- The Nano R4 four-pot controller continued working independently.
- A diagnostic sketch confirmed the D2 latching switch and D3 red LED wiring.

Those prerequisites are not evidence that this branch's end-to-end integration
has passed physical validation.

## Experimental v1 line protocol

Namespaced messages are ordinary newline-delimited ASCII lines and cannot be
mistaken for a legacy numeric slider frame.

| Direction | Message | Meaning |
|---|---|---|
| Host to Nano | `mixr:v1:hello` | Start/restart a connection handshake. |
| Nano to host | `mixr:v1:ready` | D2 was re-baselined without emitting an event. |
| Nano to host | `mixr:v1:event:switch:spotify-mute:0` | Debounced stable transition to switch open. |
| Nano to host | `mixr:v1:event:switch:spotify-mute:1` | Debounced stable transition to switch closed. |
| Host to Nano | `mixr:v1:state:indicator:spotify-mute:0` | Turn D3 off. |
| Host to Nano | `mixr:v1:state:indicator:spotify-mute:1` | Turn D3 on. |

The host sends `hello` after every serial connection and retries it until
`ready`. Firmware samples D2 when it receives `hello`, waits for a stable
debounce interval, and arms events without reporting the sampled state. The
host also rejects switch events before `ready` and rejects repeated copies of
the same stable state. These two layers prevent startup/reconnect state from
becoming a mute toggle.

Every later stable D2 transition requests exactly one Track B1 `toggle-mute`
operation for the configured target. The LED never follows D2 directly. The
host reads aggregate mute state from Windows after the toggle and polls it for
external changes.

For multiple Spotify sessions:

- LED on means every observed session is muted.
- LED off means all sessions are unmuted, the sessions are mixed, or no current
  state is observable.
- A switch transition in a mixed state uses Track B1's deterministic behavior
  and mutes every matching session; readback then turns the LED on.

Indicator writes are cached per serial connection. Unchanged poll results do
not write another line. The cache resets on reconnect, guaranteeing one fresh
state synchronization after `ready`.

## Firmware build and upload

Wiring and both Arduino IDE and CLI instructions are in
`arduino/mixr-hardware-mute-prototype/README.md`. The prototype uses:

- `A0` through `A3` for the four existing potentiometers;
- `D2` to GND with `INPUT_PULLUP` for the latching switch;
- `D3` through 1 kΩ and the red LED to GND; and
- 9600-baud USB serial.

Do not upload over the known-good reference sketch without retaining a copy or
being prepared to restore it. Close Arduino Serial Monitor before starting the
host application because only one process can own the COM port.

## Host configuration and build

Keep the existing `slider_mapping`, COM port, baud rate, inversion, and noise
settings. Add only this opt-in block to `config.yaml`:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  poll_interval_ms: 500
```

The poll interval has a 100 ms lower bound. This experimental block is read at
startup; restart mixr after changing it.

Build the host from this branch's repository root:

```powershell
$env:GOARCH = 'amd64'
cmd /c "call pkg\deej\scripts\windows\build-dev.bat"
Remove-Item Env:GOARCH
```

Place `config.yaml` beside `mixr-dev.exe`, close Serial Monitor, and run verbose
diagnostics:

```powershell
.\mixr-dev.exe --verbose
```

Expected diagnostics include connection generation, `hello`/`ready`, accepted
debounced switch state, Spotify command/readback state, session refresh after a
restart, and LED synchronization. The serial layer automatically retries after
an established USB connection is lost.

## End-to-end physical validation procedure

Record the branch commit, Windows version/build, Go version, Nano R4 board/core
version, Arduino IDE/CLI version, COM port, architecture, default endpoint,
Spotify PIDs/session count, wiring, each observed result, and all untested cases.

### 1. Startup and legacy four-pot regression

1. Start audible sources for the existing master, Chrome, Spotify, and
   foreground-app mappings.
2. Start `mixr-dev.exe --verbose` with the switch left in either stable position.
3. Confirm the log shows a connection, handshake, and `ready`, but no accepted
   switch transition or Spotify toggle during initialization.
4. Move each of the four potentiometers through low, middle, and high positions.
5. Confirm the legacy mappings remain independent and their lines remain plain
   four-value pipe-delimited numeric frames.

### 2. Switch toggle and LED readback

1. Start Spotify playback and confirm both matching Spotify sessions are shown
   in verbose command readback.
2. Move D2 to its other stable position once.
3. Confirm exactly one accepted transition and one Spotify toggle are logged,
   both Spotify sessions become muted, and D3 turns on after readback.
4. Move D2 back once. Confirm exactly one new toggle, both sessions unmute, and
   D3 turns off.
5. Hold the switch steady and lightly disturb it mechanically. Confirm debounce
   does not produce repeated toggle logs.

### 3. Mixed-session behavior

1. In Windows Volume Mixer, mute only one of the two Spotify sessions.
2. Within the configured poll interval, confirm the log reports `mixed` and D3
   is off.
3. Move D2 once. Confirm both sessions become muted and D3 turns on.
4. Move D2 again. Confirm both sessions become unmuted and D3 turns off.

### 4. External Windows changes

1. Without touching D2, mute all Spotify sessions in Windows.
2. Confirm D3 turns on after observed readback.
3. Unmute Spotify externally and confirm D3 turns off.
4. Verify no switch event or mute command is logged for either external change.

### 5. USB reconnect safety

1. Note Spotify's current mute state and leave D2 stable.
2. Disconnect the Nano USB cable and wait for the host to log connection loss
   and retry attempts.
3. Reconnect USB without moving D2.
4. Confirm a new connection generation completes `hello`/`ready`, D3 is restored
   from Windows state, and no Spotify toggle occurs.
5. Move D2 once and confirm the first post-reconnect transition toggles exactly
   once.

### 6. Spotify session recreation

1. Fully exit Spotify and confirm failed/unavailable readback turns D3 off rather
   than leaving a stale on indication.
2. Reopen Spotify and start playback.
3. Allow up to the five-second session-refresh cooldown plus one poll interval.
4. Confirm the new sessions are discovered and D3 updates to their actual state.
5. Toggle D2 and confirm every recreated matching session follows.

### 7. Stability and traffic

1. Run all four pots, switch, external mute, and Spotify restart cases for at
   least ten minutes.
2. Confirm there is no serial disconnect loop, repeated toggle, stalled pot,
   crash, or unhealthy resource growth.
3. In verbose logs, confirm LED messages occur only on state changes or initial
   synchronization, not on every poll.

## Automated validation

On 2026-10-30, both `GOARCH=386` and `GOARCH=amd64` passed:

- `go test -count=1 -buildvcs=false ./...`;
- the existing mixr development build;
- the Windows routing-spike build; and
- the Track B1 software audio-control build.

Tests simulate separation of legacy/prototype frames, pre-ready rejection,
stable-state deduplication, both switch transitions, mixed mute behavior,
external changes, session recreation/refresh, duplicate LED-write suppression,
and automatic reconnect after an established connection loss. These tests do
not exercise a physical COM port, Nano R4, LED, switch, or audible output.

`go vet -unsafeptr=false ./...` reports only the documented pre-existing
unbuffered `signal.Notify` diagnostic on both architectures.

The Arduino CLI is not installed in the validation environment, so the sketch
was source-reviewed but not compiled or uploaded here.

## Limitations and unresolved items

- End-to-end physical validation is pending; this branch must not be merged or
  tagged as a milestone based on automated checks alone.
- This protocol has one fixed control/indicator identifier and no negotiation,
  checksums, acknowledgements, sequencing, or firmware capability discovery.
- Indicator updates are polling-based rather than Core Audio notification-based.
- D3 cannot display all three multi-session states; mixed is deliberately off
  and is distinguished in logs.
- LED writes are not acknowledged by firmware. A successful host serial write
  is treated as delivery; reconnect or a later state change resynchronizes it.
- The five-second refresh cooldown bounds repeated session enumeration but can
  delay Spotify-restart recovery by up to that interval plus one poll.
- Prototype configuration changes require a host restart.
- The firmware retains the legacy frequent analog frame cadence. The prototype
  adds only handshake, edge, and changed-indicator messages; it does not attempt
  a final bandwidth/framing redesign.
- Existing COM apartment/lifetime constraints from
  `WINDOWS_AUDIO_HARDENING.md` remain unchanged.
