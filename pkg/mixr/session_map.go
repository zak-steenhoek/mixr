package mixr

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thoas/go-funk"
	"go.uber.org/zap"
)

// Command-only references use the same live mapping as a physical knob.
const sliderCommandTargetPrefix = "deej.slider."

type sessionMap struct {
	mixr   *Mixr
	logger *zap.SugaredLogger

	m    map[string][]Session
	lock sync.Locker

	refreshMu         sync.Mutex
	refreshStateMu    sync.Mutex
	refreshRequests   uint64
	refreshInProgress bool
	refreshDone       chan struct{}
	released          bool
	lastSuppressedLog time.Time

	sessionFinder SessionFinder

	lastSessionRefresh time.Time
	unmappedSessions   []Session
}

const (
	masterSessionName = "master" // master device volume
	systemSessionName = "system" // system sounds volume
	inputSessionName  = "mic"    // microphone input level

	// some targets need to be transformed before their correct audio sessions can be accessed.
	// this prefix identifies those targets to ensure they don't contradict with another similarly-named process
	specialTargetTransformPrefix = "deej."

	// targets the currently active window (Windows-only, experimental)
	specialTargetCurrentWindow = "current"

	// targets all currently unmapped sessions (experimental)
	specialTargetAllUnmapped = "unmapped"

	// this threshold constant assumes that re-acquiring all sessions is a kind of expensive operation,
	// and needs to be limited in some manner. this value was previously user-configurable through a config
	// key "process_refresh_frequency", but exposing this type of implementation detail seems wrong now
	minTimeBetweenSessionRefreshes = time.Second * 5

	// determines whether the map should be refreshed when a slider moves.
	// this is a bit greedy but allows us to ensure sessions are always re-acquired, which is
	// especially important for process groups (because you can have one ongoing session
	// always preventing lookup of other processes bound to its slider, which forces the user
	// to manually refresh sessions). a cleaner way to do this down the line is by registering to notifications
	// whenever a new session is added, but that's too hard to justify for how easy this solution is
	maxTimeBetweenSessionRefreshes = time.Second * 45

	// routine cooldown suppression is useful forensic context but must not
	// become another high-frequency diagnostic stream.
	sessionRefreshDiagnosticInterval = 30 * time.Second
)

// this matches friendly device names (on Windows), e.g. "Headphones (Realtek Audio)"
var deviceSessionKeyPattern = regexp.MustCompile(`^.+ \(.+\)$`)

func newSessionMap(mixr *Mixr, logger *zap.SugaredLogger, sessionFinder SessionFinder) (*sessionMap, error) {
	logger = logger.Named("sessions")

	m := &sessionMap{
		mixr:          mixr,
		logger:        logger,
		m:             make(map[string][]Session),
		lock:          &sync.Mutex{},
		sessionFinder: sessionFinder,
	}

	logger.Debug("Created session map instance")

	return m, nil
}

func (m *sessionMap) initialize() error {
	if err := m.refreshSessionSnapshot("startup"); err != nil {
		m.logger.Warnw("Failed to get all sessions during session map initialization", "error", err)
		return fmt.Errorf("get all sessions during init: %w", err)
	}
	m.finishRefresh(time.Now())

	m.setupOnConfigReload()
	m.setupOnSliderMove()

	return nil
}

func (m *sessionMap) release() error {
	m.refreshStateMu.Lock()
	m.released = true
	m.refreshStateMu.Unlock()
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.lock.Lock()
	for _, sessions := range m.m {
		releaseSessions(sessions)
	}
	m.m = make(map[string][]Session)
	m.unmappedSessions = nil
	m.lock.Unlock()

	if err := m.sessionFinder.Release(); err != nil {
		m.logger.Warnw("Failed to release session finder during session map release", "error", err)
		return fmt.Errorf("release session finder during release: %w", err)
	}

	return nil
}

// refreshSessionSnapshot discovers and installs one complete snapshot while
// commands are excluded. Old sessions remain usable if discovery fails.
func (m *sessionMap) refreshSessionSnapshot(reason string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	sessions, err := m.sessionFinder.GetAllSessions()
	if err != nil {
		m.logger.Warnw("Failed to get sessions from session finder", "error", err)
		return fmt.Errorf("get sessions from SessionFinder: %w", err)
	}

	next := make(map[string][]Session)
	unmapped := make([]Session, 0)
	for _, session := range sessions {
		key := session.Key()
		next[key] = append(next[key], session)
		if !m.sessionMapped(session) {
			m.logger.Debugw("Tracking unmapped session", "session", session)
			unmapped = append(unmapped, session)
		}
	}

	for _, oldSessions := range m.m {
		releaseSessions(oldSessions)
	}
	m.m = next
	m.unmappedSessions = unmapped
	m.logger.Infow("Got all audio sessions successfully", "reason", reason, "sessionCount", len(sessions))

	return nil
}

