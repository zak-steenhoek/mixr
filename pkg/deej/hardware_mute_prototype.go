package deej

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

const prototypeSessionRefreshCooldown = 5 * time.Second

type prototypeIndicatorWriter interface {
	setPrototypeIndicator(controlID string, on bool) error
}

type prototypeMuteControl struct {
	target             string
	hasLastSwitch      bool
	lastSwitchClosed   bool
	lastSessionRefresh time.Time
}

// hardwareMutePrototype is intentionally narrow and disposable. It proves two
// independent switch -> Windows mute -> observed indicator loops without
// defining VFinal's channel or protocol architecture.
type hardwareMutePrototype struct {
	logger    *zap.SugaredLogger
	commander AudioCommander
	indicator prototypeIndicatorWriter
	events    <-chan PrototypeSerialEvent
	refresh   func()
	controls  map[string]*prototypeMuteControl
	poll      time.Duration
	verbose   bool

	mu        sync.Mutex
	ready     bool
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newHardwareMutePrototype(deej *Deej, logger *zap.SugaredLogger) *hardwareMutePrototype {
	return &hardwareMutePrototype{
		logger:    logger.Named("hardware_mute_prototype"),
		commander: deej,
		indicator: deej.serial,
		events:    deej.serial.SubscribeToPrototypeEvents(),
		refresh: func() {
			deej.sessions.refreshSessions(true)
		},
		controls: map[string]*prototypeMuteControl{
			prototypeSpotifyMuteControl: {target: deej.config.HardwareMutePrototype.Target},
			prototypeMasterMuteControl:  {target: masterSessionName},
		},
		poll:    deej.config.HardwareMutePrototype.PollInterval,
		verbose: deej.Verbose(),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (p *hardwareMutePrototype) Start() {
	p.startOnce.Do(func() {
		p.logger.Infow("Starting experimental dual hardware mute integration",
			"spotifyTarget", p.controls[prototypeSpotifyMuteControl].target,
			"masterTarget", p.controls[prototypeMasterMuteControl].target,
			"pollInterval", p.poll)
		go p.run()
	})
}

func (p *hardwareMutePrototype) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}

func (p *hardwareMutePrototype) run() {
	defer close(p.done)
	ticker := time.NewTicker(p.poll)
	defer ticker.Stop()

	for {
		select {
		case event := <-p.events:
			p.handleEvent(event)
		case <-ticker.C:
			p.syncIndicators(false)
		case <-p.stop:
			p.logger.Info("Stopping experimental dual hardware mute integration")
			return
		}
	}
}

func (p *hardwareMutePrototype) handleEvent(event PrototypeSerialEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Kind {
	case PrototypeSerialReady:
		p.ready = true
		for _, control := range p.controls {
			control.hasLastSwitch = false
		}
		p.logger.Info("Prototype firmware ready; synchronizing both indicators from Windows")
		p.syncIndicatorsLocked(true)

	case PrototypeSerialSwitchEvent:
		control, known := p.controls[event.ControlID]
		if !p.ready || !known {
			if p.verbose {
				p.logger.Debugw("Ignoring prototype switch event", "ready", p.ready, "event", event)
			}
			return
		}
		if control.hasLastSwitch && control.lastSwitchClosed == event.SwitchClosed {
			p.logger.Debugw("Ignoring duplicate stable switch state", "control", event.ControlID, "closed", event.SwitchClosed)
			return
		}
		control.hasLastSwitch = true
		control.lastSwitchClosed = event.SwitchClosed
		p.logger.Infow("Accepted debounced mute switch transition",
			"control", event.ControlID, "target", control.target, "closed", event.SwitchClosed)
		p.toggleLocked(event.ControlID, control)
	}
}

func (p *hardwareMutePrototype) toggleLocked(controlID string, control *prototypeMuteControl) {
	result, err := p.executeWithRefreshLocked(control, AudioCommandToggleMute, true)
	if err != nil {
		p.logger.Warnw("Mute toggle failed", "control", controlID, "target", control.target, "error", err)
		p.syncIndicatorLocked(controlID, control, false)
		return
	}
	p.logger.Infow("Mute toggle readback",
		"control", controlID,
		"target", control.target,
		"sessionCount", len(result.Mutes),
		"muteState", result.MuteState)
	p.applyIndicatorLocked(controlID, control, result)
}

func (p *hardwareMutePrototype) syncIndicators(forceRefresh bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.syncIndicatorsLocked(forceRefresh)
}

func (p *hardwareMutePrototype) syncIndicatorsLocked(forceRefresh bool) {
	if !p.ready {
		return
	}
	for _, controlID := range []string{prototypeSpotifyMuteControl, prototypeMasterMuteControl} {
		p.syncIndicatorLocked(controlID, p.controls[controlID], forceRefresh)
	}
}

func (p *hardwareMutePrototype) syncIndicatorLocked(controlID string, control *prototypeMuteControl, forceRefresh bool) {
	result, err := p.executeWithRefreshLocked(control, AudioCommandCurrentMute, forceRefresh)
	if err != nil {
		p.logger.Warnw("Mute readback failed; setting indicator off until state is observable",
			"control", controlID,
			"target", control.target,
			"error", err)
		if indicatorErr := p.indicator.setPrototypeIndicator(controlID, false); indicatorErr != nil {
			p.logger.Warnw("Failed to update mute indicator", "control", controlID, "error", indicatorErr)
		}
		return
	}
	if p.verbose {
		p.logger.Debugw("Mute state readback",
			"control", controlID,
			"target", control.target,
			"sessionCount", len(result.Mutes),
			"muteState", result.MuteState)
	}
	p.applyIndicatorLocked(controlID, control, result)
}

func (p *hardwareMutePrototype) executeWithRefreshLocked(control *prototypeMuteControl, operation AudioCommandOperation, forceRefresh bool) (AudioCommandResult, error) {
	command := AudioCommand{Target: control.target, Operation: operation}
	result, err := p.commander.ExecuteAudioCommand(command)
	if err == nil || p.refresh == nil {
		return result, err
	}

	now := time.Now()
	if !forceRefresh && control.lastSessionRefresh.Add(prototypeSessionRefreshCooldown).After(now) {
		return result, err
	}
	p.logger.Debugw("Refreshing sessions after mute command failure", "target", control.target, "operation", operation, "error", err)
	control.lastSessionRefresh = now
	p.refresh()
	return p.commander.ExecuteAudioCommand(command)
}

func (p *hardwareMutePrototype) applyIndicatorLocked(controlID string, control *prototypeMuteControl, result AudioCommandResult) {
	// One LED cannot represent three aggregate states. On means every observed
	// session is muted; off means all-unmuted, mixed, or unavailable. The master
	// target has exactly one endpoint session, so it cannot be mixed.
	on := result.MuteState == AggregateMuteAllMuted
	if result.MuteState == AggregateMuteMixed {
		p.logger.Infow("Sessions have mixed mute state; indicator remains off",
			"control", controlID, "target", control.target, "sessionCount", len(result.Mutes))
	}
	if err := p.indicator.setPrototypeIndicator(controlID, on); err != nil {
		p.logger.Warnw("Failed to update mute indicator", "control", controlID, "on", on, "error", err)
		return
	}
	if p.verbose {
		p.logger.Debugw("Mute indicator synchronized", "control", controlID, "on", on, "muteState", result.MuteState)
	}
}
