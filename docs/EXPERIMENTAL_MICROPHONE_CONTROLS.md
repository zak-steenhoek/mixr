# Experimental microphone controls

## Status and scope

Track B2 is an isolated experiment extending the validated dual hardware-mute
baseline. Phase 1 hardware diagnostics passed with one measured analog coupling
observation. Phase 2 investigates only the Windows default recording endpoint;
serial and integrated firmware changes have not started.

This experiment does not change the final six-channel product scope, promote a
serial protocol to production, or alter the validated milestone.

## Phase 1 hardware diagnostic

The standalone sketch and full procedure are in
[`arduino/mixr-microphone-controls-diagnostic/`](../arduino/mixr-microphone-controls-diagnostic/README.md).
It uses the proposed physical mapping only for host-independent diagnostics:

| Input | Diagnostic output | Logical target |
|---|---|---|
| D2 latching switch to GND | D3 red LED | Default playback mute |
| D4 latching switch to GND | D5 red LED | Spotify mute |
| D6 latching switch to GND | D7 red LED | Default recording mute |
| A4 slider | Bounded serial ADC report | Recording input level |

The user reported on 2026-10-30 that all three buttons and LEDs behaved as
expected on the first upload. A4 covered `0..1023` with D6 open/D7 off. With D6
closed and D7 illuminated, the observed A4 minimum rose to approximately `8`.
This hardware-state-dependent offset is recorded as unresolved analog coupling;
it must be tolerated without being hidden by later filtering.

The report did not include exact board/core/tool versions, COM port, a timed
stability duration, or instrumented electrical measurements, so none are
inferred.

## Phase 2 recording-endpoint diagnostic

`cmd/windows-microphone-control` is serial-independent and uses documented Core
Audio `IMMDeviceEnumerator` and `IAudioEndpointVolume` interfaces. It selects
the current `EConsole` default capture endpoint independently of playback,
prints actual endpoint ID/volume/mute readback, and can reacquire the endpoint
during bounded polling. Missing endpoints return an error or an `unavailable`
watch state rather than falling through to a playback target.

### Live evidence

On 2026-10-30, the diagnostic discovered the current default recording endpoint
with ID `{0.0.1.00000000}.{57e89eab-f32f-4baa-bf2d-fdf396af2dce}`. It read an
initial normalized level of `0.651658` and an unmuted state.

The diagnostic temporarily set the level to `0.55`, read back exactly `0.55`,
and restored/read back `0.651658`. It then muted and unmuted the endpoint,
reading back both states. A separate watcher process observed an external
unmuted → muted → unmuted sequence without changing endpoint ID or volume. The
final explicit readback was `0.651658`, unmuted.

These checks establish discovery, scalar level access, mute access, mutation
readback, and polling-based external-change observation on the current machine.
They do not yet establish audible microphone capture or default capture-endpoint
switching.

### User-observed removal and external-change evidence

The user supplied a live watcher capture on 2026-10-30. With endpoint ID
`{0.0.1.00000000}.{57e89eab-f32f-4baa-bf2d-fdf396af2dce}`, the watcher reported
external level changes through `0.40`, `0.27`, `0.71`, `0.59`, and `0.37`, plus
an external mute transition to `true` and back to `false`. It then reported
`default recording endpoint unavailable: Element not found.` when the input was
removed or disabled. The same running watcher recovered automatically when the
endpoint returned and reported volume `0.37`, unmuted, with the original ID.

This physically confirms external volume/mute observation and missing-endpoint
recovery without restarting the diagnostic. Default capture-endpoint switching
remains untested because the user has no second microphone input; this is an
explicit equipment limitation, not a failed result.

For both `386` and `amd64`, the full Go test suite, development client build,
routing-spike build, and microphone-control diagnostic build passed. This
automated result does not replace the open physical endpoint-switching check or
the later integrated hardware regression.

## Phase 3 integrated prototype

Phase 3 uses the separate
[`arduino/mixr-microphone-controls-prototype/`](../arduino/mixr-microphone-controls-prototype/README.md)
sketch so the validated dual-mute firmware remains unchanged. Physical pins are
D2/D3 default playback, D4/D5 Spotify, and D6/D7 default recording. The logical
IDs `master-mute` and `spotify-mute` are preserved despite their pin remapping.

The four-field legacy numeric frame remains unchanged. A4 uses the smallest
additive namespaced extension:

