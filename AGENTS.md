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
- Planned, not implemented: six relative-encoder channels, button mute with
  software-controlled indicators, five persistent application routes,
  bidirectional hardware feedback, and the central display.
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

Run relevant tests for both Windows architectures from the repository root:

```powershell
foreach ($arch in '386', 'amd64') {
    $env:GOARCH = $arch
    go test -count=1 -buildvcs=false ./...
    cmd /c "call pkg\mixr\scripts\windows\build-dev.bat"
    cmd /c "call pkg\mixr\scripts\windows\build-release.bat"
    go build -o "windows-routing-spike-$arch.exe" .\cmd\windows-routing-spike
    go build -o "windows-audio-control-$arch.exe" .\cmd\windows-audio-control
    go build -o "windows-microphone-control-$arch.exe" .\cmd\windows-microphone-control
}
Remove-Item Env:GOARCH
```

Follow [docs/WINDOWS_AUDIO_HARDENING.md](docs/WINDOWS_AUDIO_HARDENING.md) for
known vet diagnostics. Changes affecting USB, serial input, audio sessions,
endpoint selection, routing, mute, callbacks, or hardware behavior also require
physical Windows/Nano validation. Record the hardware, Windows/build version,
architecture, procedure, observed result, and any untested cases.

Prefer modular, testable Go with narrow interfaces, minimal dependencies, and
incremental changes. Clearly label verified results, assumptions, and planned
behavior in code review and documentation.
