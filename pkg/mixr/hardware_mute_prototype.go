package mixr

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

const prototypeWarningInterval = 30 * time.Second

type prototypeIndicatorWriter interface {
	setPrototypeIndicator(controlID string, on bool) error
}

type prototypeMuteControl struct {
	target            string
	hasLastSwitch     bool
	lastSwitchClosed  bool
	hasLastButton     bool
	lastButtonPressed bool
}

// hardwareMutePrototype is intentionally narrow and disposable. It supports
// the validated three-switch prototype and the opt-in five-button experiment,
// plus one absolute microphone-level path, without defining VFinal's channel
// architecture.
type hardwareMutePrototype struct {
	logger                  *zap.SugaredLogger
	commander               AudioCommander
	indicator               prototypeIndicatorWriter
	events                  <-chan PrototypeSerialEvent
	refresh                 func(reason string) bool
	controls                map[string]*prototypeMuteControl
	controlOrder            []string
	poll                    time.Duration
	verbose                 bool
	microphoneLevelInverted bool
	microphoneLevelDeadband int
	hasLastMicrophoneLevel  bool
	lastMicrophoneADC       int
	warnings                *diagnosticLimiter

	mu        sync.Mutex
	ready     bool
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newHardwareMutePrototype(mixr *Mixr, logger *zap.SugaredLogger) *hardwareMutePrototype {
	controls := map[string]*prototypeMuteControl{
		prototypeSpotifyMuteControl:    {target: mixr.config.HardwareMutePrototype.Target},
		prototypeMasterMuteControl:     {target: masterSessionName},
		prototypeMicrophoneMuteControl: {target: inputSessionName},
	}
	controlOrder := []string{prototypeMasterMuteControl, prototypeSpotifyMuteControl, prototypeMicrophoneMuteControl}
	if mixr.config.HardwareMutePrototype.FiveChannelMomentary {
		controls[prototypeChromeMuteControl] = &prototypeMuteControl{target: mixr.config.HardwareMutePrototype.ChromeTarget}
		controls[prototypeForegroundMuteControl] = &prototypeMuteControl{target: mixr.config.HardwareMutePrototype.ForegroundTarget}
		controlOrder = []string{
			prototypeMasterMuteControl,
			prototypeSpotifyMuteControl,
			prototypeChromeMuteControl,
			prototypeForegroundMuteControl,
			prototypeMicrophoneMuteControl,
		}
	}
	return &hardwareMutePrototype{
		logger:    logger.Named("hardware_mute_prototype"),
		commander: mixr,
		indicator: mixr.serial,
		events:    mixr.serial.SubscribeToPrototypeEvents(),
		refresh: func(reason string) bool {
			return mixr.sessions.refreshSessionsWithReason(false, reason)
		},
		controls:                controls,
		controlOrder:            controlOrder,
		poll:                    mixr.config.HardwareMutePrototype.PollInterval,
		verbose:                 mixr.Verbose(),
		microphoneLevelInverted: mixr.config.HardwareMutePrototype.MicrophoneLevelInverted,
		microphoneLevelDeadband: mixr.config.HardwareMutePrototype.MicrophoneLevelDeadband,
		warnings:                newDiagnosticLimiter(prototypeWarningInterval),
		stop:                    make(chan struct{}),
		done:                    make(chan struct{}),
	}
}

func (p *hardwareMutePrototype) Start() {
	p.startOnce.Do(func() {
		p.logger.Infow("Starting experimental hardware mute integration",
			"spotifyTarget", p.controls[prototypeSpotifyMuteControl].target,
			"masterTarget", p.controls[prototypeMasterMuteControl].target,
			"microphoneTarget", p.controls[prototypeMicrophoneMuteControl].target,
			"controls", p.controlOrder,
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
			p.logger.Info("Stopping experimental hardware mute integration")
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
		p.hasLastMicrophoneLevel = false
		for _, control := range p.controls {
			control.hasLastSwitch = false
			control.hasLastButton = false
		}
		p.logger.Infow("Prototype firmware ready; synchronizing indicators from Windows", "count", len(p.controlOrder))
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

	case PrototypeSerialButtonEvent:
		control, known := p.controls[event.ControlID]
		if !p.ready || !known {
			if p.verbose {
				p.logger.Debugw("Ignoring prototype button event", "ready", p.ready, "event", event)
			}
			return
		}
		if control.hasLastButton && control.lastButtonPressed == event.ButtonPressed {
			p.logger.Debugw("Ignoring duplicate stable button state", "control", event.ControlID, "pressed", event.ButtonPressed)
			return
		}
		control.hasLastButton = true
		control.lastButtonPressed = event.ButtonPressed
		if !event.ButtonPressed {
			if p.verbose {
				p.logger.Debugw("Accepted momentary button release without action", "control", event.ControlID, "target", control.target)
			}
			return
		}
		p.logger.Infow("Accepted debounced momentary mute press", "control", event.ControlID, "target", control.target)
		p.toggleLocked(event.ControlID, control)

	case PrototypeSerialAnalogEvent:
		if !p.ready || event.ControlID != prototypeMicrophoneLevelControl {
			return
		}
		p.applyMicrophoneLevelLocked(event.AnalogValue)
	}
}

func (p *hardwareMutePrototype) applyMicrophoneLevelLocked(adc int) {
	if p.hasLastMicrophoneLevel && absInt(adc-p.lastMicrophoneADC) < p.microphoneLevelDeadband {
		return
	}

	level := float32(adc) / 1023
	if adc <= p.microphoneLevelDeadband {
		level = 0
	} else if adc >= 1023-p.microphoneLevelDeadband {
		level = 1
	}
	if p.microphoneLevelInverted {
		level = 1 - level
	}

	control := p.controls[prototypeMicrophoneMuteControl]
	command := AudioCommand{Target: control.target, Operation: AudioCommandSetVolume, Volume: level}
	result, err := p.executeCommandWithRefreshLocked(control, command, false)
	if err != nil {
		if p.warnings.allow("microphone-level", time.Now()) {
			p.logger.Warnw("Microphone level target is unavailable; updates will resume after recovery", "adc", adc, "level", level, "error", err)
		}
		return
	}
	if p.warnings.recovered("microphone-level") {
		p.logger.Info("Microphone level target recovered")
	}
	p.hasLastMicrophoneLevel = true
	p.lastMicrophoneADC = adc
	if p.verbose {
		p.logger.Debugw("Microphone level synchronized", "adc", adc, "requested", level, "readback", result.Volumes)
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
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
	for _, controlID := range p.controlOrder {
		p.syncIndicatorLocked(controlID, p.controls[controlID], forceRefresh)
	}
}

func (p *hardwareMutePrototype) syncIndicatorLocked(controlID string, control *prototypeMuteControl, forceRefresh bool) {
	result, err := p.executeWithRefreshLocked(control, AudioCommandCurrentMute, forceRefresh)
	if err != nil {
		warningKey := "mute-readback:" + controlID
		if p.warnings.allow(warningKey, time.Now()) {
			p.logger.Warnw("Mute target is unavailable; setting indicator off until state is observable",
				"control", controlID,
				"target", control.target,
				"error", err)
		}
		if indicatorErr := p.indicator.setPrototypeIndicator(controlID, false); indicatorErr != nil {
			p.logger.Warnw("Failed to update mute indicator", "control", controlID, "error", indicatorErr)
		}
		return
	}
	if p.warnings.recovered("mute-readback:" + controlID) {
		p.logger.Infow("Mute target recovered", "control", controlID, "target", control.target)
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
	return p.executeCommandWithRefreshLocked(control, command, forceRefresh)
}

func (p *hardwareMutePrototype) executeCommandWithRefreshLocked(control *prototypeMuteControl, command AudioCommand, allowRecoveryRefresh bool) (AudioCommandResult, error) {
	result, err := p.commander.ExecuteAudioCommand(command)
	if err == nil || p.refresh == nil || !allowRecoveryRefresh {
		return result, err
	}

	reason := "explicit-button-target-unavailable"
	p.logger.Debugw("Requesting bounded recovery refresh after explicit audio command failure",
		"reason", reason, "target", control.target, "operation", command.Operation, "error", err)
	// Every refresh replaces the complete session snapshot and may therefore
	// select a different capture endpoint even when another target triggered
	// it. The next firmware heartbeat must reapply the absolute A4 position.
	if p.refresh(reason) {
		p.hasLastMicrophoneLevel = false
	}
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
