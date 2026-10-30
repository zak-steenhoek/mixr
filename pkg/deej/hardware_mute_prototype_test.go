package deej

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHardwareMutePrototypeControlsAreIndependentAndDeduplicated(t *testing.T) {
	commander := newFakePrototypeCommander()
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)

	for _, controlID := range []string{prototypeSpotifyMuteControl, prototypeMasterMuteControl} {
		prototype.handleEvent(PrototypeSerialEvent{
			Kind: PrototypeSerialSwitchEvent, ControlID: controlID, SwitchClosed: true,
		})
	}
	if commander.totalToggles() != 0 {
		t.Fatalf("pre-ready toggle count = %d, want 0", commander.totalToggles())
	}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeMasterMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeMasterMuteControl, SwitchClosed: true,
	})
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: false,
	})

	if got := commander.toggleCount["spotify.exe"]; got != 2 {
		t.Fatalf("Spotify toggle count = %d, want 2", got)
	}
	if got := commander.toggleCount[masterSessionName]; got != 1 {
		t.Fatalf("master toggle count = %d, want 1", got)
	}
	if commander.states["spotify.exe"] != AggregateMuteAllUnmuted {
		t.Fatalf("Spotify final state = %q, want unmuted", commander.states["spotify.exe"])
	}
	if commander.states[masterSessionName] != AggregateMuteAllMuted {
		t.Fatalf("master final state = %q, want muted", commander.states[masterSessionName])
	}
}

func TestHardwareMutePrototypeSynchronizesIndicatorsIndependently(t *testing.T) {
	commander := newFakePrototypeCommander()
	commander.states["spotify.exe"] = AggregateMuteMixed
	commander.states[masterSessionName] = AggregateMuteAllMuted
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if indicator.last(prototypeSpotifyMuteControl) {
		t.Fatal("mixed Spotify indicator = on, want off")
	}
	if !indicator.last(prototypeMasterMuteControl) {
		t.Fatal("muted master indicator = off, want on")
	}
	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeSpotifyMuteControl, SwitchClosed: true,
	})
	if commander.states["spotify.exe"] != AggregateMuteAllMuted || !indicator.last(prototypeSpotifyMuteControl) {
		t.Fatalf("mixed multi-session toggle state = %q, indicator = %t, want all-muted/on",
			commander.states["spotify.exe"], indicator.last(prototypeSpotifyMuteControl))
	}

	commander.states["spotify.exe"] = AggregateMuteAllMuted
	commander.states[masterSessionName] = AggregateMuteAllUnmuted
	prototype.syncIndicators(false)
	if !indicator.last(prototypeSpotifyMuteControl) {
		t.Fatal("externally muted Spotify indicator = off, want on")
	}
	if indicator.last(prototypeMasterMuteControl) {
		t.Fatal("externally unmuted master indicator = on, want off")
	}
}

func TestHardwareMutePrototypeRefreshesSpotifyAndChangedDefaultEndpoint(t *testing.T) {
	commander := newFakePrototypeCommander()
	commander.unavailable["spotify.exe"] = true
	commander.unavailable[masterSessionName] = true
	commander.states["spotify.exe"] = AggregateMuteAllMuted
	commander.states[masterSessionName] = AggregateMuteAllMuted
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)
	refreshes := 0
	prototype.refresh = func() {
		refreshes++
		if refreshes == 1 {
			commander.unavailable["spotify.exe"] = false
		}
		if refreshes == 2 {
			// Simulate rediscovery of a newly selected default endpoint.
			commander.unavailable[masterSessionName] = false
		}
	}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if refreshes != 2 {
		t.Fatalf("refresh count = %d, want one independent refresh per unavailable target", refreshes)
	}
	if !indicator.last(prototypeSpotifyMuteControl) || !indicator.last(prototypeMasterMuteControl) {
		t.Fatalf("indicators after rediscovery: Spotify=%t master=%t, want both on",
			indicator.last(prototypeSpotifyMuteControl), indicator.last(prototypeMasterMuteControl))
	}
}

func TestHardwareMutePrototypeMissingEndpointFailsSafe(t *testing.T) {
	commander := newFakePrototypeCommander()
	commander.unavailable[masterSessionName] = true
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)
	prototype.refresh = func() {}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if indicator.last(prototypeMasterMuteControl) {
		t.Fatal("missing endpoint indicator = on, want safe off")
	}

	prototype.handleEvent(PrototypeSerialEvent{
		Kind: PrototypeSerialSwitchEvent, ControlID: prototypeMasterMuteControl, SwitchClosed: true,
	})
	if got := commander.toggleCount[masterSessionName]; got != 0 {
		t.Fatalf("missing endpoint toggle count = %d, want 0 successful toggles", got)
	}
	if indicator.last(prototypeMasterMuteControl) {
		t.Fatal("failed endpoint toggle left indicator on")
	}
}

