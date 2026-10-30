# mixr identity migration

## Scope

Track C1 changes the repository, Go package, build paths, runtime artifacts,
and Windows application resources from the inherited deej identity to mixr.
It does not change Windows audio behavior, USB framing, hardware mappings,
microphone behavior, mute behavior, or firmware.

The migration started from validated commit
`70040ea0cb90142e1cd2e4a7a728b1d4d1a3578b` on the isolated
`refactor/mixr-identity` branch.

## Rename inventory

Current application identity:

- Go module: `github.com/zak-steenhoek/mixr`;
- application package and entrypoint: `pkg/mixr` and `pkg/mixr/cmd`;
- primary type and constructor: `Mixr` and `NewMixr`;
- development/release artifacts: `mixr-dev.exe`, `mixr-release.exe`, and
  packaged `mixr.exe`;
- runtime artifacts: `logs/mixr-latest-run.log`, `mixr-crash-*.log`, and the
  materialized toast icon `%TEMP%/mixr.ico`;
- current no-tray environment variable: `MIXR_NO_TRAY_ICON`; and
- Windows manifest assembly identity, tray title, and tooltip: `mixr`.

The previous `pkg/deej` tree moved as one unit to `pkg/mixr`, including its
scripts, application assets, entrypoint, and tests. Repository-local imports
now use the actual mixr origin path rather than the upstream deej module path.

## Compatibility retained intentionally

The following old names are functional compatibility contracts and remain:

- `deej.current` and `deej.unmapped` in YAML mappings;
- the complete existing YAML key set and `config.yaml` location;
- `DEEJ_NO_TRAY_ICON` as a legacy alias for `MIXR_NO_TRAY_ICON`;
- the original newline-delimited, pipe-separated `0..1023` frames at the
  configured baud rate (currently 9600);
- every `mixr:v1:` hello, ready, switch, analog, and indicator message;
- all control identifiers, mute mappings, microphone mappings, and indicator
  behavior; and
- `arduino/deej-5-sliders-vanilla` as the upstream-compatible reference
  firmware and historical name.

The upstream MIT license and Omri Harel copyright remain unchanged. Historical
validation reports, baseline/milestone names, the upstream FAQ, and community
showcase retain deej wording when it identifies what was actually tested or
attributes upstream material.

## Reproducible Windows branding

The editable source is `pkg/mixr/assets/mixr-icon.svg`. It uses a 64-unit grid
and simple monochrome rectangles so the three faders align cleanly at 16x16 and
32x32. `tools/icon-generator` uses only the Go standard library to render 16,
20, 24, 32, 40, 48, 64, 128, and 256-pixel PNG-backed ICO frames and generate
the Go byte slices used by the tray and notifications.

From the repository root, regenerate every branding output with:

```powershell
cmd /c "call pkg\mixr\scripts\windows\generate-branding.bat"
```

That command regenerates:

- `pkg/mixr/assets/mixr.ico`;
- `pkg/mixr/icon/icon.go`;
- `pkg/mixr/icon/menu_icons.go`;
- `pkg/mixr/cmd/rsrc_windows_386.syso`; and
- `pkg/mixr/cmd/rsrc_windows_amd64.syso`.

The existing `make-rsrc.bat` name remains as a compatibility wrapper. Two
consecutive complete regenerations produced identical SHA-256 hashes for every
generated output. The resource pipeline embeds the icon and mixr assembly
manifest; it does not currently emit a Windows `VERSIONINFO` block. Build
identity continues to be injected by the existing linker flags and displayed
in the tray menu.

## Automated validation

Validation ran on Windows on 2026-10-30 with Go 1.27.3. The following matrix
passed for both `GOARCH=386` and `GOARCH=amd64`:

| Check | 386 | amd64 |
|---|---:|---:|
| `go test -count=1 -buildvcs=false ./...` | Pass | Pass |
| development tray application build | Pass | Pass |
| release tray application build | Pass | Pass |
| `cmd/windows-routing-spike` build | Pass | Pass |
| `cmd/windows-audio-control` build | Pass | Pass |
| `cmd/windows-microphone-control` build | Pass | Pass |
| executable loader/flag smoke (`mixr-dev.exe -h`) | Pass | Pass |
| embedded release icon extraction | Pass | Pass |

The extracted icon from both release executables matched the new three-fader
mark. Compatibility tests lock the YAML keys, 9600-baud default,
`deej.current`, `deej.unmapped`, no-tray alias, current validated config values,
and `mixr:v1:` identity.

Git object comparison against the starting commit confirmed that the tracked
`config.yaml`, vanilla Arduino sketch, microphone-controls prototype sketch,
and microphone-controls diagnostic sketch are byte-for-byte unchanged. The
protocol implementation diff contains only the Go package declaration rename.

CI keeps its existing push/pull-request event and branch triggers. Only package
paths and the established Go test/build execution were updated; packaging and
deployment were not redesigned.

These automated checks are not a physical regression pass.

## Manual Windows and Nano R4 regression

Record the Windows version/build, mixr commit, executable architecture, Nano R4
board/firmware, COM port, default playback/recording endpoints, procedure,
observed result, and any untested item.

- [ ] Build and start the amd64 development executable beside the existing
  validated `config.yaml` with the Nano R4 connected.
- [ ] Confirm the dedicated monochrome icon appears in Explorer and the system
  tray, the tooltip reads `mixr`, the menu opens, and the displayed build
  version is present.
- [ ] Confirm all four legacy potentiometer values are detected at 9600 baud;
  verify master, Spotify, Chrome, and `deej.current` independently through low,
  middle, and high positions.
- [ ] Confirm the master channel follows only the current default playback
  endpoint and still follows after changing the default endpoint.
- [ ] Fully exit and reopen Chrome, Spotify, and the foreground test app;
  confirm their recreated sessions work without restarting mixr.
- [ ] Exercise master, Spotify, and microphone mute controls. Confirm each
  indicator agrees with the software-authoritative mute state and external
  Windows changes are reflected.
- [ ] Move the microphone level control through its range and confirm the
  validated direction, deadband behavior, and default recording endpoint are
  unchanged.
- [ ] Edit a harmless mapping while mixr is running, confirm the existing YAML
  hot-reload behavior, then restore the validated mapping.
- [ ] Exercise `Edit configuration`, `Re-scan audio sessions`, and `Quit` from
  the tray menu.
- [ ] Confirm a release run writes `logs/mixr-latest-run.log`; existing
  `deej-*.log` files may remain as historical files but no new ones should be
  created.
- [ ] Repeat an application-start, tray, four-pot, and mute smoke test with the
  386 executable.
- [ ] If a Windows Startup-folder shortcut exists, confirm it targets
  `mixr.exe` and launches normally after sign-in.

Do not merge or tag this branch until the physical checklist is recorded.
