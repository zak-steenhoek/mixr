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

// hardwareMutePrototype is intentionally narrow and disposable. It proves the
// D2 -> Spotify toggle -> observed state -> D3 loop without defining VFinal's
// channel or protocol architecture.
type hardwareMutePrototype struct {
	logger    *zap.SugaredLogger
	commander AudioCommander
	indicator prototypeIndicatorWriter
	events    <-chan PrototypeSerialEvent
	refresh   func()
	target    string
	poll      time.Duration
	verbose   bool

	mu                 sync.Mutex
	ready              bool
	hasLastSwitch      bool
	lastSwitchClosed   bool
	lastSessionRefresh time.Time
	stop               chan struct{}
	done               chan struct{}
	startOnce          sync.Once
	stopOnce           sync.Once
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
		target:  deej.config.HardwareMutePrototype.Target,
		poll:    deej.config.HardwareMutePrototype.PollInterval,
		verbose: deej.Verbose(),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (p *hardwareMutePrototype) Start() {
	p.startOnce.Do(func() {
		p.logger.Infow("Starting experimental hardware mute integration",
			"target", p.target,
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
			p.syncIndicator(false)
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
		p.hasLastSwitch = false
		p.logger.Info("Prototype firmware ready; synchronizing indicator from Windows")
		p.syncIndicatorLocked(true)

	case PrototypeSerialSwitchEvent:
		if !p.ready || event.ControlID != prototypeSpotifyMuteControl {
			if p.verbose {
				p.logger.Debugw("Ignoring prototype switch event", "ready", p.ready, "event", event)
			}
			return
		}
		if p.hasLastSwitch && p.lastSwitchClosed == event.SwitchClosed {
			p.logger.Debugw("Ignoring duplicate stable switch state", "closed", event.SwitchClosed)
			return
		}
		p.hasLastSwitch = true
		p.lastSwitchClosed = event.SwitchClosed
		p.logger.Infow("Accepted debounced Spotify switch transition", "closed", event.SwitchClosed)
		p.toggleLocked()
	}
}

func (p *hardwareMutePrototype) toggleLocked() {
	result, err := p.executeWithRefreshLocked(AudioCommandToggleMute, true)
	if err != nil {
		p.logger.Warnw("Spotify mute toggle failed", "target", p.target, "error", err)
		p.syncIndicatorLocked(false)
		return
	}
	p.logger.Infow("Spotify mute toggle readback",
		"target", p.target,
		"sessionCount", len(result.Mutes),
		"muteState", result.MuteState)
	p.applyIndicatorLocked(result)
}

func (p *hardwareMutePrototype) syncIndicator(forceRefresh bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.syncIndicatorLocked(forceRefresh)
}

func (p *hardwareMutePrototype) syncIndicatorLocked(forceRefresh bool) {
	if !p.ready {
		return
	}
	result, err := p.executeWithRefreshLocked(AudioCommandCurrentMute, forceRefresh)
	if err != nil {
		p.logger.Warnw("Spotify mute readback failed; setting indicator off until state is observable",
			"target", p.target,
			"error", err)
		if indicatorErr := p.indicator.setPrototypeIndicator(prototypeSpotifyMuteControl, false); indicatorErr != nil {
			p.logger.Warnw("Failed to update Spotify mute indicator", "error", indicatorErr)
		}
		return
	}
	if p.verbose {
		p.logger.Debugw("Spotify mute state readback",
			"target", p.target,
			"sessionCount", len(result.Mutes),
			"muteState", result.MuteState)
	}
	p.applyIndicatorLocked(result)
}

func (p *hardwareMutePrototype) executeWithRefreshLocked(operation AudioCommandOperation, forceRefresh bool) (AudioCommandResult, error) {
	command := AudioCommand{Target: p.target, Operation: operation}
	result, err := p.commander.ExecuteAudioCommand(command)
	if err == nil || p.refresh == nil {
		return result, err
	}

	now := time.Now()
	if !forceRefresh && p.lastSessionRefresh.Add(prototypeSessionRefreshCooldown).After(now) {
		return result, err
	}
	p.logger.Debugw("Refreshing sessions after Spotify command failure", "operation", operation, "error", err)
	p.lastSessionRefresh = now
	p.refresh()
	return p.commander.ExecuteAudioCommand(command)
}

func (p *hardwareMutePrototype) applyIndicatorLocked(result AudioCommandResult) {
	// One LED cannot represent three aggregate states. On means every Spotify
	// session is muted; off means all-unmuted, mixed, or unavailable.
	on := result.MuteState == AggregateMuteAllMuted
	if result.MuteState == AggregateMuteMixed {
		p.logger.Infow("Spotify sessions have mixed mute state; indicator remains off",
			"sessionCount", len(result.Mutes))
	}
	if err := p.indicator.setPrototypeIndicator(prototypeSpotifyMuteControl, on); err != nil {
		p.logger.Warnw("Failed to update Spotify mute indicator", "on", on, "error", err)
		return
	}
	if p.verbose {
		p.logger.Debugw("Spotify mute indicator synchronized", "on", on, "muteState", result.MuteState)
	}
}
