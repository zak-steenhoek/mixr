# mixr architecture decision log

This is the authoritative lightweight record of established design decisions.
Statuses distinguish decisions already embodied in the baseline from choices for
planned VFinal work.

## D-001 — Go single-executable Windows tray client (Accepted)

**Decision.** Keep the host application Go-based, lightweight, portable, and
delivered as a single Windows system-tray executable.

**Rationale.** This preserves the proven deej operating model and simple
deployment. **Consequences.** Native Windows integration remains behind Go/COM
boundaries; avoid services, installers, and heavy frameworks without a new
decision. **Limitations.** The module and application package now use mixr
identity, while Linux heritage exists even though VFinal targets Windows.

## D-002 — Relative encoders for channel volume (Planned)

**Decision.** VFinal channel volume controls use relative rotary encoders.

**Rationale.** Relative input can follow host-side state without a physical
absolute-position conflict. **Consequences.** Firmware must report direction and
steps, and the host owns the resulting volume value. The Track B1 software
foundation reads each matching session's actual volume for every movement,
applies a configurable increment, and clamps the result; see
[relative volume and mute](RELATIVE_VOLUME_MUTE.md). **Limitations.** Resolution,
acceleration, and encoder part are unresolved; the four-pot path remains a
compatibility requirement, and no encoder serial messages exist yet.

## D-003 — Momentary mute with software indicators (Planned)

**Decision.** Each channel uses a momentary mute button and a
software-controlled indicator.

**Rationale.** Software-authoritative feedback stays correct when state changes
outside the hardware. **Consequences.** Mute becomes bidirectional state rather
than a one-way switch. For a multi-session application, state is `all-muted`,
`all-unmuted`, or `mixed`; toggle unmutes only an all-muted target and otherwise
mutes every session. Track B1 reads actual state and uses audited 32-bit Windows
`BOOL` bindings; see [relative volume and mute](RELATIVE_VOLUME_MUTE.md).
**Limitations.** Indicator technology and asynchronous external-change
notification details are unresolved, and no button/indicator protocol exists
yet.

## D-004 — Default-endpoint master (Accepted)

**Decision.** Master volume affects only the current default playback endpoint.

**Rationale.** This matches the validated baseline and keeps routed application
audio independent. **Consequences.** A routed app may remain audible when master
changes another endpoint. **Limitations.** Endpoint-role and default-device
changes require physical regression.

## D-005 — Five persistent application routes (Planned)

**Decision.** The five non-master channels each have a persistent playback-output
selector.

**Rationale.** Each controlled application can keep its intended output across
session recreation and reboot. **Consequences.** Every policy write must be read
back, original/default state must be recoverable, and live Core Audio PIDs must
be resolved. **Limitations.** Windows mutation APIs are undocumented and
version-sensitive; the validated spike is feasibility evidence, not production.

## D-006 — One centered high-resolution display (Planned)

**Decision.** Use a single centered display for channel and system feedback.

**Rationale.** One shared surface can present richer state without multiplying
small displays. **Consequences.** UI state must be derived from host-authoritative
data and sent to hardware. **Limitations.** Panel, resolution, layout, interface,
and refresh behavior are unresolved.

## D-007 — Nano R4 for prototyping (Accepted)

**Decision.** Retain the Arduino Nano R4 for prototype development.

**Rationale.** It is already physically validated with the four-pot baseline.
**Consequences.** Near-term firmware experiments should run on the known board
and keep legacy input working. **Limitations.** This does not select the final MCU
or PCB topology.

## D-008 — Bidirectional USB serial (Planned)

**Decision.** Extend USB serial to support host-to-hardware feedback as well as
hardware-to-host input.

**Rationale.** Encoder, mute-indicator, and display state require host feedback.
**Consequences.** The protocol needs framing, versioning, validation, and safe
recovery from partial/unknown messages. **Limitations.** The message schema is
unresolved, and original deej frames must continue to work.

## D-009 — Audited Windows COM compatibility layer (Accepted)

**Decision.** Use `internal/wincom` for corrected native calls rather than unsafe
legacy dependency methods.

**Rationale.** The audit found pointer-width, `HRESULT`, `BOOL`, output-width,
callback, and lifetime defects. **Consequences.** New COM calls require explicit
ABI review and tests on 386 and amd64. **Limitations.** The layer covers audited
call paths, not every method in the pinned dependency; see
[WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md).

## D-010 — Original deej protocol compatibility (Accepted)

**Decision.** Preserve the original potentiometer firmware, configuration, and
pipe-delimited serial protocol while mixr evolves.

**Rationale.** The known-good hardware remains a regression reference and usable
fallback. **Consequences.** Protocol extensions must be additive or explicitly
migrated after approval and validation. **Limitations.** The legacy stream is
one-way and expresses absolute analog values, so it cannot alone provide VFinal
feedback.

## Maintenance rule

Update this log when an architecture decision changes or a new consequential
choice is accepted. Do not add entries for routine implementation details.
