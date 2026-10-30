package deej

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHardwareMutePrototypeIgnoresInitialAndDuplicateSwitchStates(t *testing.T) {
	commander := &fakePrototypeCommander{muteState: AggregateMuteAllUnmuted, sessionCount: 2}
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)

	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	if commander.toggleCount != 0 {
		t.Fatalf("pre-ready toggle count = %d, want 0", commander.toggleCount)
	}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: false,
	})

	if commander.toggleCount != 2 {
		t.Fatalf("toggle count = %d, want one for each stable transition", commander.toggleCount)
	}
	if commander.muteState != AggregateMuteAllUnmuted {
		t.Fatalf("final mute state = %q, want unmuted", commander.muteState)
	}
}

func TestHardwareMutePrototypeMixedStateAndExternalChange(t *testing.T) {
	commander := &fakePrototypeCommander{muteState: AggregateMuteMixed, sessionCount: 2}
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if got := indicator.last(); got {
		t.Fatal("mixed-state indicator = on, want off")
	}

	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	if commander.muteState != AggregateMuteAllMuted || !indicator.last() {
		t.Fatalf("mixed toggle state = %q, indicator = %t", commander.muteState, indicator.last())
	}

	commander.muteState = AggregateMuteAllUnmuted // external Windows change
	prototype.syncIndicator(false)
	if indicator.last() {
		t.Fatal("external unmute indicator = on, want off")
	}
}

func TestHardwareMutePrototypeRefreshesRecreatedSpotifySessions(t *testing.T) {
	commander := &fakePrototypeCommander{unavailable: true, muteState: AggregateMuteAllMuted, sessionCount: 2}
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)
	refreshes := 0
	prototype.refresh = func() {
		refreshes++
		commander.unavailable = false
	}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if refreshes != 1 {
		t.Fatalf("refresh count = %d, want 1", refreshes)
	}
	if !indicator.last() {
		t.Fatal("indicator after session recreation = off, want on")
	}
}

func testHardwareMutePrototype(commander AudioCommander, indicator prototypeIndicatorWriter) *hardwareMutePrototype {
	return &hardwareMutePrototype{
		logger:    zap.NewNop().Sugar(),
		commander: commander,
		indicator: indicator,
		target:    "spotify.exe",
		poll:      500 * time.Millisecond,
	}
}

type fakePrototypeCommander struct {
	muteState    AggregateMuteState
	sessionCount int
	toggleCount  int
	unavailable  bool
}

func (c *fakePrototypeCommander) ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error) {
	if c.unavailable {
		return AudioCommandResult{Target: command.Target}, errors.New("Spotify sessions unavailable")
	}
	if command.Operation == AudioCommandToggleMute {
		c.toggleCount++
		if c.muteState == AggregateMuteAllMuted {
			c.muteState = AggregateMuteAllUnmuted
		} else {
			c.muteState = AggregateMuteAllMuted
		}
	}
	result := AudioCommandResult{Target: command.Target, MuteState: c.muteState}
	for index := 0; index < c.sessionCount; index++ {
		result.Mutes = append(result.Mutes, AudioMuteState{
			SessionKey: "spotify.exe",
			Muted:      c.muteState == AggregateMuteAllMuted,
		})
	}
	return result, nil
}

type fakePrototypeIndicator struct {
	writes []bool
}

func (i *fakePrototypeIndicator) setPrototypeIndicator(_ string, on bool) error {
	i.writes = append(i.writes, on)
	return nil
}

func (i *fakePrototypeIndicator) last() bool {
	if len(i.writes) == 0 {
		return false
	}
	return i.writes[len(i.writes)-1]
}
