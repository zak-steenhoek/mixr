package mixr

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestFiveChannelFollowSliderMapping(t *testing.T) {
	config := &CanonicalConfig{}
	config.HardwareMutePrototype.FiveChannelMomentary = true
	config.HardwareMutePrototype.FollowSliderMapping = true
	config.HardwareMutePrototype.Target = "stale-spotify.exe"
	config.HardwareMutePrototype.ChromeTarget = "stale-chrome.exe"
	config.HardwareMutePrototype.ForegroundTarget = "stale-foreground.exe"
	prototype := newHardwareMutePrototype(&Mixr{config: config, serial: &SerialIO{}}, zap.NewNop().Sugar())
	for index, controlID := range prototype.controlOrder[:4] {
		want := []string{"deej.slider.0", "deej.slider.1", "deej.slider.2", "deej.slider.3"}[index]
		if got := prototype.controls[controlID].target; got != want {
			t.Errorf("%s target = %q, want %q", controlID, got, want)
		}
	}
	if prototype.controls[prototypeMicrophoneMuteControl].target != inputSessionName {
		t.Fatal("A4 microphone target changed")
	}
	config.HardwareMutePrototype.FiveChannelMomentary = false
	prototype = newHardwareMutePrototype(&Mixr{config: config, serial: &SerialIO{}}, zap.NewNop().Sugar())
	if prototype.controls[prototypeSpotifyMuteControl].target != "stale-spotify.exe" {
		t.Fatal("legacy firmware target changed")
	}
}

func TestChannelButtonsAndLEDsFollowReloadedConfig(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config, err := NewConfig(logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	config.userConfig.Set(configKeyFiveChannelMomentary, true)
	config.userConfig.Set(configKeyFollowSliderMapping, true)
	config.userConfig.Set(configKeySliderMapping, map[string][]string{
		"0": {"master"}, "1": {"spotify.exe"}, "2": {"chrome.exe"},
		"3": {"DISCORD.EXE", "spotify.exe", "discord.exe"},
	})
	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	mappingIdentity := config.SliderMapping
	discord := &fakeCommandSession{key: "discord.exe", muted: true}
	spotify := &fakeCommandSession{key: "spotify.exe"}
	chrome := &fakeCommandSession{key: "chrome.exe"}
	mixr := &Mixr{config: config, serial: &SerialIO{}}
	sessions := &sessionMap{mixr: mixr, logger: logger, lock: &sync.Mutex{}, lastSessionRefresh: time.Now(), m: map[string][]Session{
		"discord.exe": {discord}, "spotify.exe": {spotify}, "chrome.exe": {chrome},
		"master": {&fakeCommandSession{key: "master"}}, "mic": {&fakeCommandSession{key: "mic"}},
	}}
	mixr.sessions = sessions
	indicator := &fakePrototypeIndicator{}
	prototype := newHardwareMutePrototype(mixr, logger)
	prototype.indicator, prototype.refresh = indicator, nil
	prototype.handleEvent(PrototypeSerialEvent{Kind: PrototypeSerialReady})
	if indicator.last(prototypeForegroundMuteControl) {
		t.Fatal("mixed group LED should be off")
	}
	result, err := sessions.ExecuteAudioCommand(AudioCommand{Target: "deej.slider.3", Operation: AudioCommandCurrentMute})
	if err != nil || len(result.Mutes) != 2 {
		t.Fatalf("group resolution = %+v, %v; duplicate sessions must be removed", result, err)
	}
	press := PrototypeSerialEvent{Kind: PrototypeSerialButtonEvent, ControlID: prototypeForegroundMuteControl, ButtonPressed: true}
	release := press
	release.ButtonPressed = false
	prototype.handleEvent(press)
	if !discord.muted || !spotify.muted || !indicator.last(prototypeForegroundMuteControl) {
		t.Fatal("button did not mute full mapped group and light LED")
	}
	prototype.handleEvent(press)
	if !discord.muted || !spotify.muted {
		t.Fatal("duplicate press toggled group")
	}

	// This is the same config population path invoked by the file watcher.
	config.userConfig.Set(configKeySliderMapping, map[string][]string{"3": {"chrome.exe"}})
	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	if config.SliderMapping != mappingIdentity {
		t.Fatal("reload replaced the published mapping pointer")
	}
	prototype.syncIndicators(false)
	if indicator.last(prototypeForegroundMuteControl) || chrome.muted {
		t.Fatal("reload should read the new target without muting it")
	}
	prototype.handleEvent(press) // held press remains deduplicated across reload
	if chrome.muted {
		t.Fatal("reload rearmed a held button")
	}
	prototype.handleEvent(release)
	prototype.handleEvent(press)
	if !chrome.muted || !discord.muted || !spotify.muted || !indicator.last(prototypeForegroundMuteControl) {
		t.Fatal("button/LED did not retarget after reload")
	}

	config.userConfig.Set(configKeySliderMapping, map[string][]string{})
	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	prototype.handleEvent(release)
	prototype.handleEvent(press)
	if indicator.last(prototypeForegroundMuteControl) || !chrome.muted {
		t.Fatal("removed mapping reused stale target or left LED on")
	}
}

func TestSliderMappingCommandResolvesForeground(t *testing.T) {
	previous := currentWindowProcessNames
	defer func() { currentWindowProcessNames = previous }()
	currentWindowProcessNames = func() ([]string, error) { return []string{"chrome.exe"}, nil }
	config := &CanonicalConfig{SliderMapping: newSliderMap()}
	config.SliderMapping.set(3, []string{"deej.current"})
	chrome := &fakeCommandSession{key: "chrome.exe"}
	discord := &fakeCommandSession{key: "discord.exe"}
	sessions := &sessionMap{mixr: &Mixr{config: config}, lock: &sync.Mutex{}, lastSessionRefresh: time.Now(), m: map[string][]Session{"chrome.exe": {chrome}, "discord.exe": {discord}}}
	if _, err := sessions.ExecuteAudioCommand(AudioCommand{Target: "deej.slider.3", Operation: AudioCommandToggleMute}); err != nil {
		t.Fatal(err)
	}
	if !chrome.muted || discord.muted {
		t.Fatal("mapped foreground target did not follow active window")
	}
	for _, target := range []string{"deej.slider.-1", "deej.slider.bad", "deej.slider.99"} {
		if _, err := sessions.ExecuteAudioCommand(AudioCommand{Target: target, Operation: AudioCommandToggleMute}); err == nil {
			t.Fatalf("invalid/unmapped target %q succeeded", target)
		}
	}
}

func TestSliderMappingReplacementConcurrentReaders(t *testing.T) {
	mappings := newSliderMap()
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 500; j++ {
				mappings.get(3)
				mappings.iterate(func(_ int, _ []string) {})
			}
		}()
	}
	for i := 0; i < 500; i++ {
		next := newSliderMap()
		next.set(3, []string{"discord.exe"})
		mappings.replace(next)
	}
	readers.Wait()
}

func TestFollowSliderMappingConfigCompatibility(t *testing.T) {
	config, err := NewConfig(zap.NewNop().Sugar(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	if config.HardwareMutePrototype.FollowSliderMapping {
		t.Fatal("legacy configurations must not opt in automatically")
	}
	config.userConfig.Set(configKeyFollowSliderMapping, true)
	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	if len(config.ValidationWarnings) != 1 {
		t.Fatalf("legacy firmware opt-in warnings = %q", config.ValidationWarnings)
	}
}
