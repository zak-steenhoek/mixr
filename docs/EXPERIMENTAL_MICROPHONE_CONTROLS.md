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
