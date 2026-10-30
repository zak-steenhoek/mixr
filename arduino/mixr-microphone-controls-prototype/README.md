# Nano R4 microphone-controls prototype firmware

This separate experimental sketch preserves the validated dual-mute firmware
and the legacy four-pot frame while adding D6/D7 microphone mute and A4
microphone level.

| Function | Pins |
|---|---|
| Default playback mute | D2 switch to GND / D3 LED through 1 kΩ to GND |
| Spotify mute | D4 switch to GND / D5 LED through 1 kΩ to GND |
| Default recording mute | D6 switch to GND / D7 LED through 1 kΩ to GND |
| Existing volume controls | A0–A3 unchanged |
| Microphone input level | A4 potentiometer wiper |

All switches use `INPUT_PULLUP`. Integrated LEDs are host-authoritative and do
not follow switch position directly.

The sketch continues emitting exactly four pipe-delimited legacy values. A4 is
additive and namespaced:

```text
mixr:v1:event:analog:microphone-level:512
```

A4 is sampled at 5 Hz and sent after a four-count change or once per second as
a recovery heartbeat. The host applies the configured larger deadband and
endpoint readback. On every `hello`, all switches are re-baselined without a
toggle and the next A4 sample intentionally synchronizes the absolute Windows
microphone level.

Build/upload using the same Nano R4 board profile and 9600-baud COM port as the
diagnostic sketch. Arduino CLI was not available in the automated environment,
so local IDE verification is required before upload.

Enable the existing experimental host feature and build from this branch:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  poll_interval_ms: 500
  microphone_level_inverted: false
  microphone_level_deadband: 12
```

```powershell
$env:GOARCH = 'amd64'
cmd /c "call pkg\deej\scripts\windows\build-dev.bat"
Remove-Item Env:GOARCH
.\mixr-dev.exe --verbose
```

Close Arduino Serial Monitor before starting mixr.

## Physical validation checklist

1. Start with all switches stable. Confirm handshake/ready, three indicator
   synchronizations, one initial A4 level synchronization, and no mute toggle.
2. Confirm D2/D3 controls only the current default playback endpoint, D4/D5
   controls only all Spotify sessions, and D6/D7 controls only the default
   recording endpoint. Exercise both switch directions repeatedly and confirm
   one toggle per stable transition.
3. Record microphone audio while moving A4 through low, middle, and high.
   Confirm recording level follows, remains clamped to `0..1`, and does not
   alter playback or application volume. Verify whether inversion is desired.
4. At A4 minimum, toggle D6/D7 and confirm the observed 0→8 ADC coupling does
   not cause an audible or logged microphone-level write.
5. Mute/unmute the microphone externally in Windows and confirm D7 follows
   without a hardware event. Repeat external playback and Spotify changes for
   D3/D5 regression.
6. Change the default playback endpoint and confirm D2/D3 retargets. If another
   recording endpoint becomes available later, change the default input and
   confirm D6/D7 and A4 retarget; otherwise record this case as equipment-
   limited and untested.
7. Disable/disconnect the microphone. Confirm D7 fails safe off and D6 causes no
   crash or unrelated mute. Restore it without restarting mixr; confirm D7
   recovers and the next A4 heartbeat reapplies the absolute slider position.
8. Disconnect/reconnect USB with all switches stable. Confirm no accidental
   mute, all indicators resynchronize, and A4 intentionally reapplies its
   absolute position once after `ready`.
9. Exercise A0–A3 across their ranges and confirm the original master, Chrome,
   Spotify, and foreground mappings remain independent.
10. Run all controls together for at least ten minutes. Confirm no repeated
    toggle, stale LED, disconnect loop, stalled pot, crash, or excessive serial
    traffic. Record Windows/build version, architecture, board/core/tool
    versions, COM port, endpoint IDs, procedure, results, and untested cases.
