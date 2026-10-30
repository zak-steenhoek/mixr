package mixr

import (
	"fmt"
	"strconv"
	"strings"
)

// This versioned namespace is deliberately additive to the legacy numeric
// slider frames. It is an experiment, not the final bidirectional protocol.
const (
	prototypeProtocolPrefix         = "mixr:v1:"
	prototypeHelloMessage           = prototypeProtocolPrefix + "hello"
	prototypeReadyMessage           = prototypeProtocolPrefix + "ready"
	prototypeSpotifyMuteControl     = "spotify-mute"
	prototypeMasterMuteControl      = "master-mute"
	prototypeMicrophoneMuteControl  = "microphone-mute"
	prototypeMicrophoneLevelControl = "microphone-level"
)

type PrototypeSerialEventKind string

const (
	PrototypeSerialReady       PrototypeSerialEventKind = "ready"
	PrototypeSerialSwitchEvent PrototypeSerialEventKind = "switch"
	PrototypeSerialAnalogEvent PrototypeSerialEventKind = "analog"
)

// PrototypeSerialEvent is emitted only for recognized v1 firmware messages.
type PrototypeSerialEvent struct {
	Kind         PrototypeSerialEventKind
	ControlID    string
	SwitchClosed bool
	AnalogValue  int
}

func parsePrototypeSerialLine(line string) (PrototypeSerialEvent, bool) {
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == prototypeReadyMessage {
		return PrototypeSerialEvent{Kind: PrototypeSerialReady}, true
	}

	const analogPrefix = prototypeProtocolPrefix + "event:analog:" + prototypeMicrophoneLevelControl + ":"
	if strings.HasPrefix(line, analogPrefix) {
		value, err := strconv.Atoi(strings.TrimPrefix(line, analogPrefix))
		if err != nil || value < 0 || value > 1023 {
			return PrototypeSerialEvent{}, false
		}
		return PrototypeSerialEvent{Kind: PrototypeSerialAnalogEvent, ControlID: prototypeMicrophoneLevelControl, AnalogValue: value}, true
	}

	const switchPrefix = prototypeProtocolPrefix + "event:switch:"
	if !strings.HasPrefix(line, switchPrefix) {
		return PrototypeSerialEvent{}, false
	}
	parts := strings.Split(strings.TrimPrefix(line, switchPrefix), ":")
	if len(parts) != 2 || parts[0] == "" {
		return PrototypeSerialEvent{}, false
	}
	switch parts[1] {
	case "0":
		return PrototypeSerialEvent{Kind: PrototypeSerialSwitchEvent, ControlID: parts[0], SwitchClosed: false}, true
	case "1":
		return PrototypeSerialEvent{Kind: PrototypeSerialSwitchEvent, ControlID: parts[0], SwitchClosed: true}, true
	default:
		return PrototypeSerialEvent{}, false
	}
}

func prototypeIndicatorMessage(controlID string, on bool) string {
	value := 0
	if on {
		value = 1
	}
	return fmt.Sprintf("%sstate:indicator:%s:%d", prototypeProtocolPrefix, controlID, value)
}
