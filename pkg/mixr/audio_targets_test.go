package mixr

import (
	"errors"
	"testing"
)

func TestForegroundTargetTracksCurrentWindowAndChildProcesses(t *testing.T) {
	original := currentWindowProcessNames
	t.Cleanup(func() { currentWindowProcessNames = original })

	currentWindowProcessNames = func() ([]string, error) {
		return []string{"PLAYER.EXE", "player-helper.exe", "PLAYER.EXE"}, nil
	}
	got := resolveAudioTargetNames("deej.current", nil)
	if len(got) != 2 || got[0] != "player.exe" || got[1] != "player-helper.exe" {
		t.Fatalf("first foreground targets = %q", got)
	}

	currentWindowProcessNames = func() ([]string, error) { return []string{"browser.exe"}, nil }
	got = resolveAudioTargetNames("deej.current", nil)
	if len(got) != 1 || got[0] != "browser.exe" {
		t.Fatalf("switched foreground targets = %q", got)
	}
}

func TestForegroundTargetFailsSafeWithoutResolvableWindow(t *testing.T) {
	original := currentWindowProcessNames
	t.Cleanup(func() { currentWindowProcessNames = original })

	for _, resolver := range []func() ([]string, error){
		func() ([]string, error) { return nil, nil },
		func() ([]string, error) { return nil, errors.New("foreground unavailable") },
	} {
		currentWindowProcessNames = resolver
		if got := resolveAudioTargetNames("deej.current", nil); len(got) != 0 {
			t.Fatalf("unavailable foreground targets = %q, want none", got)
		}
	}
}

func TestForegroundCommandFailsSafeWhenCurrentAppHasNoAudioSession(t *testing.T) {
	original := currentWindowProcessNames
	t.Cleanup(func() { currentWindowProcessNames = original })
	currentWindowProcessNames = func() ([]string, error) { return []string{"silent.exe"}, nil }

	session := &fakeCommandSession{key: "audible.exe", muted: true}
	executor := &SystemAudioCommandExecutor{sessions: []Session{session}}
	_, err := executor.ExecuteAudioCommand(AudioCommand{Target: "deej.current", Operation: AudioCommandCurrentMute})
	if err == nil {
		t.Fatal("foreground app without an audio session unexpectedly resolved")
	}
	if !session.muted || session.muteReads != 0 {
		t.Fatalf("unrelated session changed or was queried: muted=%t reads=%d", session.muted, session.muteReads)
	}
}