func (m *sessionMap) setupOnConfigReload() {
	configReloadedChannel := m.mixr.config.SubscribeToChanges()

	go func() {
		for {
			select {
			case <-configReloadedChannel:
				m.logger.Info("Detected config reload, attempting to re-acquire all audio sessions")
				m.refreshSessionsWithReason(false, "configuration-reload")
			}
		}
	}()
}

func (m *sessionMap) setupOnSliderMove() {
	sliderEventsChannel := m.mixr.serial.SubscribeToSliderMoveEvents()

	go func() {
		for {
			select {
			case event := <-sliderEventsChannel:
				m.handleSliderMoveEvent(event)
			}
		}
	}()
}

// performance: explain why force == true at every such use to avoid unintended forced refresh spams
func (m *sessionMap) refreshSessions(force bool) {
	m.refreshSessionsWithReason(force, "unspecified")
}

func (m *sessionMap) refreshSessionsWithReason(force bool, reason string) bool {
	m.refreshStateMu.Lock()
	m.refreshRequests++
	requestID := m.refreshRequests
	if m.released {
		m.refreshStateMu.Unlock()
		m.logger.Debugw("Audio session refresh ignored after shutdown", "requestID", requestID, "reason", reason)
		return false
	}
	if m.refreshInProgress {
		done := m.refreshDone
		m.refreshStateMu.Unlock()
		<-done
		m.logger.Debugw("Audio session refresh coalesced", "requestID", requestID, "reason", reason)
		return false
	}
	lastCompleted := m.lastSessionRefresh
	now := time.Now()
	if !force && lastCompleted.Add(minTimeBetweenSessionRefreshes).After(now) {
		logSuppression := !m.lastSuppressedLog.Add(sessionRefreshDiagnosticInterval).After(now)
		if logSuppression {
			m.lastSuppressedLog = now
		}
		m.refreshStateMu.Unlock()
		if logSuppression {
			m.logger.Debugw("Audio session refresh suppressed by global cooldown",
				"requestID", requestID,
				"reason", reason,
				"sincePreviousCompletion", now.Sub(lastCompleted))
		}
		return false
	}
	m.refreshInProgress = true
	m.refreshDone = make(chan struct{})
	m.refreshStateMu.Unlock()

	m.logger.Debugw("Audio session refresh requested", "requestID", requestID, "reason", reason, "force", force)
	m.logger.Debugw("Audio session refresh started", "requestID", requestID, "reason", reason, "sincePreviousCompletion", now.Sub(lastCompleted))
	m.refreshMu.Lock()
	err := m.refreshSessionSnapshot(reason)
	m.refreshMu.Unlock()
	m.completeCoordinatedRefresh(time.Now())
	if err != nil {
		m.logger.Warnw("Failed to re-acquire all audio sessions", "requestID", requestID, "reason", reason, "error", err)
		return true
	}
	m.logger.Debugw("Audio session refresh completed", "requestID", requestID, "reason", reason)
	return true
}

func (m *sessionMap) finishRefresh(completed time.Time) {
	m.refreshStateMu.Lock()
	m.lastSessionRefresh = completed
	m.refreshStateMu.Unlock()
}

func (m *sessionMap) completeCoordinatedRefresh(completed time.Time) {
	m.refreshStateMu.Lock()
	m.lastSessionRefresh = completed
	m.refreshInProgress = false
	close(m.refreshDone)
	m.refreshDone = nil
	m.refreshStateMu.Unlock()
}

func (m *sessionMap) sessionSnapshotStale(now time.Time) bool {
	m.refreshStateMu.Lock()
	defer m.refreshStateMu.Unlock()
	return m.lastSessionRefresh.Add(maxTimeBetweenSessionRefreshes).Before(now)
}

// returns true if a session is not currently mapped to any slider, false otherwise
// special sessions (master, system, mic) and device-specific sessions always count as mapped,
// even when absent from the config. this makes sense for every current feature that uses "unmapped sessions"
func (m *sessionMap) sessionMapped(session Session) bool {

	// count master/system/mic as mapped
	if funk.ContainsString([]string{masterSessionName, systemSessionName, inputSessionName}, session.Key()) {
		return true
	}

	// count device sessions as mapped
	if deviceSessionKeyPattern.MatchString(session.Key()) {
		return true
	}

	matchFound := false

	// look through the actual mappings
	m.mixr.config.SliderMapping.iterate(func(sliderIdx int, targets []string) {
		for _, target := range targets {

			// ignore special transforms
			if m.targetHasSpecialTransform(target) {
				continue
			}

			// safe to assume this has a single element because we made sure there's no special transform
			target = m.resolveTarget(target)[0]

			if target == session.Key() {
				matchFound = true
				return
			}
		}
	})

	return matchFound
}

