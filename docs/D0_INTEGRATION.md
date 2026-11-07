# Track D0 integration and mainline qualification

## Authoritative baseline

`main` is the authoritative development branch for the complete D0 stack.
Integration started from fetched local/remote main
`94fba24a732a76f0f27e0b10ce2f2468626e3ae0` on 2026-11-06. The isolated
`integration/d0-final` worktree started at that same commit and merged
`track-d/callback-regression-guard` at
`80a9f0aff60d5bc63e1c0ab545ed5e0308f379a8` using `--no-ff`, producing
`fed3408341c63339ad6ae0f668bfc001a379cb99`. There were no conflicts.
Mainline's context-efficiency guidance was preserved.

The full history is retained, including five-channel momentary firmware/host
integration, serialized refresh and dedicated Windows COM ownership,
foreground callback reuse and process-disappearance handling, opt-in slider
mapping for buttons/LEDs, and the permanent callback regression guard.
Intermediate branches were not separately merged or cherry-picked.
Historical branches, worktrees, local configurations, and evidence remain.

The annotated `milestone/five-channel-d0-validated` tag identifies the final
main commit. Its annotation scopes physical evidence to `5b955d1` and records
the foreground/race deferrals below; it is not a claim that every final-main
qualification check has passed. Final executable identity, full main hash,
build metadata, and mainline automated results are preserved with the separate
release bundle in `releases/d0-mainline-amd64-<main-short-hash>/`.

## Integration review

- Mainline C1/C2 ancestry and identity/diagnostic behavior are retained. The
  established build scripts, serial transport, normal log retention, reference
  deej firmware, and audited `internal/wincom` bindings are unchanged by D0.
