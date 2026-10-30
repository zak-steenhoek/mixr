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

`go vet ./...` is not clean on either target. It reports the pre-existing
unbuffered `signal.Notify` channel in `pkg/deej/util/util.go` plus
`unsafe.Pointer` diagnostics in raw COM/Win32 code and the fake-vtable tests.
Those diagnostics were recorded but not changed because the signal handling is
outside this track and native vtable calls necessarily cross `unsafe.Pointer`.

## Nano R4 physical regression checklist

Use the existing four-potentiometer configuration and test the amd64 build
first. Repeat on 386 if continued x86 distribution is required. Record the app
build, Windows build, endpoint names, and result for each item.

- [ ] Start mixr with the Nano R4 connected and confirm all four input values are detected.
- [ ] Move the Windows master pot through low, middle, and high positions; confirm the default playback endpoint follows without affecting configured per-app isolation.
- [ ] Play audio in Chrome and verify only the Chrome pot changes Chrome's session volume.
- [ ] Play audio in Spotify and verify only the Spotify pot changes Spotify's session volume.
- [ ] Play audio in the configured fourth application and verify only the fourth pot changes that session.
- [ ] Close and reopen Chrome, Spotify, and the fourth application one at a time; confirm mixr discovers each recreated session without restarting mixr.
- [ ] Run `windows-routing-spike.exe list`, `default`, and `sessions`; confirm endpoint IDs, default roles, and live session PIDs are present on amd64.
- [ ] Change the default playback endpoint through Windows Settings, then confirm mixr refreshes master/session handles and continues responding.
- [ ] Confirm the routing spike's previously validated read-only behavior on both architectures. Do not invoke `set-default`, `set-app`, or `clear-apps` during this regression pass.
- [ ] Let all four channels run for at least ten minutes and confirm no crash, stalled callback, or steadily increasing handle count.

## Remaining risks and integration recommendation

- Physical controller and audible volume behavior still require the checklist above.
- Unused areas of the old dependency remain unaudited for future feature work;
  new Windows audio calls must go through corrected bindings.
- The pre-existing design retains COM interfaces across repeated discovery
  cycles. It passed current runtime checks, but a future lifecycle-focused track
  should keep COM apartment ownership and interface lifetime on one OS thread.
- Routing mutations still use undocumented Windows interfaces and retain the
  compatibility risks documented in `WINDOWS_ROUTING_SPIKE.md`; this track does
  not alter them.

Do not merge this branch into `main` or create a new baseline tag until the Nano
R4 checklist passes and integration is explicitly approved. If the physical
regression passes, the changes are suitable for a focused review and merge; no
additional audio or serial feature should be bundled with that integration.
