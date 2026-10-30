package deej

import (
	"fmt"
	"math"
	"strings"
)

// AudioCommandOperation identifies a transport-independent audio operation.
type AudioCommandOperation string

const (
	AudioCommandRelativeVolume AudioCommandOperation = "relative-volume"
	AudioCommandMute           AudioCommandOperation = "mute"
	AudioCommandUnmute         AudioCommandOperation = "unmute"
	AudioCommandToggleMute     AudioCommandOperation = "toggle-mute"
	AudioCommandCurrentVolume  AudioCommandOperation = "current-volume"
	AudioCommandCurrentMute    AudioCommandOperation = "current-mute"
)

// AudioCommand describes one operation for a mapping target. Relative volume
// uses Steps*Increment as a signed scalar adjustment; Increment must be in
// (0, 1]. The command intentionally contains no serial or hardware concepts.
type AudioCommand struct {
	Target    string
	Operation AudioCommandOperation
	Steps     int
	Increment float32
}

// AudioVolumeState is one session's live volume readback.
type AudioVolumeState struct {
	SessionKey string
	Volume     float32
}

// AudioMuteState is one session's live mute readback.
type AudioMuteState struct {
	SessionKey string
	Muted      bool
}

// AggregateMuteState summarizes a target containing one or more sessions.
type AggregateMuteState string

const (
	AggregateMuteAllMuted   AggregateMuteState = "all-muted"
	AggregateMuteAllUnmuted AggregateMuteState = "all-unmuted"
	AggregateMuteMixed      AggregateMuteState = "mixed"
)

// AudioCommandResult reports actual state read from Core Audio. A result may
// accompany an error when only some independent sessions failed.
type AudioCommandResult struct {
	Target      string
	Volumes     []AudioVolumeState
	Mutes       []AudioMuteState
	MixedVolume bool
	MuteState   AggregateMuteState
}

// AudioCommander is the small command boundary future encoder/button events
// can call without depending on the serial transport.
type AudioCommander interface {
	ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error)
}

type audioCommandSession interface {
	Key() string
	CurrentVolume() (float32, error)
	SetVolume(float32) error
	CurrentMute() (bool, error)
	SetMute(bool) error
}

func executeAudioCommand(sessions []audioCommandSession, command AudioCommand) (AudioCommandResult, error) {
	result := AudioCommandResult{Target: command.Target}
	if len(sessions) == 0 {
		return result, fmt.Errorf("no audio sessions matched target %q", command.Target)
	}

	switch command.Operation {
	case AudioCommandRelativeVolume:
		if command.Increment <= 0 || command.Increment > 1 || math.IsNaN(float64(command.Increment)) || math.IsInf(float64(command.Increment), 0) {
			return result, fmt.Errorf("relative volume increment must be greater than 0 and at most 1")
		}
		var failures []string
		for index, session := range sessions {
			current, err := session.CurrentVolume()
			if err != nil {
				failures = append(failures, sessionFailure(index, session, "read current volume", err))
				continue
			}
			next := clampVolume(current + float32(command.Steps)*command.Increment)
			if err := session.SetVolume(next); err != nil {
				failures = append(failures, sessionFailure(index, session, "set relative volume", err))
				continue
			}
			actual, err := session.CurrentVolume()
			if err != nil {
				failures = append(failures, sessionFailure(index, session, "read volume back", err))
				continue
			}
			result.Volumes = append(result.Volumes, AudioVolumeState{SessionKey: session.Key(), Volume: actual})
		}
		result.MixedVolume = volumesAreMixed(result.Volumes)
		return result, combinedCommandError(failures)

	case AudioCommandCurrentVolume:
		var failures []string
		for index, session := range sessions {
			current, err := session.CurrentVolume()
			if err != nil {
				failures = append(failures, sessionFailure(index, session, "read current volume", err))
				continue
			}
			result.Volumes = append(result.Volumes, AudioVolumeState{SessionKey: session.Key(), Volume: current})
		}
		result.MixedVolume = volumesAreMixed(result.Volumes)
		return result, combinedCommandError(failures)

	case AudioCommandMute:
		return setAndReadMute(sessions, command, true)

	case AudioCommandUnmute:
		return setAndReadMute(sessions, command, false)

	case AudioCommandToggleMute:
		// Snapshot every live state before mutating anything. Mixed state is
		// deterministic: toggle mutes all unless every session is already muted.
		allMuted := true
		for index, session := range sessions {
			muted, err := session.CurrentMute()
			if err != nil {
				return result, fmt.Errorf("%s", sessionFailure(index, session, "read mute before toggle", err))
			}
			allMuted = allMuted && muted
		}
		return setAndReadMute(sessions, command, !allMuted)

	case AudioCommandCurrentMute:
		return readMuteStates(sessions, command)

	default:
		return result, fmt.Errorf("unsupported audio command operation %q", command.Operation)
	}
}

func setAndReadMute(sessions []audioCommandSession, command AudioCommand, muted bool) (AudioCommandResult, error) {
	result := AudioCommandResult{Target: command.Target}
	var failures []string
	for index, session := range sessions {
		if err := session.SetMute(muted); err != nil {
			failures = append(failures, sessionFailure(index, session, "set mute", err))
		}
	}

	readback, err := readMuteStates(sessions, command)
	if err != nil {
		failures = append(failures, err.Error())
	}
	result = readback
	return result, combinedCommandError(failures)
}

func readMuteStates(sessions []audioCommandSession, command AudioCommand) (AudioCommandResult, error) {
	result := AudioCommandResult{Target: command.Target}
	var failures []string
	mutedCount := 0
	for index, session := range sessions {
		muted, err := session.CurrentMute()
		if err != nil {
			failures = append(failures, sessionFailure(index, session, "read current mute", err))
			continue
		}
		if muted {
			mutedCount++
		}
		result.Mutes = append(result.Mutes, AudioMuteState{SessionKey: session.Key(), Muted: muted})
	}

	switch {
	case len(result.Mutes) == 0:
		result.MuteState = ""
	case mutedCount == len(result.Mutes):
		result.MuteState = AggregateMuteAllMuted
	case mutedCount == 0:
		result.MuteState = AggregateMuteAllUnmuted
	default:
		result.MuteState = AggregateMuteMixed
	}
	return result, combinedCommandError(failures)
}

func clampVolume(volume float32) float32 {
	if volume < 0 {
		return 0
	}
	if volume > 1 {
		return 1
	}
	return volume
}

func volumesAreMixed(states []AudioVolumeState) bool {
	if len(states) < 2 {
		return false
	}
	const tolerance = 0.0001
	first := states[0].Volume
	for _, state := range states[1:] {
		if math.Abs(float64(state.Volume-first)) > tolerance {
			return true
		}
	}
	return false
}

func sessionFailure(index int, session audioCommandSession, action string, err error) string {
	return fmt.Sprintf("session %d (%s): %s: %v", index+1, session.Key(), action, err)
}

func combinedCommandError(failures []string) error {
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("audio command failed: %s", strings.Join(failures, "; "))
}
