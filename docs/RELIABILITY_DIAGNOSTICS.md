# Reliability, diagnostics, and tray controls

## Status and scope

Track C2 is a stabilization change based on validated main commit
`d13d396e099255e01dfe81c73bbd8e2ffae9e921`. It changes host diagnostics,
recovery, release-log retention, and tray controls. It does not change Arduino
firmware, pins, serial message semantics, ADC scaling, Windows volume/mute
semantics, or the two-second authoritative indicator resend.

Automated checks are software evidence only. The branch is not physically
validated until the checklist below is completed on the Nano R4 setup.

## Audit findings

The following behavior is intentionally preserved:

- `config.yaml`, its existing keys and defaults, and `deej.current` /
  `deej.unmapped` compatibility aliases;
- configured 9600-baud serial operation and four-field legacy potentiometer
  frames;
- every existing `mixr:v1:` hello, ready, switch, analog, and indicator line;
- switch baselining, duplicate switch suppression, independent LED state, the
  two-second indicator resend, and A4's 12-count default deadband; and
- default-endpoint and application-session volume/mute behavior through the
  audited Windows COM layer.

Observed reliability and diagnostic gaps were:

- failure of the first serial open left mixr running but never retried;
- the tray gave no indication of connected, reconnecting, or disconnected
  state;
- malformed/truncated serial input was silently ignored, while a legacy frame
  checked the `0..1023` range incompletely;
- a missing prototype `ready` response retried forever without a useful
  diagnostic;
- invalid semantic configuration values were silently normalized or accepted,
  and invalid mapping keys could collapse onto slider zero;
- unavailable mute/microphone targets could emit the same warning on every
  poll;
- release logging appended to one file indefinitely across runs and the tray
  offered no direct log access; and
- startup logs did not summarize active controls, ADC assumptions, mapped
  targets, or endpoint availability.

Low-risk changes in this track address those gaps. Larger changes are deferred:

- automatic COM-port discovery or device identity matching;
- a transactional configuration snapshot and complete support for editor
  rename/replace reload patterns;
- retaining the previous session snapshot when a refresh fails;
- asynchronous Core Audio session creation/mute notifications;
- a hard byte-size cap within one unusually long run; and
- a graphical settings or diagnostics window.

## Configuration validation and startup diagnostics

Existing configuration remains valid without new keys. On load, mixr reports
the absolute configuration path, serial port/baud rate, mapping summary, enabled
hardware controls, A4 inversion/deadband, polling interval, and the fixed
`0..1023` ADC assumption. After audio discovery it reports default playback and
recording availability and whether each configured static target currently has
matching sessions. Dynamic `deej.*` targets are identified separately.

Invalid values produce a warning with the corrective fallback: empty serial
port, non-positive baud, unsupported noise reduction, invalid slider indexes,
empty or duplicate mapping targets, an empty hardware-mute target, a poll below
100 ms, or a microphone deadband outside `0..128`. Fallbacks preserve prior
defaults: `COM4`, 9600, `default`, `spotify.exe`, 500 ms, and 12 counts.

Hardware-mute/microphone settings still require restart because the experimental
controller is constructed once; this track does not broaden hot reload.

## Serial recovery and diagnostics

The configured port remains explicit; mixr does not scan or select ports. A
failed initial open now enters the same one-second background reconnect loop as
an established disconnect. Repeated open failures are warning-rate-limited to
once per 30 seconds. Connection generation resets handshake and indicator
caches, so reconnect still requires a fresh `ready` baseline before switches
can toggle and indicators synchronize.

When the hardware-mute prototype is enabled, `hello` remains additive and is
sent every 750 ms until `ready`. After five seconds without `ready`, mixr warns
and continues supporting legacy slider frames; there is deliberately no
compatibility-breaking handshake deadline.

Malformed, truncated, unknown, or out-of-range lines do not emit control
events. Their warning is limited to once per ten seconds and records count and
byte length rather than raw serial content. Every legacy field is now checked
against `0..1023`; valid framing and delivery are unchanged.

Unavailable poll-driven mute and microphone targets warn immediately and then
at most once per 30 seconds. A recovery message is logged when the target is
observable again. Normal ADC frames and successful polls remain quiet unless
verbose logging is enabled.

## Tray and logs