- One package-level non-capturing callback serves foreground enumeration;
  lookup/cache/context access is mutex-protected. Missing parent/child
  processes fail safely. Both structural/pointer regression guards and the
  existing 10,000-lookup test remain present. See the
  [callback guard's guarantees and limits](D0_CALLBACK_REGRESSION_GUARD.md).
- Retained Windows audio operations, discovery, release, and COM apartment
  lifetime use the dedicated locked OS-thread owner. The session-map lock
  excludes refresh/release while a command uses sessions. Automated refresh
  requests share one cooldown and coalesce; missing-target indicator polling
  does not force discovery. Manual refresh retains its explicit bypass.
- Firmware contract tests cover A0-A3 legacy ADC frames, separate A4 microphone
  events, D2/D4/D6/D8/D10 buttons, D3/D5/D7/D9/D11 LEDs, 9600 baud, and the
  additive `mixr:v1` protocol. Earlier latching and legacy one-way firmware
  remain supported. Firmware was not changed or uploaded during integration.
- `follow_slider_mapping: true` resolves A0-A3 button and LED targets through
  the same live `slider_mapping` as analog controls, including groups and
  special targets. A4 keeps the default recording-device path. Mapping reloads
  retarget actions/readback together; hardware mode changes require restart.
  Old configs remain opt-out by default. The embedded example now explicitly
  documents the existing false default; no production Go behavior was changed
  during consolidation.
- Diff review found no tracked executable/log/generated artifacts, accidental
  reference-firmware changes, or missing D0 files. No integration-specific
  release blocker was found. Existing COM timeout/lifetime edge cases remain
  outside this consolidation's scope, as recorded by the original investigation.

## Physical evidence and limits

The user reported the following passing against the D0 release candidate at
`5b955d1630028f2740d7ffd4608f1c9f22df2d70`:

| Exercise | User-reported result |
| --- | --- |
| Five-channel volume and momentary mute | Passed |
| LED synchronization | Passed |
| Microphone controls | Passed |
| Playback and recording endpoint disconnect/reconnect | Passed |
| Nano USB disconnect/reconnect and recovery | Passed |
| Long-duration real-world soak/stability | Passed |

The fourth analog channel used Discord rather than `deej.current`, so dynamic
foreground application switching has **not** received its planned physical
test. `80a9f0a` subsequently added tests/documentation only, with no production
changes. These are the supplied observations, not independently repeated
hardware results. Exact soak duration, tested architecture, Windows/build,
board-core/firmware hashes, and resource measurements were not supplied with
the integration request; no additional physical parity or individual checklist
item is inferred. The previous deliverable contains a COM5 configuration, but
that alone does not establish the exact physical test environment.

## Automated verification

The full established Windows matrix ran successfully on the integration tree
for **386 and amd64**: full `go test -count=1 -buildvcs=false ./...`, development
and release application scripts, routing-spike/audio-control/microphone-control
CLI builds, and explicit foreground/callback tests. The full suite executes
the callback guard and unchanged 10,000-lookup regression; it does not merely
compile them. Read-only opt-in live foreground results are recorded below.

| Integration live foreground check | Result |
| --- | --- |
| Windows 386, 2,500 actual uncached lookups | Passed, 51.78 seconds |
| Windows amd64, 2,500 actual uncached lookups | Passed, 41.89 seconds |

These tests required normal desktop/process visibility and were run with
`MIXR_TEST_LIVE_FOREGROUND=1`. They are read-only resolver checks, not physical
foreground volume/mute/LED qualification. The complete matrix is repeated on
the actual merged main before tagging/pushing; those final captures and their
commit identity are retained in the mainline release manifest.

`go vet ./...` is not clean: its existing unsafe-pointer diagnostics occur in
`internal/wincom` ABI tests, the routing spike, and Windows session-finder
callback ABI code. `go vet -unsafeptr=false ./...` leaves only the existing
`pkg/mixr/util/util.go:43` unbuffered `signal.Notify` diagnostic. Neither target
introduced a new vet warning. The known Go module stat-cache access denial
was non-fatal to builds. Captures are retained locally in the integration
worktree's `releases/d0-integration-validation/`.

Race testing has **not run** because the suitable Windows C compiler is
missing. No compiler installation or system environment change was made here.
The [callback guard report](D0_CALLBACK_REGRESSION_GUARD.md) records the failed
CGO build attempt, precise MinGW-w64 prerequisite, proposed installation, and
repeatable optional amd64 race command. This deferral is not a race-test pass
or a detected race failure. Normal 386/amd64 checks remain unchanged.

## Mainline release and next qualification

The separate ignored release bundle contains the final-main amd64 executable,
its `config.yaml`, `Launch-mixr.ps1`, and a build/hash/validation manifest. Run
the launcher from that bundle; it supplies the correct working directory and
captures fatal stderr/process evidence separately from normal run-log pruning.
No hardware-affecting application is launched by this integration.

The bundled configuration is copied from the delivered amd64 `5b955d1` config:
COM5, 9600 baud, A0 master, A1 Spotify, A2 Chrome, A3 Discord, separate A4
microphone, five-channel momentary mode and `follow_slider_mapping: true`,
500 ms polling, microphone inversion false, deadband 12. The user can change
the bundle's COM port to the board's actual port. These are local runtime
settings, not new portable defaults; the pre-existing COM4 fallback/example is
retained for compatibility. Older release executables and runtime logs are
preserved. The microphone deadband adjustment and log-retention expansion
remain separate deferred follow-ups.

Next user-operated qualification on the final-main executable must include:

1. A longer mainline soak with all analog/mute channels and resource observation.
2. Set `slider_mapping.3: deej.current` in this new bundle's config. Exercise
   two audio apps, an app without a session, app exit/recreation, and bounded
   recovery; check A3 volume, D8 mute, and D9 readback target the active app.
3. Change mute externally and verify all five LEDs follow actual Windows state.
4. Repeat playback/recording endpoint and USB recovery, including held-button
   reconnect behavior; run the C2 tray/startup/config/log checklist as applicable.
5. Optionally approve compiler setup and run the amd64 race suite. Record the
   actual result and investigate any reports.

Record final commit/executable hash, architecture, Windows version/build,
Nano/core/firmware identity, COM port, endpoints, durations, observations, and
untested cases. These final-main physical checks are pending, not passed.
