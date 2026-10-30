package mixr

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestRelativeVolumeReadsEachSessionAndClamps(t *testing.T) {
	first := &fakeCommandSession{key: "spotify.exe", volume: 0.30}
	second := &fakeCommandSession{key: "spotify.exe", volume: 0.95}

	result, err := executeAudioCommand([]audioCommandSession{first, second}, AudioCommand{
		Target:    "spotify.exe",
		Operation: AudioCommandRelativeVolume,
		Steps:     2,
		Increment: 0.10,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertVolume(t, first.volume, 0.50)
	assertVolume(t, second.volume, 1.00)
	if first.volumeReads != 2 || second.volumeReads != 2 {
		t.Fatalf("volume reads = %d, %d; want two live reads per session", first.volumeReads, second.volumeReads)
	}
	if !result.MixedVolume {
		t.Fatal("mixed volume = false, want true")
	}
}

func TestAbsoluteVolumeClampsAndReadsBack(t *testing.T) {
	session := &fakeCommandSession{key: "mic", volume: 0.5}
	for _, test := range []struct{ requested, want float32 }{{-0.2, 0}, {0.42, 0.42}, {1.2, 1}} {
		result, err := executeAudioCommand([]audioCommandSession{session}, AudioCommand{Target: "mic", Operation: AudioCommandSetVolume, Volume: test.requested})
		if err != nil {
			t.Fatal(err)
		}
		assertVolume(t, session.volume, test.want)
		assertVolume(t, result.Volumes[0].Volume, test.want)
	}
}

func TestRelativeVolumeLowerBoundaryAndFreshState(t *testing.T) {
	session := &fakeCommandSession{key: "master", volume: 0.05}
	command := AudioCommand{Target: "master", Operation: AudioCommandRelativeVolume, Steps: -2, Increment: 0.10}

	if _, err := executeAudioCommand([]audioCommandSession{session}, command); err != nil {
		t.Fatal(err)
	}
	assertVolume(t, session.volume, 0)

	// Simulate an external Windows volume change. The next command must start
	// from this value rather than the previous command's desired state.
	session.volume = 0.80
	command.Steps = 1
	if _, err := executeAudioCommand([]audioCommandSession{session}, command); err != nil {
		t.Fatal(err)
	}
	assertVolume(t, session.volume, 0.90)
}

func TestRelativeVolumeRejectsInvalidIncrement(t *testing.T) {
	session := &fakeCommandSession{key: "master", volume: 0.5}
	for _, increment := range []float32{0, -0.1, 1.1, float32(math.NaN())} {
		_, err := executeAudioCommand([]audioCommandSession{session}, AudioCommand{
			Target: "master", Operation: AudioCommandRelativeVolume, Steps: 1, Increment: increment,
		})
		if err == nil {
			t.Fatalf("increment %v: expected error", increment)
		}
	}
}

func TestExplicitMuteTransitionsAllSessions(t *testing.T) {
	first := &fakeCommandSession{key: "chrome.exe", muted: false}
	second := &fakeCommandSession{key: "chrome.exe", muted: true}
	sessions := []audioCommandSession{first, second}

	muted, err := executeAudioCommand(sessions, AudioCommand{Target: "chrome.exe", Operation: AudioCommandMute})
	if err != nil {
		t.Fatal(err)
	}
	if muted.MuteState != AggregateMuteAllMuted || !first.muted || !second.muted {
		t.Fatalf("mute result = %q, states = %t/%t", muted.MuteState, first.muted, second.muted)
	}

	unmuted, err := executeAudioCommand(sessions, AudioCommand{Target: "chrome.exe", Operation: AudioCommandUnmute})
	if err != nil {
		t.Fatal(err)
	}
	if unmuted.MuteState != AggregateMuteAllUnmuted || first.muted || second.muted {
		t.Fatalf("unmute result = %q, states = %t/%t", unmuted.MuteState, first.muted, second.muted)
	}
}

func TestToggleMuteDeterministicallyHandlesMixedSessions(t *testing.T) {
	first := &fakeCommandSession{key: "spotify.exe", muted: true}
	second := &fakeCommandSession{key: "spotify.exe", muted: false}
	sessions := []audioCommandSession{first, second}

	result, err := executeAudioCommand(sessions, AudioCommand{Target: "spotify.exe", Operation: AudioCommandToggleMute})
	if err != nil {
		t.Fatal(err)
	}
	if result.MuteState != AggregateMuteAllMuted || !first.muted || !second.muted {
		t.Fatalf("mixed toggle result = %q, states = %t/%t", result.MuteState, first.muted, second.muted)
	}

	result, err = executeAudioCommand(sessions, AudioCommand{Target: "spotify.exe", Operation: AudioCommandToggleMute})
	if err != nil {
		t.Fatal(err)
	}
	if result.MuteState != AggregateMuteAllUnmuted || first.muted || second.muted {
		t.Fatalf("all-muted toggle result = %q, states = %t/%t", result.MuteState, first.muted, second.muted)
	}
}

func TestMuteQueryDetectsExternalMixedState(t *testing.T) {
	first := &fakeCommandSession{key: "app.exe", muted: false}
	second := &fakeCommandSession{key: "app.exe", muted: false}
	sessions := []audioCommandSession{first, second}

	result, err := executeAudioCommand(sessions, AudioCommand{Target: "app.exe", Operation: AudioCommandCurrentMute})
	if err != nil {
		t.Fatal(err)
	}
	if result.MuteState != AggregateMuteAllUnmuted {
		t.Fatalf("initial mute state = %q", result.MuteState)
	}

	second.muted = true // external Windows change between commands
	result, err = executeAudioCommand(sessions, AudioCommand{Target: "app.exe", Operation: AudioCommandCurrentMute})
	if err != nil {
		t.Fatal(err)
	}
	if result.MuteState != AggregateMuteMixed {
		t.Fatalf("external mixed mute state = %q, want %q", result.MuteState, AggregateMuteMixed)
	}
}

func TestAudioCommandRequiresMatchingSession(t *testing.T) {
	_, err := executeAudioCommand(nil, AudioCommand{Target: "missing.exe", Operation: AudioCommandCurrentVolume})
	if err == nil {
		t.Fatal("expected missing-session error")
	}
}

func TestSessionMapCommandUsesExistingApplicationMappingSemantics(t *testing.T) {
	first := &fakeCommandSession{key: "chrome.exe", volume: 0.25}
	second := &fakeCommandSession{key: "chrome.exe", volume: 0.75}
	m := &sessionMap{
		m: map[string][]Session{
			"chrome.exe": {first, second},
		},
		lock:               &sync.Mutex{},
		lastSessionRefresh: time.Now(),
	}

	result, err := m.ExecuteAudioCommand(AudioCommand{
		Target: "CHROME.EXE", Operation: AudioCommandRelativeVolume, Steps: 1, Increment: 0.10,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertVolume(t, first.volume, 0.35)
	assertVolume(t, second.volume, 0.85)
	if len(result.Volumes) != 2 {
		t.Fatalf("volume readbacks = %d, want 2", len(result.Volumes))
	}
}

type fakeCommandSession struct {
	key         string
	volume      float32
	muted       bool
	volumeReads int
	muteReads   int
}

func (s *fakeCommandSession) Key() string { return s.key }
func (s *fakeCommandSession) GetVolume() float32 {
	return s.volume
}
func (s *fakeCommandSession) CurrentVolume() (float32, error) {
	s.volumeReads++
	return s.volume, nil
}
func (s *fakeCommandSession) SetVolume(volume float32) error {
	s.volume = volume
	return nil
}
func (s *fakeCommandSession) CurrentMute() (bool, error) {
	s.muteReads++
	return s.muted, nil
}
func (s *fakeCommandSession) SetMute(muted bool) error {
	s.muted = muted
	return nil
}
func (s *fakeCommandSession) Release() {}

func assertVolume(t *testing.T, got, want float32) {
	t.Helper()
	if math.Abs(float64(got-want)) > 0.0001 {
		t.Fatalf("volume = %.4f, want %.4f", got, want)
	}
}