The Windows tray includes a disabled live connection-status item reflected in
the tooltip, `Open log folder`, the existing configuration editor and session
rescan, a disabled `About mixr` build-identity item, and Quit. Status delivery
is buffered and non-blocking, so tray work cannot stall serial I/O.

Release builds continue writing `logs/mixr-latest-run.log`. At the next release
start, a non-empty current log becomes a timestamped `mixr-run-*.log`; the five
newest archived run logs are retained. Crash logs are excluded. Development
builds continue logging to stderr. One continuously running process can still
grow its current log without a byte cap.

## Manual physical regression checklist

### Current status (2026-11-02)

The original prototype breadboard used for the validated three-mute and
microphone-control firmware has been disassembled. A replacement five-channel
board has passed a standalone electrical diagnostic: A0–A4 correspond to their
intended analog controls, momentary buttons are connected on D2/D4/D6/D8/D10,
and LEDs are connected on D3/D5/D7/D9/D11.

That result validates the replacement board's tested wiring only. The current
production firmware does not support the five-channel momentary-button layout,
so the diagnostic does not validate C2 serial recovery, switch baselining,
Windows mute behavior, indicator synchronization, tray behavior, or combined
software/hardware operation. The complete C2 physical regression below has not
been performed and must not be inferred from the electrical diagnostic. This is
an explicit validation limitation, not a C2 software test failure.

### Outstanding C2 checklist

Record commit, executable architecture, Windows build, Nano R4/Arduino core,
COM port, baud, audio endpoints, application versions, procedure, observed
result, and every untested case.

- [ ] Launch amd64 mixr with the validated `config.yaml`; confirm startup logs
  identify all mappings, COM4/9600, prototype controls, A4 deadband, and both
  endpoint availability states.
- [ ] Confirm tray status, tooltip, About, Edit configuration, Open log folder,
  Re-scan, and Quit all work without a freeze.
- [ ] Exercise A0–A3 and confirm master, Spotify, Chrome, and `deej.current`
  retain validated isolation.
- [ ] Exercise A4 and D2/D4/D6; confirm no startup/duplicate toggle and that
  D3/D5/D7 reflect actual Windows state.
- [ ] Disconnect/reconnect USB; confirm Reconnecting then Connected, fresh
  switch baselining, A4 reapplication, and indicator synchronization.
- [ ] Start mixr with the Nano disconnected and confirm automatic recovery when
  it appears.
- [ ] Restart mixr while the Nano remains connected and confirm synchronization
  without an unintended toggle.
- [ ] Close/reopen Chrome and Spotify and change the default playback endpoint;
  confirm existing recovery.
- [ ] Disable/restore recording; confirm D7 fails off, D6/A4 affect no unrelated
  target, and control recovers without restart.
- [ ] Repeat a smoke test with the 386 build.
- [ ] Run combined controls for at least ten minutes and inspect the log for
  loops, warning floods, malformed frames, stale indicators, stalls, or crashes.

GUI interaction, USB timing, audible isolation, and endpoint removal cannot be
proven by automated tests and remain manual requirements.

## Automated validation

On 2026-10-30, both `GOARCH=386` and `GOARCH=amd64` passed:

- `go test -count=1 -buildvcs=false ./...`;
- the Windows development and release build scripts; and
- builds of the routing-spike, audio-control, and microphone-control
  diagnostics.

The same full test and build matrix passed again on 2026-11-02 before C2
integration. This repeat is automated software/build evidence and does not
change the physical-validation limitation recorded above.

The validated repository `config.yaml` is loaded by a focused compatibility
test. Additional tests cover semantic fallbacks/warnings, malformed and
truncated serial messages, pre-ready ordering, duplicate `ready`, initial-open
retry, established reconnect state reset, warning limiting/recovery, and
five-run log retention. Existing tests continue covering switch baselining,
duplicate switch suppression, indicator resend, A4 deadband/reapplication,
missing audio targets, default-endpoint refresh, and protocol identity.

`go vet -unsafeptr=false ./...` on both architectures reported only the
documented pre-existing unbuffered `signal.Notify` warning in
`pkg/mixr/util/util.go`. Race instrumentation was unavailable because this
Windows Go environment has CGO disabled. No automated GUI or physical hardware
test was claimed.
