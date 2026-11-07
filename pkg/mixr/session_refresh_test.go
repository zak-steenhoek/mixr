package mixr

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSimultaneousRefreshRequestsShareOneDiscovery(t *testing.T) {
	started := make(chan struct{})
	resume := make(chan struct{})
	finder := &refreshTestFinder{get: func(call int) ([]Session, error) {
		if call == 1 {
			close(started)
			<-resume
		}
		return []Session{newRefreshTestSession("chrome.exe")}, nil
	}}
	m := newRefreshTestMap(finder)
	m.lastSessionRefresh = time.Time{}

	results := make(chan bool, 2)
	go func() { results <- m.refreshSessionsWithReason(true, "first") }()
	<-started
	go func() { results <- m.refreshSessionsWithReason(true, "overlap") }()
	deadline := time.Now().Add(time.Second)
	for {
		m.refreshStateMu.Lock()
		requests := m.refreshRequests
		m.refreshStateMu.Unlock()
		if requests == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("overlapping refresh request did not arrive")
		}
		time.Sleep(time.Millisecond)
	}
	close(resume)
	first, second := <-results, <-results
	if finder.calls.Load() != 1 {
		t.Fatalf("discoveries = %d, want 1", finder.calls.Load())
	}
	if first == second {
		t.Fatalf("refresh results = %t/%t, want one performed and one coalesced", first, second)
	}
}

func TestAutomatedRefreshRequestsShareGlobalCooldown(t *testing.T) {
	finder := &refreshTestFinder{}
	m := newRefreshTestMap(finder)
	m.lastSessionRefresh = time.Now()
	for _, reason := range []string{"missing-current", "missing-chrome", "missing-spotify", "missing-mic"} {
		if m.refreshSessionsWithReason(false, reason) {
			t.Fatalf("refresh %q bypassed global cooldown", reason)
		}
	}
	if finder.calls.Load() != 0 {
		t.Fatalf("discoveries = %d, want 0", finder.calls.Load())
	}
}

func TestCommandCannotUseSessionDuringRelease(t *testing.T) {
	old := newRefreshTestSession("spotify.exe")
	old.readStarted = make(chan struct{})
	old.continueRead = make(chan struct{})
	finder := &refreshTestFinder{get: func(int) ([]Session, error) {
		return []Session{newRefreshTestSession("spotify.exe")}, nil
	}}
	m := newRefreshTestMap(finder)
	m.m["spotify.exe"] = []Session{old}

	commandDone := make(chan error, 1)
	go func() {
		_, err := m.ExecuteAudioCommand(AudioCommand{Target: "spotify.exe", Operation: AudioCommandCurrentMute})
		commandDone <- err
	}()
	<-old.readStarted
	refreshDone := make(chan struct{})
	go func() {
		m.refreshSessionsWithReason(true, "test")
		close(refreshDone)
	}()

	select {
	case <-old.releasedCh:
		t.Fatal("session released while command was using it")
	case <-time.After(25 * time.Millisecond):
	}
	close(old.continueRead)
	if err := <-commandDone; err != nil {
		t.Fatal(err)
	}
	<-refreshDone
	if !old.released.Load() {
		t.Fatal("old session was not released after command completed")
	}
}

func TestStaleSnapshotRecoversPreviouslyAbsentApplication(t *testing.T) {
	finder := &refreshTestFinder{get: func(int) ([]Session, error) {
		return []Session{newRefreshTestSession("chrome.exe")}, nil
	}}
	m := newRefreshTestMap(finder)
	m.lastSessionRefresh = time.Now().Add(-maxTimeBetweenSessionRefreshes - time.Second)

	result, err := m.ExecuteAudioCommand(AudioCommand{Target: "chrome.exe", Operation: AudioCommandCurrentMute})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Mutes) != 1 || finder.calls.Load() != 1 {
		t.Fatalf("mutes/discoveries = %d/%d, want 1/1", len(result.Mutes), finder.calls.Load())
	}
}

func TestStaleEndpointErrorGetsOneBoundedRecovery(t *testing.T) {
	for _, target := range []string{masterSessionName, inputSessionName} {
		t.Run(target, func(t *testing.T) {
			stale := newRefreshTestSession(target)
			stale.readErr = errRefreshSessions
			finder := &refreshTestFinder{get: func(int) ([]Session, error) {
				return []Session{newRefreshTestSession(target)}, nil
			}}
			m := newRefreshTestMap(finder)
			m.m[target] = []Session{stale}
			m.lastSessionRefresh = time.Now().Add(-minTimeBetweenSessionRefreshes - time.Second)

			result, err := m.ExecuteAudioCommand(AudioCommand{Target: target, Operation: AudioCommandCurrentMute})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Mutes) != 1 || finder.calls.Load() != 1 {
				t.Fatalf("mutes/discoveries = %d/%d, want 1/1", len(result.Mutes), finder.calls.Load())
			}
		})
	}
}

func TestManualRefreshBypassesCooldown(t *testing.T) {
	finder := &refreshTestFinder{}
	m := newRefreshTestMap(finder)
	m.lastSessionRefresh = time.Now()
	if !m.refreshSessionsWithReason(true, "manual-tray-refresh") {
		t.Fatal("manual refresh was suppressed")
	}
	if finder.calls.Load() != 1 {
		t.Fatalf("discoveries = %d, want 1", finder.calls.Load())
	}
}

func newRefreshTestMap(finder SessionFinder) *sessionMap {
	config := &CanonicalConfig{SliderMapping: newSliderMap()}
	return &sessionMap{
		mixr:               &Mixr{config: config},
		logger:             zap.NewNop().Sugar(),
		m:                  make(map[string][]Session),
		lock:               &sync.Mutex{},
		sessionFinder:      finder,
		lastSessionRefresh: time.Now(),
	}
}

type refreshTestFinder struct {
	calls atomic.Int32
	get   func(call int) ([]Session, error)
}

func (f *refreshTestFinder) GetAllSessions() ([]Session, error) {
	call := int(f.calls.Add(1))
	if f.get != nil {
		return f.get(call)
	}
	return nil, nil
}

func (f *refreshTestFinder) Release() error { return nil }

type refreshTestSession struct {
	key          string
	muted        bool
	readErr      error
	readStarted  chan struct{}
	continueRead chan struct{}
	readOnce     sync.Once
	released     atomic.Bool
	releasedCh   chan struct{}
}

func newRefreshTestSession(key string) *refreshTestSession {
	return &refreshTestSession{key: key, releasedCh: make(chan struct{})}
}

func (s *refreshTestSession) Key() string                     { return s.key }
func (s *refreshTestSession) GetVolume() float32              { return 0.5 }
func (s *refreshTestSession) CurrentVolume() (float32, error) { return 0.5, nil }
func (s *refreshTestSession) SetVolume(float32) error         { return nil }
func (s *refreshTestSession) CurrentMute() (bool, error) {
	if s.readStarted != nil {
		s.readOnce.Do(func() { close(s.readStarted) })
		<-s.continueRead
	}
	return s.muted, s.readErr
}
func (s *refreshTestSession) SetMute(muted bool) error {
	if s.readErr != nil {
		return s.readErr
	}
	s.muted = muted
	return nil
}
func (s *refreshTestSession) Release() {
	if s.released.CompareAndSwap(false, true) {
		close(s.releasedCh)
	}
}