func (m *sessionMap) handleSliderMoveEvent(event SliderMoveEvent) {

	// first of all, ensure our session map isn't moldy
	if m.sessionSnapshotStale(time.Now()) {
		m.logger.Debug("Stale session map detected on slider move, refreshing")
		m.refreshSessionsWithReason(false, "slider-stale-snapshot")
	}

	// get the targets mapped to this slider from the config
	targets, ok := m.mixr.config.SliderMapping.get(event.SliderID)

	// if slider not found in config, silently ignore
	if !ok {
		return
	}

	targetFound := false
	adjustmentFailed := false
	m.lock.Lock()

	// for each possible target for this slider...
	for _, target := range targets {

		// resolve the target name by cleaning it up and applying any special transformations.
		// depending on the transformation applied, this can result in more than one target name
		resolvedTargets := m.resolveTarget(target)

		// for each resolved target...
		for _, resolvedTarget := range resolvedTargets {

			// check the map for matching sessions
			sessions, ok := m.m[resolvedTarget]

			// no sessions matching this target - move on
			if !ok {
				continue
			}

			targetFound = true

			// iterate all matching sessions and adjust the volume of each one
			for _, session := range sessions {
				if session.GetVolume() != event.PercentValue {
					if err := session.SetVolume(event.PercentValue); err != nil {
						m.logger.Warnw("Failed to set target session volume", "error", err)
						adjustmentFailed = true
					}
				}
			}
		}
	}
	m.lock.Unlock()

	// if we still haven't found a target or the volume adjustment failed, maybe look for the target again.
	// processes could've opened since the last time this slider moved.
	// if they haven't, the cooldown will take care to not spam it up
	if !targetFound {
		m.refreshSessionsWithReason(false, "slider-target-unavailable")
	} else if adjustmentFailed {

		// performance: the reason that forcing a refresh here is okay is that we'll only get here
		// when a session's SetVolume call errored, such as in the case of a stale master session
		// (or another, more catastrophic failure happens)
		m.refreshSessionsWithReason(false, "slider-session-error")
	}
}

// ExecuteAudioCommand lets future encoder/button event sources use the tray
// application's existing session snapshot without depending on SerialIO.
func (m *sessionMap) ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error) {
	if m.sessionSnapshotStale(time.Now()) {
		m.refreshSessionsWithReason(false, "command-stale-snapshot")
	}

	m.lock.Lock()
	result, err := m.executeAudioCommandLocked(command)
	m.lock.Unlock()
	if errors.Is(err, errRefreshSessions) {
		m.refreshSessionsWithReason(false, "command-reported-stale-session")
		m.lock.Lock()
		result, err = m.executeAudioCommandLocked(command)
		m.lock.Unlock()
	}
	return result, err
}

func (m *sessionMap) executeAudioCommandLocked(command AudioCommand) (AudioCommandResult, error) {
	targets := []string{command.Target}
	if strings.HasPrefix(strings.ToLower(command.Target), sliderCommandTargetPrefix) {
		index, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(command.Target), sliderCommandTargetPrefix))
		if err != nil || index < 0 || m.mixr == nil || m.mixr.config == nil || m.mixr.config.SliderMapping == nil {
			return AudioCommandResult{Target: command.Target}, fmt.Errorf("invalid slider command target %q", command.Target)
		}
		targets, _ = m.mixr.config.SliderMapping.get(index)
	}
	resolvedTargets := []string{}
	for _, target := range targets {
		resolvedTargets = append(resolvedTargets, m.resolveTarget(target)...)
	}
	seen := make(map[Session]struct{})
	matched := make([]audioCommandSession, 0)
	for _, target := range resolvedTargets {
		for _, session := range m.m[target] {
			if _, exists := seen[session]; exists {
				continue
			}
			seen[session] = struct{}{}
			matched = append(matched, session)
		}
	}

	return executeAudioCommand(matched, command)
}

func (m *sessionMap) targetHasSpecialTransform(target string) bool {
	return strings.HasPrefix(target, specialTargetTransformPrefix)
}

func (m *sessionMap) resolveTarget(target string) []string {
	return resolveAudioTargetNames(target, m.unmappedSessions)
}

func (m *sessionMap) applyTargetTransform(specialTargetName string) []string {
	return resolveSpecialAudioTarget(specialTargetName, m.unmappedSessions)
}

func (m *sessionMap) add(value Session) {
	m.lock.Lock()
	defer m.lock.Unlock()

	key := value.Key()

	existing, ok := m.m[key]
	if !ok {
		m.m[key] = []Session{value}
	} else {
		m.m[key] = append(existing, value)
	}
}

func (m *sessionMap) get(key string) ([]Session, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	value, ok := m.m[key]
	return value, ok
}

func (m *sessionMap) clear() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.logger.Debug("Releasing and clearing all audio sessions")

	for key, sessions := range m.m {
		for _, session := range sessions {
			session.Release()
		}

		delete(m.m, key)
	}

	m.logger.Debug("Session map cleared")
}

func (m *sessionMap) String() string {
	m.lock.Lock()
	defer m.lock.Unlock()

	sessionCount := 0

	for _, value := range m.m {
		sessionCount += len(value)
	}

	return fmt.Sprintf("<%d audio sessions>", sessionCount)
}

func (m *sessionMap) sessionCounts() map[string]int {
	m.lock.Lock()
	defer m.lock.Unlock()

	counts := make(map[string]int, len(m.m))
	for key, sessions := range m.m {
		counts[key] = len(sessions)
	}
	return counts
}
