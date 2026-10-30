# Windows playback routing feasibility spike

This standalone command is intentionally separate from deej. It does not use or
modify production audio-session logic, firmware, configuration, or serial code.

Build from the repository root:

```powershell
$env:GOARCH = 'amd64' # or '386'
& 'C:\Program Files\Go\bin\go.exe' build -o windows-routing-spike.exe .\cmd\windows-routing-spike
```

The command uses mixr's native-width COM compatibility layer, so both 32-bit
and 64-bit builds can enumerate endpoint IDs and audio sessions safely.

Run `windows-routing-spike.exe` without arguments for commands. Read
`docs/WINDOWS_ROUTING_SPIKE.md` before invoking policy-changing commands.

`sessions` is read-only and lists the Core Audio session PID and endpoint for
each active render endpoint. On Windows 25H2, `set-app` accepts the shorter
endpoint ID printed by `list` and converts it to the render interface path used
by the internal policy service.
