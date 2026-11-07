# Experimental five-channel momentary mute

> Historical stage report. Current integration, user-reported physical results,
> and validation deferrals are consolidated in [D0 integration](D0_INTEGRATION.md).


## Status and scope

Track D0 is an isolated experiment based on `main` commit
`2554efcf17898aafbb2a74de438ce86335ee0484`. It extends the physically wired
five-channel analog board with five momentary mute buttons and five
host-authoritative LEDs. It does not add encoders, routing controls, a display,
or a final six-channel protocol.

The user's standalone diagnostic firmware physically established the pin and
channel wiring. The integrated firmware and host behavior in this report remain
ready for, but not passed by, physical validation until the user records the
checklist results below.

The repeatable 15:31-15:57 release-build termination found after this report is
addressed on the isolated D0 fix branch. Its evidence, refresh coordination,
dedicated COM-owner design, automated results, and outstanding 60-minute soak
are recorded in [Track D0 runtime stability](D0_RUNTIME_STABILITY.md). This
document's original automated and physical-validation claims remain scoped to
the commit and date stated below.

## Audit and compatibility decision

The earlier host accepts latching-switch messages in this form:

```text
mixr:v1:event:switch:<control-id>:<0|1>
```

It remembers the last stable value independently for each control, rejects an
identical duplicate, and toggles Windows mute on either stable transition. That
is intentional latching-switch behavior. Reinterpreting `1` as the only action
would silently break the physically validated firmware, while sending a
momentary press and release through this path would toggle twice.

Track D0 therefore adds a narrowly scoped event family under the same versioned
namespace:

```text
mixr:v1:event:button:<control-id>:1  # stable press
mixr:v1:event:button:<control-id>:0  # stable release/rearm
```

The host deduplicates button state independently per control, toggles only a
distinct `1`, and records but never acts on `0`. Thus press/release/press causes
two actions, a held button causes one, duplicate lines do not cause extra
actions, and historical `event:switch` semantics remain unchanged.

The configuration key `hardware_mute_prototype.five_channel_momentary` defaults
to false. Older three-mute firmware and configurations keep the three existing
control identities and indicator set. Enable the key only with the Track D0
sketch; this explicit opt-in avoids silently assuming that older firmware has
the D6-D11 controls.

## Hardware and logical mapping

The table shows legacy/default bindings. With `follow_slider_mapping: true`,
the A0-A3 button/LED pairs instead follow `slider_mapping` entries 0-3. A4 and
its button/LED remain the default microphone path. Firmware control IDs stay
unchanged, even when a channel is reassigned to another application.

| Channel | Analog | Button | LED | Control ID | Default target |
|---|---:|---:|---:|---|---|
| 1 | A0 | D2 | D3 | `master-mute` | `master` (current default playback endpoint) |
| 2 | A1 | D4 | D5 | `spotify-mute` | `spotify.exe` |
| 3 | A2 | D6 | D7 | `chrome-mute` | `chrome.exe` |
| 4 | A3 | D8 | D9 | `foreground-mute` | `deej.current` |
| 5 | A4 | D10 | D11 | `microphone-mute` | `mic` (current default recording endpoint) |

Buttons are normally open to GND and use `INPUT_PULLUP`. LEDs are active-high
outputs through individual current-limiting resistors to GND. LED state never
comes from the physical button position.

## Protocol and state machines

The sketch retains 9600 baud, `mixr:v1:hello` / `mixr:v1:ready`, four-field
A0-A3 frames such as `0|256|512|1023`, and the existing A4 message:

```text
mixr:v1:event:analog:microphone-level:<0..1023>
```

It adds `chrome-mute` and `foreground-mute` to the button and indicator message
families. Host feedback remains:

```text
mixr:v1:state:indicator:<control-id>:<0|1>
```

Every button has its own raw sample, stable state, change timestamp, and 35 ms
debounce timer. After a stable edge the firmware emits exactly one event. It
emits no repeat while held. Releases are transmitted so host-side duplicate
suppression rearms, but releases never toggle.

On setup and every `hello`, firmware disables event delivery and baselines all
five current input levels. Each input must then remain stable for a debounce
interval before it is armed, without emitting that initial state. A button held
through startup or reconnect is therefore baselined as held: it causes no mute,
its later release only rearms, and only a subsequent press acts. The host also
rejects all input events before `ready` and resets its per-control deduplication
state on each accepted ready transition.

A4 is sent after a four-count firmware change and at least once per second as a
recovery heartbeat. Host deadband, inversion, endpoint clamping, and
post-rediscovery absolute reapplication are unchanged.

## Windows target and indicator behavior

- Master resolves the current default playback endpoint. A default-device
  change is observed after session refresh; unavailable playback fails safe
  with D3 off.
