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
| Change system defaults | `IPolicyConfig::SetDefaultEndpoint` | Undocumented COM interface; version-sensitive and unsupported as an application contract |
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
- On the tested Windows 25H2 build, per-app policy stores the render device
  interface path (`\\?\SWD#MMDEVAPI#...`) rather than the shorter endpoint ID
  returned by `IMMDevice::GetId`. The CLI converts the latter before setting.
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

## Physical validation environment

Validation was completed on 2026-10-29 using Windows 25H2 build `26200.9711`:

| Purpose | Windows endpoint | Endpoint ID |
|---|---|---|
| Sony WH-XB910N headphones through Realtek headphone output | `Realtek HD Audio 2nd output (Realtek(R) Audio)` | `{0.0.0.00000000}.{3a66bf09-fed7-4d59-8132-4b0ade73df37}` |
| UE Boom 2 through Realtek speaker output | `Speakers (Realtek(R) Audio)` | `{0.0.0.00000000}.{b161a7f4-3a85-4399-bbee-9eaf5d947a2d}` |

Chrome was the control application on the Windows default headphones. Spotify
was the routed application on the speakers. The existing Nano R4 and four-pot
deej setup remained running during live routing and application-restart tests.

## Actual results (2026-10-29)

- The 32-bit source compiled with Go 1.27.3. An initial 64-bit run crashed in
  the pinned 2019 `go-wca` dependency because its `IMMDevice::GetId` wrapper
  stores an allocated pointer in `uint32`. The CLI is therefore constrained to
  `GOARCH=386`, matching the validated deej build. This is a dependency defect,
  not a Windows API limitation.
- `list` successfully enumerated 30 render endpoints in active, disabled,
  not-present, and unplugged states. It reported two active Realtek endpoints.
- `default` successfully read separate role assignments: Speakers (Realtek) for
  console/multimedia and Realtek HD Audio 2nd output for communications.
- The Vista-era PolicyConfig class returned success without changing the default
  endpoint on this Windows build. Switching to the current `IPolicyConfig` class
  and adding read-back verification changed all three default roles correctly.
- Chrome and Spotify both moved from speakers to the headphone default
  immediately while already playing; no pause, restart, or stream recreation was
  needed.
- The read-only `sessions` command identified Spotify's actual Core Audio session
  PID and showed that Windows can retain stale session records on both endpoints
  after a move. Physical listening, rather than session presence alone, was used
  to determine the active route.
- Initial per-app writes failed with `E_INVALIDARG` because the internal API
  expects the full render interface path. Windows Volume Mixer supplied the
  comparator value, after which endpoint-ID conversion and all-role read-back
  verification were added to the CLI.
- `set-app` then routed already-playing Spotify to the UE Boom speakers
  immediately while Chrome continued through the Sony headphones. No glitch,
  silence, duplication, pause, or restart was observed.
- `set-app <pid> default` cleared all three Spotify preferences and moved the
  active stream back to the headphone default immediately. Reapplying the route
  also moved it immediately.
- deej remained running through the live routing changes. The Spotify pot
  controlled Spotify on speakers; the Chrome pot controlled Chrome on
  headphones; and the master pot controlled only the default headphone endpoint.
  Setting master to zero silenced Chrome without changing Spotify.
- After fully exiting and reopening Spotify, Windows restored the speakers route
  under the new PID and the still-running deej process automatically controlled
  the recreated Spotify session.
- After a Windows reboot, before deej was started, Spotify again opened on
  speakers and Chrome on headphones. All three Spotify preferences were readable
  under its new PID. Starting deej afterward restored all pot behavior without a
  manual session refresh.
- Unplugging the analog UE Boom cable made Spotify silent without falling back to
  headphones; Chrome was unaffected. Windows continued reporting the onboard
  Realtek speakers endpoint as active. Reconnecting the cable restored Spotify
  immediately and deej control continued without further action.
- The original configuration was restored and verified: Spotify has no persisted
  override; console/multimedia defaults are Speakers (Realtek); communications
  default is Realtek HD Audio 2nd output.

## Validation matrix

| Requirement | Result |
|---|---|
| Enumerate endpoints and IDs | Passed |
| Read default roles | Passed |
| Change all default roles with read-back | Passed on build 26200.9711 using undocumented `IPolicyConfig` |
| Read/set/clear Spotify preferred endpoint | Passed on build 26200.9711 using undocumented `IAudioPolicyConfigFactory` |
| Spotify on speakers while Chrome stays on headphones | Physically confirmed |
| Move an already-playing application | Immediate for Spotify and Chrome in this test |
| Preserve independent deej application volume control | Passed |
| Preserve default-endpoint master isolation | Passed |
| Persist across Spotify restart/session recreation | Passed |
| Persist across Windows reboot | Passed |
| Analog cable disconnect/reconnect | Silence without fallback; immediate recovery on reconnect |
| True endpoint removal or disable | Not tested |

## Feasibility conclusion and risks

Public APIs are sufficient for discovery, stable-enough installation-local IDs,
reading defaults, and notifications. They are not sufficient for a supported
implementation that changes the user's system or per-app default endpoint.

Persistent, independent per-application playback routing is practically feasible
on the tested Windows 25H2 build. The primary success criterion was met, including
application restart, Windows reboot, session recreation, and existing deej volume
control.

Shipping still creates compatibility risk because Microsoft does not document
the mutation interfaces, their ABI, or availability. The experiment itself found
three version/dependency sensitivities: the obsolete PolicyConfig class silently
did nothing, the per-app setter required a render interface path rather than the
ordinary endpoint ID, and the pinned `go-wca` wrappers contain unsafe 64-bit and
`IMMDevice::Activate` defects.

A production design must feature-detect failures, verify every mutation by
reading it back, preserve and restore user settings, enumerate actual Core Audio
session PIDs, and retain a Windows Volume Mixer fallback. Immediate migration was
observed here but cannot be promised for every application or audio API. True
endpoint disappearance remains unresolved because unplugging an analog cable did
not remove the onboard Realtek endpoint from Windows.
