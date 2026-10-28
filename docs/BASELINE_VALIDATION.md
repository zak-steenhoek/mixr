# Known-Good deej Baseline Validation

**Validation date:** 2026-10-28  
**Validated commit:** `9c0307b96341538ec46f18d2e9b18aaa84441175`  
**Baseline tag:** `baseline/deej-working`

## Configuration under test

- Windows development build
- Arduino Nano R4 (Renesas RA4M1)
- Four B10K rotary potentiometers
- Existing deej-compatible firmware and serial protocol
- USB serial at 9600 baud
- Four independent mappings, including Windows master volume, Chrome, and Spotify

## Build result

The original application built successfully from source with the repository's
Windows development build script. The checked-in `rsrc_windows.syso` resource
is a 32-bit x86 COFF object, so the baseline was built with `GOARCH=386`:

```powershell
cmd /c "set GOARCH=386&& call pkg\deej\scripts\windows\build-dev.bat"
```

The build produced `deej-dev.exe`. The original executable name is intentionally
retained for this validation; renaming it to mixr is future implementation work.

## Hardware and behavior result

The following behavior was manually verified:

- The Nano R4 connected using the existing serial protocol.
- All four potentiometers were detected and operational.
- The master control changed the Windows default playback endpoint volume.
- Chrome volume control worked independently.
- Spotify volume control worked independently.
- The fourth configured application channel worked independently.
- Audio-session refresh and application reopening behavior worked.
- The repository remained clean after building and testing.

## Baseline rule

This commit and tag are the known-good compatibility baseline. Future mixr
development must preserve support for the existing potentiometer firmware until
an explicitly planned compatibility change is approved and validated.
