# Nano R4 microphone-controls hardware diagnostic

## Scope

This standalone Track B2 sketch validates the three switch/LED pairs and the
new A4 microphone-level slider before any Windows host changes are made. It
does not connect to mixr, control Windows audio, or implement the experimental
bidirectional protocol. The LEDs follow switch state only for this diagnostic;
the later integrated firmware must drive them from Windows audio readback.

The known-good sketches in `arduino/deej-5-sliders-vanilla/` and
`arduino/mixr-hardware-mute-prototype/` remain unchanged.

## Wiring

Disconnect USB power while changing wiring. All three switches connect their
input pin directly to GND and use the Nano's internal `INPUT_PULLUP`; do not add
an external voltage to a switch input.

| Diagnostic function | Nano R4 pin | Connection |
|---|---|---|
| Default playback mute switch | `D2` | Latching switch between D2 and GND |
| Default playback diagnostic LED | `D3` | D3 → 1 kΩ resistor → LED anode; LED cathode → GND |
| Spotify mute switch | `D4` | Latching switch between D4 and GND |
| Spotify diagnostic LED | `D5` | D5 → 1 kΩ resistor → LED anode; LED cathode → GND |
| Microphone mute switch | `D6` | Latching switch between D6 and GND |
| Microphone diagnostic LED | `D7` | D7 → 1 kΩ resistor → LED anode; LED cathode → GND |
| Microphone-level slider | `A4` | Potentiometer wiper to A4; outer legs to GND and the same supply/reference rail already used by A0–A3 |
| USB serial | USB | 9600 baud |

Keep A0–A3 wired as they are, but note that this diagnostic does not read or
report them. Reversing the two outer A4 potentiometer connections reverses the
reported slider direction and is safe with power disconnected.

## Build and upload

### Arduino IDE

1. Open `mixr-microphone-controls-diagnostic.ino` from this directory.
2. Select the installed Nano R4 board/profile and the board's COM port.
3. Click **Verify**, then **Upload**.
4. Open Serial Monitor at **9600 baud** and select a line-ending option of any
   value; the sketch does not read serial input.

### Arduino CLI

Discover the installed board identifier instead of guessing it:

```powershell
arduino-cli board list
$fqbn = '<Nano R4 FQBN reported above>'
$port = '<Nano R4 COM port reported above>'
arduino-cli compile --fqbn $fqbn .\arduino\mixr-microphone-controls-diagnostic
arduino-cli upload --fqbn $fqbn -p $port .\arduino\mixr-microphone-controls-diagnostic
arduino-cli monitor -p $port -c baudrate=9600
```

After reset, expected output begins with one ready line and one initial line per
switch. A4 then reports at most five lines per second:

```text
diag:mixr-microphone-controls:ready
diag:switch:master-mute:open
diag:switch:spotify-mute:open
diag:switch:microphone-mute:open
diag:a4:512
```

A closed switch reports `closed` and turns on only its paired LED. An open
switch reports `open` and turns that LED off. Mechanical bounce is filtered for
35 ms, so one deliberate latching transition should produce exactly one line.

## Physical validation procedure

Record the commit, Nano R4 model, board-core version, Arduino IDE/CLI version,
COM port, wiring, and every observed pass/fail result.

1. **Startup safety:** With all three switches in arbitrary stable positions,
   power/reset the Nano. Confirm one ready line, exactly three initial switch
   lines, and that D3, D5, and D7 each match only its paired switch.
2. **D2/D3:** Latch D2 closed and open at least five times. Confirm exactly one
   `master-mute` line per stable transition and that only D3 changes.
3. **D4/D5:** Repeat for D4. Confirm exactly one `spotify-mute` line per stable
   transition and that only D5 changes.
4. **D6/D7:** Repeat for D6. Confirm exactly one `microphone-mute` line per
   stable transition and that only D7 changes.
5. **Debounce:** On each switch, move it slowly through its mechanical change
   point and lightly disturb it while held. Confirm there are no repeated lines
   once the stable state is unchanged.
6. **A4 range and direction:** Move the A4 slider from end to end several times.
   Confirm values remain within `0..1023`, approach both ends of the range, move
   smoothly, and note whether physical up corresponds to increasing or
   decreasing values. Exact endpoint values need not be 0 and 1023.
7. **Independence:** Leave A4 moving while operating D2, D4, and D6 in a varied
   sequence. Confirm every switch still produces one correctly named line,
   only its paired LED changes, A4 reporting continues, and no other LED or
   switch report changes unexpectedly.
8. **Short stability check:** Exercise all controls together for at least five
   minutes. Confirm no reset, stalled output, unexplained transition, LED
   cross-talk, or excessive serial backlog.

Do not start Windows host integration after this test. Report the observations,
including any noisy A4 range, reversed direction, duplicate/missed switch line,
or LED interaction, so the Phase 2 design can use measured hardware behavior.

## User-observed Phase 1 result

On 2026-10-30, the user reported that all three switch/LED pairs worked as
expected on the first upload. A4 reached `0..1023` while microphone mute was
open and D7 was off. With microphone mute closed and its red D7 LED on, the A4
minimum rose reproducibly to approximately `8` while the maximum remained
`1023`.

This is a small but real hardware-state-dependent ADC offset, not a validated
zero. Its cause has not yet been isolated; shared ground/supply impedance, LED
current, and analog noise remain possibilities. Later host filtering and
deadband choices must tolerate the observation, but must not conceal worsening
cross-talk. Exact board/core/tool versions and stability duration were not
reported and are not inferred.
