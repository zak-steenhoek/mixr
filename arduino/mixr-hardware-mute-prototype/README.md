# Nano R4 hardware mute prototype firmware

This experimental sketch preserves four-pot legacy frames while adding the D2
latching-switch and D3 red-LED proof of concept. It is separate from the
known-good reference firmware and is not the final bidirectional protocol.

## Wiring

- Existing potentiometers: `A0`, `A1`, `A2`, `A3`
- Latching switch: `D2` to GND (`INPUT_PULLUP`)
- Red LED: `D3` through 1 kΩ to the LED, then GND
- USB serial: 9600 baud

## Arduino IDE

1. Open `mixr-hardware-mute-prototype.ino`.
2. Select the installed Nano R4 board package/profile and the board's COM port.
3. Verify/compile the sketch.
4. Upload it, then close Serial Monitor so the mixr host can open the port.

## Arduino CLI

Discover the exact installed board identifier rather than guessing it:

```powershell
arduino-cli board list
$fqbn = '<Nano R4 FQBN reported above>'
$port = 'COM4'
arduino-cli compile --fqbn $fqbn .\arduino\mixr-hardware-mute-prototype
arduino-cli upload --fqbn $fqbn -p $port .\arduino\mixr-hardware-mute-prototype
```

The firmware ignores D2 until the host sends `mixr:v1:hello`, baselines the
current switch state without an event, then returns `mixr:v1:ready`. D3 is off
until the host supplies an observed Windows mute state.