func TestHardwareMutePrototypeReadyResetsBothSwitchBaselines(t *testing.T) {
	commander := newFakePrototypeCommander()
	indicator := &fakePrototypeIndicator{}
	prototype := testHardwareMutePrototype(commander, indicator)

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	for _, controlID := range []string{prototypeSpotifyMuteControl, prototypeMasterMuteControl} {
		prototype.handleEvent(PrototypeSerialEvent{
			Kind: PrototypeSerialSwitchEvent, ControlID: controlID, SwitchClosed: true,
		})
		prototype.handleEvent(PrototypeSerialEvent{
			Kind: PrototypeSerialSwitchEvent, ControlID: controlID, SwitchClosed: true,
		})
	}
	if commander.totalToggles() != 2 {
		t.Fatalf("first connection toggles = %d, want 2", commander.totalToggles())
	}

	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	for _, controlID := range []string{prototypeSpotifyMuteControl, prototypeMasterMuteControl} {
		prototype.handleEvent(PrototypeSerialEvent{
			Kind: PrototypeSerialSwitchEvent, ControlID: controlID, SwitchClosed: false,
		})
	}
	if commander.totalToggles() != 4 {
		t.Fatalf("post-reconnect transitions = %d, want one new transition per control", commander.totalToggles())
	}
}

func testHardwareMutePrototype(commander AudioCommander, indicator prototypeIndicatorWriter) *hardwareMutePrototype {
	return &hardwareMutePrototype{
		logger:    zap.NewNop().Sugar(),
		commander: commander,
		indicator: indicator,
		controls: map[string]*prototypeMuteControl{
			prototypeSpotifyMuteControl: {target: "spotify.exe"},
			prototypeMasterMuteControl:  {target: masterSessionName},
		},
		poll: 500 * time.Millisecond,
	}
}

type fakePrototypeCommander struct {
	states       map[string]AggregateMuteState
	sessionCount map[string]int
	toggleCount  map[string]int
	unavailable  map[string]bool
}

func newFakePrototypeCommander() *fakePrototypeCommander {
	return &fakePrototypeCommander{
		states: map[string]AggregateMuteState{
			"spotify.exe":     AggregateMuteAllUnmuted,
			masterSessionName: AggregateMuteAllUnmuted,
		},
		sessionCount: map[string]int{
			"spotify.exe":     2,
			masterSessionName: 1,
		},
		toggleCount: make(map[string]int),
		unavailable: make(map[string]bool),
	}
}

func (c *fakePrototypeCommander) ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error) {
	if c.unavailable[command.Target] {
		return AudioCommandResult{Target: command.Target}, errors.New("audio target unavailable")
	}
	state, exists := c.states[command.Target]
	if !exists {
		return AudioCommandResult{Target: command.Target}, errors.New("unknown audio target")
	}
	if command.Operation == AudioCommandToggleMute {
		c.toggleCount[command.Target]++
		if state == AggregateMuteAllMuted {
			state = AggregateMuteAllUnmuted
		} else {
			state = AggregateMuteAllMuted
		}
		c.states[command.Target] = state
	}
	result := AudioCommandResult{Target: command.Target, MuteState: state}
	for index := 0; index < c.sessionCount[command.Target]; index++ {
		muted := state == AggregateMuteAllMuted
		if state == AggregateMuteMixed {
			muted = index%2 == 0
		}
		result.Mutes = append(result.Mutes, AudioMuteState{SessionKey: command.Target, Muted: muted})
	}
	return result, nil
}

func (c *fakePrototypeCommander) totalToggles() int {
	total := 0
	for _, count := range c.toggleCount {
		total += count
	}
	return total
}

type fakePrototypeIndicator struct {
	writes map[string][]bool
}

func (i *fakePrototypeIndicator) setPrototypeIndicator(controlID string, on bool) error {
	if i.writes == nil {
		i.writes = make(map[string][]bool)
	}
	i.writes[controlID] = append(i.writes[controlID], on)
	return nil
}

func (i *fakePrototypeIndicator) last(controlID string) bool {
	states := i.writes[controlID]
	if len(states) == 0 {
		return false
	}
	return states[len(states)-1]
}