```text
mixr:v1:event:analog:microphone-level:<0..1023>
```

Firmware reports A4 after a four-count change and once per second as a recovery
heartbeat. Host-side `microphone_level_deadband` defaults to 12 ADC counts,
which suppresses the observed approximately eight-count D7 LED offset. Values
within the deadband of either endpoint clamp to exactly 0 or 1. The first A4
event after `ready` intentionally applies absolute microphone level; switch
positions are only baselined and never become startup mute actions.

Configuration remains under the existing opt-in feature flag:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  poll_interval_ms: 500
  microphone_level_inverted: false
  microphone_level_deadband: 12
```

Microphone mute targets the current `mic` session. D7 is driven from actual
Core Audio readback, including external changes. Missing capture endpoints fail
safe with D7 off. Polling refreshes stale/missing sessions, and the next A4
heartbeat reapplies the absolute slider position after capture-endpoint
rediscovery. Application playback volume is not part of the microphone-level
path.

This integrated behavior is implemented but not physically validated. Do not
merge or tag it until the manual regression checklist passes.

### User-observed integrated control checkpoint

On 2026-10-30, the user uploaded the integrated prototype sketch and confirmed
all items in the initial end-to-end checkpoint:

- the host completed the `hello`/`ready` handshake without a startup mute;
- D3, D5, and D7 synchronized to default-playback, Spotify, and default-
  recording mute state respectively;
- D2 controlled only default-playback mute, D4 only Spotify mute, and D6 only
  default-recording mute;
- A4 changed the Windows microphone input level while A0–A3 retained their
  original mappings; and
- at A4 minimum, toggling D6/D7 did not produce a visible microphone-level
  change from the previously measured approximately eight-count ADC offset.

These observations validate the basic integrated mappings and configured
deadband on the user's hardware. External-state synchronization, USB recovery,
integrated microphone removal/restoration, and the ten-minute combined soak
remain open.

During the next checkpoint, default-playback and microphone indicator sync plus
USB recovery passed. Spotify D5 intermittently failed to follow an external
Windows Mixer change until Spotify mute changed in the opposite direction;
after that reversal, external changes followed again. This pattern is
consistent with one host-to-firmware indicator line being missed after the host
had cached it as sent: identical state polls were then suppressed, while an
opposite state produced a new line and repaired the mismatch.

The host now retains immediate duplicate suppression but authoritatively
resends each unchanged indicator state every two seconds. This adds at most 1.5
short lines per second across all three indicators and bounds recovery from one
missed LED command without requiring a mute reversal. Automated tests cover
both immediate suppression and the delayed resend. The D5 behavior requires
physical revalidation with repeated external Spotify mute changes before this
checkpoint can pass.

After rebuilding the host with the bounded resend, the user confirmed that D5
followed repeated external Spotify mute changes and that the previously observed
stale-indicator behavior no longer reproduced. This physically validates the
two-second self-healing mitigation on the tested hardware. Together with the
previous report, external synchronization for D3, D5, and D7 plus USB
disconnect/reconnect recovery now pass. Integrated microphone removal/
restoration and the combined ten-minute soak remain open.

The user then disabled the active Windows recording device while the integrated
host remained running and confirmed the expected fail-safe behavior. D7 turned
off, D6 caused no crash or unrelated mute, and the microphone became
unavailable. After re-enabling the device without restarting mixr, D7 recovered
to actual Windows mute state, A4 reapplied its absolute position, and D6/A4
resumed normal control. Integrated recording-endpoint removal and restoration
therefore pass on the tested hardware. The ten-minute combined stability run is
in progress and is not yet recorded as passed.

### Automated integration validation

On 2026-10-30, both `GOARCH=386` and `GOARCH=amd64` passed the full Go test
suite, development client build, routing-spike build, Track B1 audio-control
build, and microphone-control diagnostic build. Tests cover the three logical
mute controls, independent duplicate rejection, microphone indicator readback,
missing/default recording endpoint rediscovery, A4 endpoint clamping and
deadband, post-reconnect absolute synchronization, and unchanged four-field
legacy frames.

`go vet -unsafeptr=false ./...` on both architectures reported only the
documented pre-existing unbuffered `signal.Notify` warning in
`pkg/deej/util/util.go`. Arduino CLI is unavailable, so the integrated sketch
was source-reviewed but not compiled in this environment.