- Spotify and Chrome target every exact, case-insensitive session key match.
  Mixed or all-unmuted state shows LED off; all-muted shows on. A press in a
  mixed state mutes every matching session.
- Foreground resolves `deej.current` on every command/poll to the foreground
  window's process name plus distinct child-window process names already
  supported by mixr. Focus changes between known sessions retarget the next
  poll. A foreground app with no matching audio session fails safe with D9 off;
  session creation/destruction is recovered by the existing bounded refresh.
- Microphone resolves the current default recording endpoint. Missing capture
  endpoints fail safe with D11 off and recover after rediscovery; A4 remains an
  independent absolute input-level path.

Every indicator is derived from live Windows readback. Changed states are sent
immediately, and unchanged authoritative states are resent at most once every
two seconds so a lost host-to-firmware line self-heals.

Foreground resolution is intentionally bounded: it does not scan arbitrary
process trees or infer audio owned by unrelated service processes. The Windows
foreground resolver caches results for up to 350 ms, so focus indication can
lag briefly. Apps whose visible window and audio session belong to unrelated
processes may show unavailable unless the existing child-window discovery finds
the audio process.

## Configuration

The experimental branch's checked-in `config.yaml` retains the validated A0-A4
mapping and enables:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  five_channel_momentary: true
  chrome_target: chrome.exe
  foreground_target: deej.current
  poll_interval_ms: 500
  microphone_level_inverted: false
  microphone_level_deadband: 12
```

`target` remains the historical Spotify target. The two new target keys are
case-normalized and fall back with a configuration warning if empty. Master and
microphone remain fixed special targets so they always follow the current
default endpoints.

### Follow each knob's mapping

To avoid separately maintaining button targets, enable the opt-in mapping mode:

```yaml
slider_mapping:
  0: master
  1: spotify.exe
  2: chrome.exe
  3: discord.exe

hardware_mute_prototype:
  enabled: true
  five_channel_momentary: true
  follow_slider_mapping: true
  poll_interval_ms: 500
  microphone_level_inverted: false
  microphone_level_deadband: 12
