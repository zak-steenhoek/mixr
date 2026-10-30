# Windows microphone-control diagnostic

This serial-independent diagnostic talks directly to the current Windows
`EConsole` default capture endpoint through documented `IMMDeviceEnumerator`
and `IAudioEndpointVolume` interfaces. It does not enumerate or change playback
sessions, use routing policy APIs, or connect to Arduino hardware.

Build from the repository root:

```powershell
go build -o windows-microphone-control.exe .\cmd\windows-microphone-control
```

Read the endpoint ID, normalized input level, and mute state:

```powershell
.\windows-microphone-control.exe current
```

Make deliberate changes; each mutation prints Windows readback:

```powershell
.\windows-microphone-control.exe set-volume --value 0.50
.\windows-microphone-control.exe mute
.\windows-microphone-control.exe unmute
.\windows-microphone-control.exe toggle-mute
```

Watch external changes and default recording-endpoint switches. The command
reacquires the current default endpoint on every poll and prints only changes:

```powershell
.\windows-microphone-control.exe watch --interval 500ms
```

Press Ctrl+C to stop. If no default recording endpoint exists, one-shot commands
return an error and watch reports `unavailable` until an endpoint returns.
