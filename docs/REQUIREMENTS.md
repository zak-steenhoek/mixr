# mixr VFinal requirements

This is the authoritative functional, interface, compatibility, and
nonfunctional requirement set. `Validated` means supported by recorded evidence,
`Planned` means agreed scope not yet implemented, and `Unresolved` means a
product or engineering choice remains open. Validation applies only to the
environment and limits stated in the linked report.

## Verified V0/V1 baseline

| ID | Requirement | Status | Acceptance / evidence |
|---|---|---|---|
| AUD-001 | The master control changes only the Windows default playback endpoint volume. | Validated | Four-pot physical test in [BASELINE_VALIDATION.md](BASELINE_VALIDATION.md) and default-device regression in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| AUD-002 | Mapped application controls adjust matching sessions independently of master and other mappings. | Validated | Chrome, Spotify, and foreground-app isolation physically verified in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| AUD-003 | Recreated supported application sessions are reacquired without restarting mixr. | Validated | Chrome, Spotify, and Discord recreation results in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| HW-001 | The prototype supports an Arduino Nano R4 connected over USB serial with four potentiometers. | Validated | Physical configuration and results in [BASELINE_VALIDATION.md](BASELINE_VALIDATION.md). |
| SER-001 | mixr accepts the original deej stream: newline-delimited `0..1023` values separated by `|`, at the configured baud rate (currently 9600). | Validated | Nano R4 four-pot test in [BASELINE_VALIDATION.md](BASELINE_VALIDATION.md). |
| CMP-001 | Existing deej YAML mappings, potentiometer firmware, and special mapping names remain compatible. | Validated | Compatibility baseline plus current `deej.current` physical result in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| APP-001 | The Windows client remains a lightweight, portable Go system-tray application built as a single executable. | Validated | Current build scripts and both-architecture build matrix in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| COM-001 | Production Windows audio calls use audited, native-width COM bindings on both `386` and `amd64`. | Validated | ABI audit, compatibility layer, and automated/live checks in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md). |
| RTE-001 | Windows can persist and read back a per-application playback preference without changing default-endpoint master behavior. | Validated | Feasibility only, on the recorded Windows build; see [WINDOWS_ROUTING_SPIKE.md](WINDOWS_ROUTING_SPIKE.md). This is not yet a product feature. |

## VFinal capabilities

| ID | Requirement | Status | Acceptance criteria / open point |
|---|---|---|---|
| AUD-101 | Provide six independent physical volume channels: one master and five application channels. | Planned | Each control changes only its selected target; master continues to affect only the default playback endpoint. |
| AUD-102 | Each channel provides momentary mute control with a software-authoritative mute indicator. | Planned | A press changes the intended mute state and the indicator reflects the state read from software, including external changes where observable. |
| RTE-101 | Each of the five application channels provides a persistent playback-output selector. | Planned | The selected route survives application session recreation and Windows reboot, is read back after every write, and can be restored to system default. |
| RTE-102 | Routing failures are non-destructive and visible; unavailable or unsupported policy APIs retain a Windows Volume Mixer fallback. | Planned | Failed verification is reported and is never presented as a successful route change. |
| HW-101 | Channel volume input uses relative rotary encoders rather than absolute potentiometers in the final control surface. | Planned | Direction and step input adjust software volume without requiring an absolute knob position. Encoder resolution/acceleration remains an implementation choice. |
| HW-102 | The Nano R4 remains the prototype controller. | Validated | Current prototype is physically proven; this does not select the production MCU. |
| SER-101 | USB communication becomes bidirectional for host commands and hardware state feedback while retaining `SER-001` compatibility. | Planned | Legacy one-way frames still operate; malformed or unsupported bidirectional messages fail safely. Message framing is not yet specified. |
| UI-101 | One centered high-resolution display presents relevant channel/system state. | Planned | Displayed state agrees with host-authoritative state. Specific panel hardware, resolution, and UI layout are unresolved. |
| PHY-101 | The final device uses a custom PCB and enclosure suitable for the six-channel interface. | Planned | PCB topology, production MCU, connector placement, materials, and enclosure dimensions are unresolved. |

## Nonfunctional and validation requirements

| ID | Requirement | Status | Acceptance criteria |
|---|---|---|---|
| NFR-001 | Changes remain modular, testable, lightweight, incremental, and dependency-minimal. | Planned | New dependencies require a documented need; hardware/OS boundaries are isolated behind narrow interfaces. |
| NFR-002 | Supported Windows audio and COM code builds and tests on `GOARCH=386` and `GOARCH=amd64`. | Validated | Current matrix passed in [WINDOWS_AUDIO_HARDENING.md](WINDOWS_AUDIO_HARDENING.md); every affected change must repeat it. |
| NFR-003 | USB, audio-session, routing, callback, or hardware behavior changes receive physical regression validation. | Planned | Record environment, architecture, procedure, observed behavior, and untested cases before milestone integration. |
| NFR-004 | Documentation distinguishes verified results, assumptions, planned behavior, and unresolved choices. | Planned | Requirements and decisions link to evidence instead of converting expectations into claims. |

## Maintenance rule

Update this file when a requirement changes. When a feature is completed, change
its status only after linking validation evidence. Implementation-only changes
need no update unless they alter an important interface or operating procedure.
