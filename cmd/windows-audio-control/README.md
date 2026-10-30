# Windows audio command test interface

This small CLI exercises the production relative-volume and mute command model
against live Windows Core Audio. It does not read or change Arduino firmware,
the legacy serial protocol, configuration mappings, or routing policy.

Build from the repository root:

```powershell
go build -o windows-audio-control.exe .\cmd\windows-audio-control
```

Read state without changing it:

```powershell
.\windows-audio-control.exe current-volume --target master
.\windows-audio-control.exe current-mute --target chrome.exe
.\windows-audio-control.exe current-mute --target deej.current
```

Exercise mutations deliberately:

```powershell
.\windows-audio-control.exe relative-volume --target master --steps 1 --increment 0.02
.\windows-audio-control.exe mute --target chrome.exe
.\windows-audio-control.exe unmute --target chrome.exe
.\windows-audio-control.exe toggle-mute --target chrome.exe
```

Application targets match every Core Audio session with the same executable
name. Per-session state is printed separately. A mixed mute target toggles to
all muted; a target that is already all muted toggles to all unmuted.
