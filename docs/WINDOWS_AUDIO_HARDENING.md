# Windows audio dependency hardening

## Scope and baseline

This track starts at `c5b1aa7` (`baseline/v1-start`) and is limited to Windows
Core Audio interoperability and architecture-compatible application resources.
It does not change routing policy, the serial protocol, firmware, encoders,
mute behavior, or display behavior.

## Root-cause analysis

| Defect | Impact | Correction |
|---|---|---|
| The pinned `go-wca` `IMMDevice::GetId` wrapper stores the returned `LPWSTR` in `uint32`. | An amd64 pointer is truncated before conversion and `CoTaskMemFree`, causing invalid memory access. | `internal/wincom.DeviceID` receives `LPWSTR` through `*uint16`, checks the HRESULT and null result, converts the string, and frees it with `CoTaskMemFree`. |
| The pinned `IMMDevice::Activate` wrapper passes `&ctx`, discards `pActivationParams`, and treats every nonzero HRESULT as failure. | COM receives a pointer where a `DWORD` is required; activation can fail or behave unpredictably on both architectures. | `internal/wincom.Activate` passes `DWORD` by value, forwards the optional `PROPVARIANT`, uses a native output pointer, and rejects only failed HRESULTs. |
| `IAudioSessionEnumerator::GetCount` is bound to Go `*int`, although the COM method writes a 32-bit `int`. | The exported signature changes width on amd64 and does not describe the native ABI. | `internal/wincom.AudioSessionCount` receives into `int32` and converts after validation. |
| One zero-argument callback was installed into every `IMMNotificationClient` vtable slot. | The callback argument count is incorrect, especially for 32-bit stdcall stack cleanup, and `QueryInterface` did not return an interface pointer. | Each callback now has its native argument shape. The by-value `PROPERTYKEY` callback has separate 386 and amd64 implementations. `QueryInterface` supports `IUnknown` and `IMMNotificationClient`. |
| The pinned dependency returns `E_NOTIMPL` for endpoint-notification unregister. | The enumerator could be released while Windows retained the callback pointer. | The compatibility layer invokes the real vtable slot before releasing the enumerator. |
| The device collection and several error-path session interfaces were not released; one constructor split error ownership between caller and callee. | Refreshes and failures could leak references or make ownership ambiguous. | The collection is released, all queried interfaces have one clear owner, and failed session construction releases each interface exactly once. |
| The checked-in `rsrc_windows.syso` has COFF machine `0x014c` (x86). | The amd64 application link fails with an unsupported relocation from the x86 object. | Resources are generated as `rsrc_windows_386.syso` (`0x014c`) and `rsrc_windows_amd64.syso` (`0x8664`), so Go selects the matching object. |

The audit also found unsafe four-byte Windows `BOOL` outputs in unused methods
of the pinned dependency, including `GetMute`. Current mixr does not call those
methods. Any future mute work must use a corrected binding that receives into
`int32`; it must not call the pinned methods directly.

## Dependency strategy

The implemented strategy keeps the known-good pinned dependency and places the
small number of unsafe calls used by mixr behind an audited compatibility
package. This minimizes behavioral change for V1 while removing every defective
binding from current production and diagnostic call paths.

Alternatives considered:

- `go-wca v0.3.0` widens several returned pointers, but its tagged
  `IMMDevice::Activate` implementation is still incorrect and its notification
  API changes would broaden this track.
- `github.com/degubites/go-wca v0.4.1` fixes several `BOOL` overwrite and
  vtable-order defects, but its tagged `Activate` implementation is still
  incorrect and adopting a new fork adds provenance and migration risk.
- Vendoring all of `go-wca` would permit direct fixes but add a large local fork
  for a handful of used methods.
- Replacing the Core Audio binding wholesale would be disproportionate for V1.

## Automated validation

Validation was run on Windows with Go 1.27.3 on 2026-10-29.

| Check | 386 | amd64 |
|---|---:|---:|
| `go test ./...` | Pass | Pass |
| mixr application build | Pass | Pass |
| routing-spike build | Pass | Pass |
| fake-vtable pointer, activation, session-count, and volume ABI tests | Pass | Pass |
| routing spike `list` | Pass: 30 endpoints | Pass: 30 endpoints |
| routing spike `default` | Pass | Pass |
| routing spike `sessions` | Pass: 7 sessions | Pass: 7 sessions |

The live read-only routing checks were run outside the tool sandbox against the
host Windows audio service. Inside the sandbox, both architectures received the
same `E_ACCESSDENIED` while activating a session manager; outside it, both passed.
No policy-changing spike command was run.

The volume tests exercise the native get/set argument ABI against fake COM
vtables and do not change host volume. The successful live checks prove native
endpoint enumeration, ID conversion, session-manager activation, and session
discovery on both architectures. They do not prove physical audio response or
Nano R4 behavior.

After physical regression, the full automated matrix was repeated on both
targets. `go test -count=1 -buildvcs=false ./...`, the mixr application build,
and the routing-spike build all passed for 386 and amd64. Fresh read-only
`list`, `default`, and `sessions` runs also exited successfully on both targets.

