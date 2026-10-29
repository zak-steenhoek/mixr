# Windows audio-routing feasibility spike

## Scope and safety

The experiment is isolated in `cmd/windows-routing-spike`. It does not change
deej's audio-session logic, Arduino firmware, serial protocol, or baseline tag.
Policy-changing commands print warnings. Record the output of `default` and
`get-app` before changing anything so the original state can be restored.

## API classification

| Capability | Mechanism | Support status |
|---|---|---|
| Enumerate render endpoints, state, friendly name, endpoint ID | `IMMDeviceEnumerator`, `IMMDevice`, `IPropertyStore` | Documented public Core Audio API |
| Read defaults by role | `IMMDeviceEnumerator::GetDefaultAudioEndpoint` | Documented public Core Audio API |
| Observe endpoint/default changes | `IMMNotificationClient` (not needed by this one-shot CLI) | Documented public Core Audio API |
| Change system defaults | `IPolicyConfigVista::SetDefaultEndpoint` | Undocumented COM interface; version-sensitive and unsupported as an application contract |
| Read/set persisted per-app defaults | `Windows.Media.Internal.AudioPolicyConfig` / `IAudioPolicyConfigFactory` | Undocumented WinRT interface; version-sensitive and unsupported as an application contract |

Primary Microsoft references:

- [MMDevice API](https://learn.microsoft.com/windows/win32/coreaudio/mmdevice-api)
- [`IMMDevice::GetId`](https://learn.microsoft.com/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-immdevice-getid)
- [Default endpoint selection](https://learn.microsoft.com/windows-hardware/drivers/audio/default-audio-endpoint-selection)
- [Stream routing](https://learn.microsoft.com/windows/win32/coreaudio/stream-routing)
- [`PKEY_AudioEndpoint_StableId`](https://learn.microsoft.com/windows/win32/coreaudio/pkey-audioendpoint-stableid)

`IMMDevice::GetId` IDs are opaque and reusable on the same installation, but
Microsoft does not promise durability through OS or driver updates. Windows 11
24H2 adds `PKEY_AudioEndpoint_StableId`, which is more durable but still not
absolute and is unavailable on older supported Windows versions. This spike
prints the broadly available endpoint ID and treats it as installation-local.

## Expected behavior

- Windows persists a preferred endpoint by application identity. The experiment
  accepts a PID because the internal policy service resolves identity from a
  currently running process.
- A new or recreated stream should select the persisted preferred endpoint when
  the app uses the system default-selection path.
- An existing stream may remain on its original endpoint. Immediate movement is
  application/API dependent; changing policy does not itself migrate a WASAPI
  stream that was opened directly on a particular endpoint.
- If the preferred endpoint is unavailable, Windows should select an active
  fallback. Reattachment behavior must be tested; it is not guaranteed here.

## Test plan and result record

Automated checks that can run without changing policy:

```powershell
.\windows-routing-spike.exe list
.\windows-routing-spike.exe default
```

Manual two-endpoint test (speakers plus headphones):

1. Save `list` and `default` output. Identify the speakers and headphones IDs.
2. Start Spotify and a second audio app; make both play continuously. In Task
   Manager's Details view, record Spotify's actual audio-process PID.
3. Run `get-app <spotify-pid>`, then `set-app <spotify-pid> <speakers-id>`.
4. Keep the other app on the Windows default headphones. Verify by listening or
   muting one physical endpoint at a time; a mixer/session listing alone is not
   proof of physical routing.
5. Record whether Spotify moves immediately. If not, pause/resume, change track,
   and restart Spotify, recording which action creates a stream on speakers.
6. Run `get-app` again after Spotify restart (using its new PID). Verify that the
   saved application preference is still returned and used.
7. Unplug speakers during playback. Record whether audio stops or falls back to
   headphones. Reconnect speakers, then pause/resume and restart Spotify; record
   when the preferred route returns.
8. Reboot Windows. Repeat `list`, start both apps, run `get-app` with the new
   Spotify PID, and verify physical routing. This is required before claiming
   reboot persistence.
9. Restore Spotify to system default with `set-app <pid> default`. Avoid
   `clear-apps` unless intentionally removing every per-app preference for the
   current user.

System-default mutation test (optional): save all three role values, run
`set-default <id>`, verify with `default` and Windows Settings, then restore the
original endpoint ID. The command changes all three render roles.

## Actual results (2026-10-28)

- The 32-bit source compiled with Go 1.27.3. An initial 64-bit run crashed in
  the pinned 2019 `go-wca` dependency because its `IMMDevice::GetId` wrapper
  stores an allocated pointer in `uint32`. The CLI is therefore constrained to
  `GOARCH=386`, matching the validated deej build. This is a dependency defect,
  not a Windows API limitation.
- `list` successfully enumerated 30 render endpoints in active, disabled,
  not-present, and unplugged states. It reported two active Realtek endpoints.
- `default` successfully read separate role assignments: Speakers (Realtek) for
  console/multimedia and Realtek HD Audio 2nd output for communications.
- `get-app` reached the internal policy factory without crashing. A read against
  the host PowerShell process returned `E_INVALIDARG` for all roles, which is
  expected evidence that an arbitrary non-audio PID is not a valid functional
  test. A running Spotify audio-process PID is still required.
- Physical separation, live stream migration, unplug/replug, application restart,
  and reboot persistence have **not** been claimed. They require the explicit
  manual procedure above with two known physical endpoints.

## Feasibility conclusion and risks

Public APIs are sufficient for discovery, stable-enough installation-local IDs,
reading defaults, and notifications. They are not sufficient for a supported
implementation that changes the user's system or per-app default endpoint.

The experimental policy calls can establish practical feasibility on tested
Windows builds, but shipping them creates compatibility risk because Microsoft
does not document their ABI or availability. A production design must feature-
detect failures, preserve user settings, clearly label affected Windows builds,
and retain a fallback that opens Windows Volume Mixer for user-managed routing.
It must not promise immediate migration: the target application controls stream
creation and may bind directly to a device.
