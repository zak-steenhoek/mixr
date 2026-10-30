# Nano R4 dual hardware mute prototype firmware

This experimental sketch preserves four-pot legacy frames while adding two
independent, host-authoritative mute controls. It remains separate from the
known-good reference firmware and is not the final bidirectional protocol.

## Wiring

| Function | Nano R4 pin | Connection |
|---|---|---|
| Existing potentiometers | `A0`–`A3` | Unchanged |
| Reserved microphone slider | `A4` | Reserved; not read by this sketch |
| Spotify latching switch | `D2` | Switch to GND; firmware uses `INPUT_PULLUP` |
| Spotify red indicator | `D3` | Pin → 1 kΩ resistor → LED anode; LED cathode → GND |
| Default-endpoint latching switch | `D4` | Switch to GND; firmware uses `INPUT_PULLUP` |
| Default-endpoint red indicator | `D5` | Pin → 1 kΩ resistor → LED anode; LED cathode → GND |
| USB serial | USB | 9600 baud |

D3 and D5 are outputs controlled only by state messages from the host. They do
not directly follow the physical switch positions.

## Arduino IDE

1. Open `mixr-hardware-mute-prototype.ino`.
2. Select the installed Nano R4 board package/profile and the board's COM port.
3. Verify/compile the sketch.
4. Upload it, then close Serial Monitor so the mixr host can open the port.

## Arduino CLI

Discover the installed board identifier instead of guessing it:

```powershell
arduino-cli board list
$fqbn = '<Nano R4 FQBN reported above>'
$port = 'COM4'
arduino-cli compile --fqbn $fqbn .\arduino\mixr-hardware-mute-prototype
arduino-cli upload --fqbn $fqbn -p $port .\arduino\mixr-hardware-mute-prototype
```

On every `mixr:v1:hello`, the firmware disables events, samples both switches
without emitting an event, resets their independent debounce state, then sends
one `mixr:v1:ready`. Each switch is armed only after it remains stable for the
debounce interval. Subsequent D2 and D4 transitions are debounced independently.
Both LEDs remain off until the host reports observed Windows state.