`go vet ./...` is not clean on either target. Its `unsafe.Pointer` diagnostics
are confined to the audited raw COM/Win32 boundary and fake-vtable tests. They
are expected for native vtable interoperation and did not identify another
incorrect width or call signature. Running `go vet -unsafeptr=false ./...`
leaves one diagnostic on both targets: the pre-existing unbuffered
`signal.Notify` channel in `pkg/deej/util/util.go`. That warning represents a
real correctness risk because a process signal can be missed when no receiver
is ready. It is unrelated to Windows audio ABI hardening and remains an explicit
follow-up rather than being folded into this track.

## Nano R4 physical regression checklist

Use the existing four-potentiometer configuration and test the amd64 build
first. Repeat on 386 if continued x86 distribution is required. Record the app
build, Windows build, endpoint names, and result for each item.

- [x] Start mixr with the Nano R4 connected and confirm all four input values are detected.
- [x] Move the Windows master pot through low, middle, and high positions; confirm the default playback endpoint follows without affecting configured per-app isolation.
- [x] Play audio in Chrome and verify only the Chrome pot changes Chrome's session volume.
- [x] Play audio in Spotify and verify only the Spotify pot changes Spotify's session volume.
- [x] Use `deej.current` for the fourth pot and verify it follows foreground Hulu and Discord sessions.
- [x] Close and reopen Chrome, Spotify, and Discord; confirm mixr discovers each recreated session without restarting mixr.
- [x] Run the routing spike's `list`, `default`, and `sessions` commands; confirm endpoint IDs, default roles, and live session PIDs are present on amd64.
- [x] Change the default playback endpoint through Windows Settings, then confirm mixr refreshes master/session handles and continues responding.
- [x] Confirm the routing spike's read-only behavior and four-pot control on both architectures. No `set-default`, `set-app`, or `clear-apps` command was invoked.
- [x] Run all four channels for at least ten minutes and confirm no crash, stalled callback, or unhealthy resource growth.

## Physical regression results

Physical regression was completed on 2026-10-29 with the existing Arduino Nano
R4, four rotary potentiometers, `COM4` at 9600 baud, and the native amd64 build.
A 386 controller and diagnostic parity smoke test was also completed.

| Test | Confirmed result |
|---|---|
| Native amd64 startup | Passed. The tray icon appeared, all four pots produced debug activity, and startup remained nominal. |
| Windows master volume | Passed. The first pot followed the default endpoint while individual application sliders remained independent. |
| Chrome volume | Passed. The second pot controlled Chrome independently. |
| Spotify volume | Passed. The third pot controlled audible Spotify playback. Two mixer entries moved together because mixr groups sessions by `spotify.exe`. |
| Foreground fourth channel | Passed with `deej.current`. Hulu and Discord followed the fourth pot when foreground. The installed Hulu application is an Edge/PWA session and appears as `msedge.exe` in Volume Mixer. |
| Spotify recreation | Passed immediately after a full exit and reopen; no mixr restart or refresh delay was observed. The post-reopen read-only snapshot showed one active Spotify Core Audio session PID and multiple live Spotify helper processes. |
| Chrome recreation | Passed after fully terminating Chrome's retained background processes and reopening it. |
| Discord recreation | Passed by observing the recreated Discord mixer slider respond through `deej.current`; audible confirmation was unavailable because the voice call ended. |
| Default-device change | Passed. Console/multimedia moved from Speakers (`{b161a7f4-3a85-4399-bbee-9eaf5d947a2d}`) to Realtek HD Audio 2nd output (`{3a66bf09-fed7-4d59-8132-4b0ade73df37}`), then restored. The master pot followed both changes without restarting mixr. |
| amd64 read-only routing diagnostics | Passed. `list`, `default`, and `sessions` completed and showed the expected active endpoints and sessions. |
| Ten-minute stability run | Passed. All controls remained responsive with no observed crash, serial disconnect, stalled callback, or unhealthy process behavior. |
| 386 parity smoke | Passed. Four-pot control plus read-only endpoint and session diagnostics completed without a pointer-related failure. |

After a PC sleep, Windows Volume Mixer initially displayed two Spotify entries,
including one with stale song-title metadata. Both entries followed the Spotify
pot. A full Spotify exit and reopen cleared the duplicate mixer record; the new
audio session was controlled immediately. The stale title alone did not prove
that an exited PID was still active.

## Remaining risks and integration recommendation

- Discord recreation was confirmed through the mixer but not audibly because
  the voice call ended before that check.
- Windows can retain stale mixer-session metadata after sleep. Volume control
  safely applies to all matching `spotify.exe` sessions, but future routing work
  must resolve live Core Audio session PIDs and tolerate retained records rather
  than trusting mixer display names.
- The process signal channel remains unbuffered, as reported by `go vet`; a
  separate scoped fix should give the `signal.Notify` channel capacity for at
  least one signal.
- Unused areas of the old dependency remain unaudited for future feature work;
  new Windows audio calls must go through corrected bindings.
- The pre-existing design retains COM interfaces across repeated discovery
  cycles. It passed current runtime checks, but a future lifecycle-focused track
  should keep COM apartment ownership and interface lifetime on one OS thread.
- Routing mutations still use undocumented Windows interfaces and retain the
  compatibility risks documented in `WINDOWS_ROUTING_SPIKE.md`; this track does
  not alter them.

The Nano R4 checklist and x86 parity smoke test passed. The branch is suitable
for focused integration review, but it must not be merged into `main` or tagged
until integration is explicitly approved. No additional audio or serial feature
should be bundled with that integration.