```

In this mode the first four buttons and LEDs resolve the same current config
mapping as their knobs. The separate `target`, `chrome_target`, and
`foreground_target` entries are ignored. A0's button also follows entry 0,
rather than being permanently tied to master. A4 stays the default microphone;
adding `slider_mapping.4` does not remap that separate firmware path.

Changing entry 3 to `deej.current` makes both knob and button follow the active
audio application. Lists of applications are supported: the button mutes all
matched sessions unless all are already muted, in which case it unmutes all.
The LED is on only when every observed session is muted. An absent mapping or
no matching sessions leaves the LED off and performs no action; it never falls
back to the previous application's mapping. Overlapping targets are deduplicated
within each button command. Closed applications in a group contribute no live
sessions, consistent with the knob's existing behavior.

Restart once when enabling/disabling this mode or changing firmware mode.
Subsequent `slider_mapping` edits take effect after a successful existing config
reload, including the next LED poll; no restart is needed for those edits.
Reload itself never toggles mute, and it does not rearm a button held down.
Omitting `follow_slider_mapping` defaults it to false and preserves legacy
configurations. It applies only with `five_channel_momentary: true`.

The live mapping object publishes complete replacements under its lock, keeping
button/LED/knob readers safe while configuration reloads. Synthetic integration
tests cover grouped mute, case normalization, duplicate sessions, runtime
remapping, held-button deduplication, removed mappings, foreground targets and
legacy mode. Physical reconfiguration/LED verification remains pending.

## Firmware build and upload

The separate sketch and IDE steps are in
[`arduino/mixr-five-channel-momentary-mute/README.md`](../arduino/mixr-five-channel-momentary-mute/README.md).
Do not overwrite the validated hardware-mute or microphone-control sketches.
Close Arduino Serial Monitor before launching mixr.

## Automated verification

Focused Go tests cover all five identities and configured mappings; legacy
latching deduplication; momentary press/release/press, duplicate press/release,
holding semantics at the host boundary, and reconnect release rearming; five
independent indicators; Spotify/Chrome mixed aggregate behavior; foreground
switching, child-window names, and missing audio sessions; default endpoint
failure/recovery; A4 deadband/reapplication; malformed input; legacy frames;
and C2 serial reconnect/indicator resend behavior.

On 2026-11-02, both `GOARCH=386` and `GOARCH=amd64` passed:

- `go test -count=1 -buildvcs=false ./...`;
- `pkg/mixr/scripts/windows/build-dev.bat`;
- `pkg/mixr/scripts/windows/build-release.bat`;
- the Windows routing-spike build;
- the Windows audio-control diagnostic build; and
- the Windows microphone-control diagnostic build.

`go vet -unsafeptr=false ./...` on both architectures reported only the
documented pre-existing unbuffered `signal.Notify` diagnostic in
`pkg/mixr/util/util.go`. The sandbox denied nonessential Go module stat-cache
writes during builds, but every build command returned success and produced its
requested executable. Arduino CLI was not installed, so the new sketch was
source-reviewed and its exact pin/protocol contract is covered by a Go source
contract test, but it was not compiled here. Automated and source-review
evidence does not count as physical validation.

## Physical validation checklist

Record date/time, commit, Windows edition/build, tested architecture, Go
version, Arduino IDE/CLI and board-core versions, Nano R4 identity, COM port,
firmware file/hash, default playback and recording endpoints, Spotify/Chrome
session counts, wiring, observed results, and every untested case. Mark an item
passed only from direct observation.

1. Upload the Track D0 sketch, start mixr, and confirm one successful
   `hello`/`ready` sequence, no startup mute, and five Windows-derived LEDs.
2. Sweep A0-A3 and confirm independent master, Spotify, Chrome, and foreground
   volume. Sweep A4 and confirm default-recording level, inversion, endpoint
   clamps, and deadband behavior.
3. Press D2, D4, D6, D8, and D10 independently. Confirm one correct mute action
   per press and the paired D3, D5, D7, D9, and D11 response.
4. For every button, hold longer than debounce and several poll intervals,
   release, and press repeatedly. Confirm no hold repeat and no release action.
5. Change all five mute states externally in Windows. Confirm each LED follows
   actual readback and recovers within two seconds if a serial state line is
   intentionally missed.
6. Exercise multiple Spotify and Chrome audio sessions. Verify all-muted is on,
   all-unmuted/mixed is off, and a mixed-state press mutes all sessions.
7. Move foreground focus among two audio apps and an app with no audio session.
   Confirm D8 targets only the currently resolved app and D9 clears for the
   unavailable target. Create/destroy a foreground app session and verify
   bounded recovery without stale state from the previous app.
8. Change the Windows default playback endpoint. Confirm D2/D3 retargets only
   the new default and other routed application audio remains independent.
9. Disable/remove the default recording endpoint. Confirm D11 off, D10/A4 fail
   safely, and both recover after the endpoint returns. If a second input is
   available, switch defaults and verify retargeting; otherwise record the
   equipment limitation.
10. Disconnect/reconnect USB with all buttons released. Confirm no mute action,
    five LEDs resynchronize, and the next press on each control works once.
11. Repeat reconnect while each button is held in turn. Confirm reconnect and
    release cause no action, then the next press works once.
12. Start mixr with USB disconnected, verify reconnecting tray status and
    rate-limited diagnostics, then connect and confirm recovery without restart.
13. Verify tray Connected/Reconnecting state, Refresh Sessions, Edit
    Configuration, About/version, Open Log, and Quit behavior using the C2
    checklist.
14. Run all analog controls, buttons, external state changes, focus changes,
    endpoint changes, and one reconnect for at least ten minutes. Record any
    stale LED, extra toggle, stalled analog path, reconnect loop, crash, noisy
    routine log, or unusual resource growth.

## Known risks and untested cases

- Integrated Track D0 firmware behavior is not physically passed until the user
  confirms the checklist.
- Polling observes external changes rather than registering per-session mute
  callbacks, so indication latency is bounded by the configured poll interval.
- Foreground resolution has the documented process-ownership limitations above.
- On the D0 stability-fix branch, application session creation is subject to a
  global refresh budget and the ordinary 45-second freshness path; stale
  snapshots can briefly show unavailable without causing per-control refresh
  amplification. See [Track D0 runtime stability](D0_RUNTIME_STABILITY.md).
- Arduino compilation is environment-dependent and must be recorded separately
  if Arduino CLI and the installed Nano R4 core are unavailable here.
- The experimental five-channel interface is not the final six-encoder VFinal
  control surface or protocol.

### Mapping-mode validation — 2026-11-06

Based on the isolated foreground callback fix 46ddbf5. Full
`go test -count=1 -buildvcs=false ./...`, established Windows development and
release builds, and routing/audio/microphone diagnostic builds passed for both
386 and amd64. Vet reported only the documented existing unbuffered
signal.Notify warning in util.go:43. The existing module stat-cache write warning
was non-fatal. Race testing remains unavailable because CGO is disabled.
No firmware changes or upload were performed. The final release keeps the
foreground callback fix and a COM5 runtime config with follow_slider_mapping
enabled; its separately retained bundle is identified in the delivery notes.
Physical button/LED remapping, config-save behavior on the user's editor,
combined Nano controls, reconnect, endpoint changes and the long stability soak
remain unverified. No application was launched or stopped during this change.
