package deej

import (
	"fmt"
	"strings"
)

// This versioned namespace is deliberately additive to the legacy numeric
// slider frames. It is an experiment, not the final bidirectional protocol.
const (
	prototypeProtocolPrefix     = "mixr:v1:"
	prototypeHelloMessage       = prototypeProtocolPrefix + "hello"
	prototypeReadyMessage       = prototypeProtocolPrefix + "ready"
	prototypeSpotifyMuteControl = "spotify-mute"
)

type PrototypeSerialEventKind string

const (
	PrototypeSerialReady       PrototypeSerialEventKind = "ready"
	PrototypeSerialSwitchEvent PrototypeSerialEventKind = "switch"
)

// PrototypeSerialEvent is emitted only for recognized v1 firmware messages.
type PrototypeSerialEvent struct {
	Kind         PrototypeSerialEventKind
	ControlID    string
	SwitchClosed bool
}

func parsePrototypeSerialLine(line string) (PrototypeSerialEvent, bool) {
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == prototypeReadyMessage {
		return PrototypeSerialEvent{Kind: PrototypeSerialReady}, true
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
