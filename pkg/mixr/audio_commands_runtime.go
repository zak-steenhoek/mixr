package mixr

import (
	"fmt"

	"go.uber.org/zap"
)

// SystemAudioCommandExecutor discovers the current platform audio sessions and
// executes transport-independent commands against them. Call Refresh after
// application-session churn and Close when finished.
type SystemAudioCommandExecutor struct {
	finder   SessionFinder
	sessions []Session
	closed   bool
}

// NewSystemAudioCommandExecutor creates a software-only command endpoint using
// the same session finder as the tray application.
func NewSystemAudioCommandExecutor(logger *zap.SugaredLogger) (*SystemAudioCommandExecutor, error) {
	finder, err := newSessionFinder(logger)
	if err != nil {
		return nil, fmt.Errorf("create session finder: %w", err)
	}

	executor := &SystemAudioCommandExecutor{finder: finder}
	if err := executor.Refresh(); err != nil {
		finder.Release()
		return nil, err
	}
	return executor, nil
}

// Refresh releases the previous snapshot and discovers sessions again.
func (e *SystemAudioCommandExecutor) Refresh() error {
	if e.closed {
		return fmt.Errorf("audio command executor is closed")
	}
	releaseSessions(e.sessions)
	e.sessions = nil

	sessions, err := e.finder.GetAllSessions()
	if err != nil {
		return fmt.Errorf("discover audio sessions: %w", err)
	}
	e.sessions = sessions
	return nil
}

// ExecuteAudioCommand resolves master, deej.current, or an application name
// with the same case-insensitive exact-key semantics used by slider mappings.
func (e *SystemAudioCommandExecutor) ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error) {
	if e.closed {
		return AudioCommandResult{Target: command.Target}, fmt.Errorf("audio command executor is closed")
	}

	resolvedTargets := resolveAudioTargetNames(command.Target, e.sessions)
	seen := make(map[Session]struct{})
	matched := make([]audioCommandSession, 0)
	for _, target := range resolvedTargets {
		for _, session := range e.sessions {
			if session.Key() != target {
				continue
			}
			if _, exists := seen[session]; exists {
				continue
			}
			seen[session] = struct{}{}
			matched = append(matched, session)
		}
	}

	return executeAudioCommand(matched, command)
}

// Close releases every acquired session and the underlying finder exactly once.
func (e *SystemAudioCommandExecutor) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	releaseSessions(e.sessions)
	e.sessions = nil
	if err := e.finder.Release(); err != nil {
		return fmt.Errorf("release session finder: %w", err)
	}
	return nil
}

func releaseSessions(sessions []Session) {
	for _, session := range sessions {
		session.Release()
	}
}
