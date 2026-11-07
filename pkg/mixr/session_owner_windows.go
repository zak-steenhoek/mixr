package mixr

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	ole "github.com/go-ole/go-ole"
	"go.uber.org/zap"
)

const comRequestTimeout = 30 * time.Second

type comThreadRequest struct {
	run      func() error
	response chan error
	stop     bool
}

// comThreadExecutor owns every retained Core Audio interface on one STA thread.
// Callers may be concurrent, but interface work and release are serialized here.
type comThreadExecutor struct {
	requests chan comThreadRequest
	mu       sync.RWMutex
	closed   bool
}

func newCOMThreadExecutor() (*comThreadExecutor, error) {
	executor := &comThreadExecutor{requests: make(chan comThreadRequest, 16)}
	started := make(chan error, 1)
	go executor.run(started)
	if err := <-started; err != nil {
		return nil, err
	}
	return executor, nil
}

func (e *comThreadExecutor) run(started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := initializeCOMApartment(); err != nil {
		started <- err
		return
	}
	defer ole.CoUninitialize()
	started <- nil

	for request := range e.requests {
		if request.stop {
			request.response <- nil
			return
		}
		request.response <- request.run()
	}
}

func initializeCOMApartment() error {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err == nil {
		return nil
	}
	var oleError *ole.OleError
	if errors.As(err, &oleError) && oleError.Code() == 1 { // E_FALSE still requires CoUninitialize.
		return nil
	}
	return fmt.Errorf("initialize Core Audio COM apartment: %w", err)
}

func (e *comThreadExecutor) call(operation string, run func() error) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return fmt.Errorf("Core Audio owner is closed")
	}

	request := comThreadRequest{run: run, response: make(chan error, 1)}
	timer := time.NewTimer(comRequestTimeout)
	defer timer.Stop()
	select {
	case e.requests <- request:
	case <-timer.C:
		return fmt.Errorf("%s: Core Audio request queue timed out", operation)
	}
	select {
	case err := <-request.response:
		return err
	case <-timer.C:
		return fmt.Errorf("%s: Core Audio request timed out", operation)
	}
}

func (e *comThreadExecutor) close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	request := comThreadRequest{response: make(chan error, 1), stop: true}
	timer := time.NewTimer(comRequestTimeout)
	defer timer.Stop()
	select {
	case e.requests <- request:
	case <-timer.C:
		return fmt.Errorf("stop Core Audio owner: request queue timed out")
	}
	select {
	case err := <-request.response:
		return err
	case <-timer.C:
		return fmt.Errorf("stop Core Audio owner: request timed out")
	}
}

type threadOwnedSessionFinder struct {
	owner  *comThreadExecutor
	finder *wcaSessionFinder
}

func newSessionFinder(logger *zap.SugaredLogger) (SessionFinder, error) {
	owner, err := newCOMThreadExecutor()
	if err != nil {
		return nil, err
	}
	var finder *wcaSessionFinder
	if err := owner.call("create session finder", func() error {
		var createErr error
		finder, createErr = newWCASessionFinder(logger)
		return createErr
	}); err != nil {
		_ = owner.close()
		return nil, err
	}
	return &threadOwnedSessionFinder{owner: owner, finder: finder}, nil
}

func (f *threadOwnedSessionFinder) GetAllSessions() ([]Session, error) {
	var discovered []Session
	err := f.owner.call("discover audio sessions", func() error {
		var discoverErr error
		discovered, discoverErr = f.finder.GetAllSessions()
		return discoverErr
	})
	if err != nil {
		return nil, err
	}

	owned := make([]Session, 0, len(discovered))
	for _, session := range discovered {
		owned = append(owned, &threadOwnedSession{
			owner:   f.owner,
			session: session,
			key:     session.Key(),
		})
	}
	return owned, nil
}

func (f *threadOwnedSessionFinder) Release() error {
	releaseErr := f.owner.call("release session finder", f.finder.Release)
	closeErr := f.owner.close()
	if releaseErr != nil {
		return releaseErr
	}
	return closeErr
}

type threadOwnedSession struct {
	owner    *comThreadExecutor
	session  Session
	key      string
	released atomic.Bool
}

func (s *threadOwnedSession) Key() string { return s.key }

func (s *threadOwnedSession) GetVolume() float32 {
	var value float32
	_ = s.owner.call("get session volume", func() error {
		value = s.session.GetVolume()
		return nil
	})
	return value
}

func (s *threadOwnedSession) CurrentVolume() (float32, error) {
	var value float32
	err := s.owner.call("read session volume", func() error {
		var readErr error
		value, readErr = s.session.CurrentVolume()
		return readErr
	})
	return value, err
}

func (s *threadOwnedSession) SetVolume(value float32) error {
	return s.owner.call("set session volume", func() error { return s.session.SetVolume(value) })
}

func (s *threadOwnedSession) CurrentMute() (bool, error) {
	var muted bool
	err := s.owner.call("read session mute", func() error {
		var readErr error
		muted, readErr = s.session.CurrentMute()
		return readErr
	})
	return muted, err
}

func (s *threadOwnedSession) SetMute(muted bool) error {
	return s.owner.call("set session mute", func() error { return s.session.SetMute(muted) })
}

func (s *threadOwnedSession) Release() {
	if !s.released.CompareAndSwap(false, true) {
		return
	}
	_ = s.owner.call("release audio session", func() error {
		s.session.Release()
		return nil
	})
}
