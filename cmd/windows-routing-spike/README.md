# Windows playback routing feasibility spike

This standalone command is intentionally separate from deej. It does not use or
modify production audio-session logic, firmware, configuration, or serial code.

Build from the repository root:

```powershell
$env:GOARCH = '386'
& 'C:\Program Files\Go\bin\go.exe' build -o windows-routing-spike.exe .\cmd\windows-routing-spike
```

The 32-bit target is currently required because the old `go-wca` version pinned
by deej truncates the pointer returned by `IMMDevice::GetId` in 64-bit builds.
This is an experiment dependency constraint, not a Windows Core Audio limit.

Run `windows-routing-spike.exe` without arguments for commands. Read
`docs/WINDOWS_ROUTING_SPIKE.md` before invoking policy-changing commands.
