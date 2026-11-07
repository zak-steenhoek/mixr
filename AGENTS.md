# Repository guidance

## Purpose and architecture

mixr is a fork of deej evolving into a six-channel Windows desktop audio
controller. The current application is a lightweight Go system-tray process:
`pkg/mixr/cmd` starts the client, `pkg/mixr` owns configuration, USB serial,
session discovery, and volume control, and Arduino firmware supplies control
values. Windows Core Audio interop is isolated behind `internal/wincom` where
the pinned dependency is unsafe.

The authoritative product scope is [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md),
and established design choices are in [docs/DECISIONS.md](docs/DECISIONS.md).
Keep them current when requirements or architecture decisions change, or when
implementation completes and validation evidence exists. Do not update them for
ordinary implementation-detail changes unless an important interface or
operational procedure changes.

## Current state

- Verified: portable Windows tray application; original deej configuration and
  serial protocol; Nano R4 with four potentiometers; independent master, Chrome,
  Spotify, and foreground-app volume control; 386/amd64 audio paths; and the
  routing feasibility spike.
- Track D0: five-channel analog momentary mute/LED and microphone prototype,
  refresh/COM ownership fixes, and callback regression guard are integrated.
  User-reported soak, channel/LED, microphone, endpoint and USB recovery passed
  on `5b955d1`; dynamic foreground switching and race testing remain deferred.
  See [D0 integration and qualification](docs/D0_INTEGRATION.md).
- Planned VFinal scope: six relative-encoder channels and their mute/indicators,
  five persistent application routes, final bidirectional hardware feedback,
  and the central display. D0's additive analog prototype is not this final
  six-channel interface.
- Unresolved: final display part, production MCU, PCB topology, and enclosure.

Never describe a spike or planned design as production functionality. Evidence
is recorded in [baseline validation](docs/BASELINE_VALIDATION.md),
[Windows audio hardening](docs/WINDOWS_AUDIO_HARDENING.md), and the
[routing spike report](docs/WINDOWS_ROUTING_SPIKE.md).

## Repository map

- `pkg/mixr/`: application, tray, configuration, serial, and audio sessions.
- `pkg/mixr/scripts/windows/`: supported Windows build scripts.
- `internal/wincom/`: audited native-width COM compatibility layer and ABI tests.
- `cmd/windows-routing-spike/`: isolated routing diagnostic/feasibility CLI.
- `arduino/deej-5-sliders-vanilla/`: compatible one-way reference firmware.
- `docs/`: requirements, decisions, validation reports, and contributor notes.

## Efficient task discovery

- Start with this file and the files directly relevant to the requested task.
  Consult the latest applicable milestone report or architecture document when
  its decisions or validation evidence are needed; do not automatically read
  every historical document, source file, or entire directory.
- Expand exploration as needed to understand dependencies, interfaces,
  regressions, compatibility, or architectural implications. Reuse findings
  from the current session instead of repeatedly reading unchanged files, and
  prefer targeted searches, diffs, and file sections over broad repository
  dumps.
- Keep progress reports and final summaries concise while retaining material
  test results, limitations, and commit information. Never trade correctness,
  safety, required regression coverage, or compliance with the rules below for
  lower context or token usage.

## Change discipline

- Branch focused work as `track-<letter>/<scope>` and exploratory work as
  `experiment/<scope>`, consistent with repository history. Keep commits small,
  descriptive, and limited to one concern; do not mix opportunistic refactors
  into feature or compatibility work.
- Treat `baseline/deej-working` as the known-good original baseline and
  `baseline/v1-start` as the V1 starting point. Milestone tags record physically
  validated integrations. Do not move or replace baseline/milestone tags.
- Before integration, compare against the applicable baseline, run both-arch
  checks, perform required physical regression, document the result, then merge
  and tag. A build or unit test alone is not a regression pass.
- Preserve the existing potentiometer path, YAML mappings, 9600-baud USB serial,
  and newline-delimited `0..1023` values separated by `|`. Protocol evolution
  must be backward compatible unless an explicitly approved requirement and
  migration plan say otherwise.

## Windows audio and COM rules

- Use `internal/wincom`; do not reintroduce the unsafe pinned `go-wca` methods
  identified by the hardening report.
- Match native ABI exactly: pointers and handles use native width; `HRESULT`
  fails only when its signed value is negative; Windows `BOOL` storage is
  32-bit, not Go `bool`; output integers use their declared Win32 widths; and
  callbacks must match every vtable signature on both 386 and amd64.
- Make COM ownership explicit. Release each acquired interface exactly once,
  unregister callbacks before releasing their source, keep callback objects
  alive while registered, and respect COM apartment/thread lifetime.
- Routing mutation APIs are undocumented and version-sensitive. Feature-detect
  failures, save restorable state, and verify every policy write by reading back
  the actual default roles or per-application preference. A successful
  `HRESULT`, mixer entry, or session listing is not proof of physical routing.

## Verification

Use the repeatable Windows validation entry point from the repository root:

```powershell
.\pkg\mixr\scripts\windows\validate.ps1 -Mode quick # Windows amd64 tests
.\pkg\mixr\scripts\windows\validate.ps1 -Mode full  # Established 386/amd64 suite
.\pkg\mixr\scripts\windows\validate.ps1 -Mode race  # amd64, requires GCC/CGO
```

See [validation workflow](docs/VALIDATION_WORKFLOW.md) for artifact retention,
result summaries, safe command previews, and deferred physical checks, and
[Windows race setup](docs/WINDOWS_RACE_TESTING.md) for compiler prerequisites.
Use `-Plan` during an active hardware soak; defer actual tests/builds until it
ends. Use `-KeepArtifacts` to retain application/diagnostic executables with
version metadata. Automated validation never launches the application or tools.

Follow [docs/WINDOWS_AUDIO_HARDENING.md](docs/WINDOWS_AUDIO_HARDENING.md) for
known vet diagnostics. Changes affecting USB, serial input, audio sessions,
endpoint selection, routing, mute, callbacks, or hardware behavior also require
physical Windows/Nano validation. Record the hardware, Windows/build version,
architecture, procedure, observed result, and any untested cases.

## Operational diagnostics

- Keep routine polling and valid ADC frames quiet. Repeated unavailable-device
  or target warnings must be rate-limited and should log a recovery transition.
- Serial/tray status delivery must be buffered or non-blocking so diagnostics
  and UI work cannot stall the control path.
- Do not log raw serial payloads by default. For malformed input, prefer bounded
  counts, sizes, and expected framing unless verbose troubleshooting explicitly
  requires content.
- Preserve `logs/mixr-latest-run.log` and the five-run release-log retention
  convention. Crash logs are separate evidence and must not be pruned by normal
  run-log retention.
- Use [the C2 manual checklist](docs/RELIABILITY_DIAGNOSTICS.md) for changes to
  startup, reconnect, tray, configuration validation, or log behavior.

Prefer modular, testable Go with narrow interfaces, minimal dependencies, and
incremental changes. Clearly label verified results, assumptions, and planned
behavior in code review and documentation.
