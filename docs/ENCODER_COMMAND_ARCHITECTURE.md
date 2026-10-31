# Encoder and volume-preset command architecture proposal

## Scope

This is design-only guidance for a future encoder track. C2 adds no encoder
serial events, configuration keys, preset execution, or firmware behavior.

## Proposed command boundary

Firmware should report physical intent, not calculate host volume. A future
channel event adapter should translate validated serial events into the existing
transport-independent `AudioCommand` boundary:

| Physical action | Command intent | Existing Track B1 support |
|---|---|---|
| Encoder clockwise/counterclockwise | Signed relative steps plus configured scalar increment | Yes: `AudioCommandRelativeVolume` reads current volume, applies `Steps * Increment`, clamps, writes, and reads back. |
| Encoder pushbutton | Set configured absolute preset percentage | Yes at the command layer: `AudioCommandSetVolume` clamps `Volume` and reads back. Configuration/event wiring is not implemented. |
| Separate mute button | Toggle mute independently | Yes: `AudioCommandToggleMute` handles endpoint and multi-session targets deterministically. |
| Indicator LED | Display actual Windows mute state | Yes on query: `AudioCommandCurrentMute`; transport and external-change scheduling remain future work. |

Absolute and relative changes remain distinct command operations. Relative
movement contains signed steps and a positive normalized increment and starts
from fresh Windows state. A preset contains normalized absolute volume and does
not derive a delta. Both use established target resolution; neither reads or
writes mute state.

## Preset resolution

Future configuration should support one preset per channel with a global
fallback. Resolution should use the channel value first, then the global value,
then a documented future default. The result clamps to `0..100` before conversion
to `0.0..1.0`.

Examples such as 0%, 50%, 75%, and 100% require no special semantics. An
application preset sets every matching session to the same absolute value and
reads each back. A `master` preset applies only to the current default playback
endpoint. Presets leave mute unchanged even at 0%.

## Future implementation seams

A future track needs a backward-compatible event schema; configuration for
global/per-channel increments and presets; a channel adapter emitting
`AudioCommand`; authoritative indicator scheduling; and tests for deduplication,
acceleration policy, reconnect baselining, fallback, clamping, and unchanged
mute. Encoder acceleration, detent resolution, batching, and asynchronous
Windows notifications remain unresolved pending physical evidence.
