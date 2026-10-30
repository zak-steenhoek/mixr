package deej

import (
	"errors"
	"strings"

	"go.uber.org/zap"
)

// Session represents a single addressable audio session
type Session interface {
	GetVolume() float32
	CurrentVolume() (float32, error)
	SetVolume(v float32) error
	CurrentMute() (bool, error)
	SetMute(muted bool) error

	Key() string
	Release()
}

// ErrMuteUnsupported is returned by non-Windows session implementations that
// do not provide mute control. Track B's supported runtime is Windows.
var ErrMuteUnsupported = errors.New("mute control is unsupported on this platform")

const (

	// ideally these would share a common ground in baseSession
	// but it will not call the child GetVolume correctly :/
	sessionCreationLogMessage = "Created audio session instance"

	// format this with s.humanReadableDesc and whatever the current volume is
	sessionStringFormat = "<session: %s, vol: %.2f>"
)

type baseSession struct {
	logger *zap.SugaredLogger
	system bool
	master bool

	// used by Key(), needs to be set by child
	name string

	// used by String(), needs to be set by child
	humanReadableDesc string
}

func (s *baseSession) Key() string {
	if s.system {
		return systemSessionName
	}

	if s.master {
		return strings.ToLower(s.name) // could be master or mic, or any device's friendly name
	}

	return strings.ToLower(s.name)
}
