package deej

import "testing"

func TestParsePrototypeSerialLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		want  PrototypeSerialEvent
		valid bool
	}{
		{name: "ready crlf", line: "mixr:v1:ready\r\n", want: PrototypeSerialEvent{Kind: PrototypeSerialReady}, valid: true},
		{name: "switch open", line: "mixr:v1:event:switch:spotify-mute:0\r\n", want: PrototypeSerialEvent{Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl}, valid: true},
		{name: "switch closed", line: "mixr:v1:event:switch:spotify-mute:1\n", want: PrototypeSerialEvent{Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true}, valid: true},
		{name: "master switch closed", line: "mixr:v1:event:switch:master-mute:1\n", want: PrototypeSerialEvent{Kind: PrototypeSerialSwitchEvent, ControlID: prototypeMasterMuteControl, SwitchClosed: true}, valid: true},
		{name: "legacy analog", line: "10|20|30|40\r\n", valid: false},
		{name: "future version", line: "mixr:v2:ready\r\n", valid: false},
		{name: "invalid state", line: "mixr:v1:event:switch:spotify-mute:true\r\n", valid: false},
		{name: "unknown message", line: "mixr:v1:event:encoder:one:1\r\n", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := parsePrototypeSerialLine(test.line)
			if valid != test.valid {
				t.Fatalf("valid = %t, want %t", valid, test.valid)
			}
			if valid && got != test.want {
				t.Fatalf("event = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestPrototypeIndicatorMessage(t *testing.T) {
	if got := prototypeIndicatorMessage(prototypeSpotifyMuteControl, true); got != "mixr:v1:state:indicator:spotify-mute:1" {
		t.Fatalf("on message = %q", got)
	}
	if got := prototypeIndicatorMessage(prototypeSpotifyMuteControl, false); got != "mixr:v1:state:indicator:spotify-mute:0" {
		t.Fatalf("off message = %q", got)
	}
	if got := prototypeIndicatorMessage(prototypeMasterMuteControl, true); got != "mixr:v1:state:indicator:master-mute:1" {
		t.Fatalf("master on message = %q", got)
	}
}
