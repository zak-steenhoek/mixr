# Five-channel momentary-mute prototype

This experimental Nano R4 sketch is separate from every physically validated
reference sketch. It preserves the four-field A0-A3 legacy stream and reports
A4 with the existing `microphone-level` event.

| Channel | Analog | Button (`INPUT_PULLUP`, to GND) | LED output | Control ID | Windows target |
|---|---:|---:|---:|---|---|
| 1 | A0 | D2 | D3 | `master-mute` | Default playback endpoint |
| 2 | A1 | D4 | D5 | `spotify-mute` | Configured Spotify target |
| 3 | A2 | D6 | D7 | `chrome-mute` | Configured Chrome target |
| 4 | A3 | D8 | D9 | `foreground-mute` | `deej.current` by default |
| 5 | A4 | D10 | D11 | `microphone-mute` | Default recording endpoint |

Each LED is driven only by a host `state:indicator` message. A button is
debounced independently for 35 ms and emits one `1` press plus one `0` release;
the host toggles only on the press. Holding produces no repeats.

On every `mixr:v1:hello`, the sketch stops event delivery, samples all five
buttons as a new baseline, resets the A4 recovery heartbeat, and replies with
`mixr:v1:ready`. Therefore startup and reconnect do not synthesize a press. A
button held through reconnect must first be released (the host ignores that
release) before its next press can act.

## Build and upload

In Arduino IDE, select the installed Nano R4 board/core and the board's COM
port, open `mixr-five-channel-momentary-mute.ino`, Verify, then Upload. Close
Serial Monitor before starting mixr because only one process can own the port.
The sketch uses 9600 baud.

Enable the matching host mode in `config.yaml`:

```yaml
hardware_mute_prototype:
  enabled: true
  target: spotify.exe
  five_channel_momentary: true
  chrome_target: chrome.exe
  foreground_target: deej.current
  poll_interval_ms: 500
  microphone_level_inverted: false
  microphone_level_deadband: 12
```

Leave `five_channel_momentary` false when using the earlier three-mute firmware.
