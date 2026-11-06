// Package mixr provides the machine-side client for the mixr hardware audio
// controller.
package mixr

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/zak-steenhoek/mixr/pkg/mixr/util"
)

const (
	// MIXR_NO_TRAY_ICON is the current identity. DEEJ_NO_TRAY_ICON remains
	// supported for compatibility with existing development workflows.
	envNoTray       = "MIXR_NO_TRAY_ICON"
	legacyEnvNoTray = "DEEJ_NO_TRAY_ICON"
)

// Mixr is the main entity managing access to all sub-components.
type Mixr struct {
	logger                *zap.SugaredLogger
	notifier              Notifier
	config                *CanonicalConfig
	serial                *SerialIO
	sessions              *sessionMap
	hardwareMutePrototype *hardwareMutePrototype

	stopChannel chan bool
	stopOnce    sync.Once
	version     string
	buildType   string
	versionTag  string
	gitCommit   string
	verbose     bool
}

// NewMixr creates a Mixr instance.
func NewMixr(logger *zap.SugaredLogger, verbose bool) (*Mixr, error) {
	logger = logger.Named("mixr")

	notifier, err := NewToastNotifier(logger)
	if err != nil {
		logger.Errorw("Failed to create ToastNotifier", "error", err)
		return nil, fmt.Errorf("create new ToastNotifier: %w", err)
	}

	config, err := NewConfig(logger, notifier)
	if err != nil {
		logger.Errorw("Failed to create Config", "error", err)
		return nil, fmt.Errorf("create new Config: %w", err)
	}

	d := &Mixr{
		logger:      logger,
		notifier:    notifier,
		config:      config,
		stopChannel: make(chan bool),
		verbose:     verbose,
	}

	serial, err := NewSerialIO(d, logger)
	if err != nil {
		logger.Errorw("Failed to create SerialIO", "error", err)
		return nil, fmt.Errorf("create new SerialIO: %w", err)
	}

	d.serial = serial

	sessionFinder, err := newSessionFinder(logger)
	if err != nil {
		logger.Errorw("Failed to create SessionFinder", "error", err)
		return nil, fmt.Errorf("create new SessionFinder: %w", err)
	}

	sessions, err := newSessionMap(d, logger, sessionFinder)
	if err != nil {
		logger.Errorw("Failed to create sessionMap", "error", err)
		return nil, fmt.Errorf("create new sessionMap: %w", err)
	}

	d.sessions = sessions

	logger.Debug("Created mixr instance")

	return d, nil
}

// Initialize sets up components and starts to run in the background
func (d *Mixr) Initialize() error {
	d.logger.Infow("Starting mixr",
		"version", d.displayVersion(),
		"buildType", valueOrUnknown(d.buildType),
		"gitCommit", valueOrUnknown(d.gitCommit),
		"goos", runtime.GOOS,
		"goarch", runtime.GOARCH)

	// load the config for the first time
	if err := d.config.Load(); err != nil {
		d.logger.Errorw("Failed to load config during initialization", "error", err)
		return fmt.Errorf("load config during init: %w", err)
	}

	// initialize the session map
	if err := d.sessions.initialize(); err != nil {
		d.logger.Errorw("Failed to initialize session map", "error", err)
		return fmt.Errorf("init session map: %w", err)
	}
	d.logStartupDiagnostics()

	if d.config.HardwareMutePrototype.Enabled {
		d.hardwareMutePrototype = newHardwareMutePrototype(d, d.logger)
		d.hardwareMutePrototype.Start()
	}

	// decide whether to run with/without tray
	if noTrayRequested() {

		d.logger.Debugw("Running without tray icon", "reason", "envvar set")

		// run in main thread while waiting on ctrl+C
		d.setupInterruptHandler()
		d.run()

	} else {
		d.setupInterruptHandler()
		d.initializeTray(d.run)
	}

	return nil
}

// SetVersion causes mixr to add a version string to its tray menu if called before Initialize.
func (d *Mixr) SetVersion(version string) {
	d.version = version
}

// SetBuildInfo records build metadata for startup diagnostics and the About menu.
func (d *Mixr) SetBuildInfo(buildType, versionTag, gitCommit string) {
	d.buildType = buildType
	d.versionTag = versionTag
	d.gitCommit = gitCommit
}

// Verbose returns whether mixr is running in verbose mode.
func (d *Mixr) Verbose() bool {
	return d.verbose
}

// ExecuteAudioCommand exposes the transport-independent command boundary on
// the tray application's existing session map.
func (d *Mixr) ExecuteAudioCommand(command AudioCommand) (AudioCommandResult, error) {
	if d.sessions == nil {
		return AudioCommandResult{Target: command.Target}, fmt.Errorf("audio sessions are not initialized")
	}
	return d.sessions.ExecuteAudioCommand(command)
}

func (d *Mixr) setupInterruptHandler() {
	interruptChannel := util.SetupCloseHandler()

	go func() {
		signal := <-interruptChannel
		d.logger.Debugw("Interrupted", "signal", signal)
		d.signalStop()
	}()
}

func (d *Mixr) run() {
	d.logger.Info("Run loop starting")

	// watch the config file for changes
	go d.config.WatchConfigFileChanges()

	// connect to the arduino for the first time
	go func() {
		if err := d.serial.Start(); err != nil {
			d.logger.Warnw("Initial serial connection failed; retrying in the background",
				"comPort", d.config.ConnectionInfo.COMPort,
				"baudRate", d.config.ConnectionInfo.BaudRate,
				"error", err)
			d.notifier.Notify(fmt.Sprintf("Waiting for %s", d.config.ConnectionInfo.COMPort),
				"mixr could not open the configured serial port and will keep retrying. Check the cable, port setting, and other serial-monitor applications.")
		}
	}()

	// wait until stopped (gracefully)
	<-d.stopChannel
	d.logger.Debug("Stop channel signaled, terminating")

	if err := d.stop(); err != nil {
		d.logger.Warnw("Failed to stop mixr", "error", err)
		os.Exit(1)
	} else {
		// exit with 0
		os.Exit(0)
	}
}

func noTrayRequested() bool {
	_, currentSet := os.LookupEnv(envNoTray)
	_, legacySet := os.LookupEnv(legacyEnvNoTray)
	return currentSet || legacySet
}

func (d *Mixr) signalStop() {
	d.stopOnce.Do(func() {
		d.logger.Debug("Signalling stop channel")
		d.stopChannel <- true
	})
}

func (d *Mixr) stop() error {
	d.logger.Info("Stopping")

	d.config.StopWatchingConfigFile()
	if d.hardwareMutePrototype != nil {
		d.hardwareMutePrototype.Stop()
	}
	d.serial.Stop()

	// release the session map
	if err := d.sessions.release(); err != nil {
		d.logger.Errorw("Failed to release session map", "error", err)
		return fmt.Errorf("release session map: %w", err)
	}

	d.stopTray()

	// attempt to sync on exit - this won't necessarily work but can't harm
	d.logger.Sync()

	return nil
}

func (d *Mixr) displayVersion() string {
	if d.version != "" {
		return d.version
	}
	if d.versionTag != "" {
		return d.versionTag
	}
	if d.gitCommit != "" {
		return d.gitCommit
	}
	return "development (build metadata unavailable)"
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func (d *Mixr) logStartupDiagnostics() {
	d.logger.Infow("Hardware control configuration",
		"legacyAnalogProtocol", "newline-delimited 0..1023 values separated by |",
		"adcRange", "0..1023",
		"analogMappings", d.config.SliderMapping,
		"hardwareMuteEnabled", d.config.HardwareMutePrototype.Enabled,
		"hardwareMuteTarget", d.config.HardwareMutePrototype.Target,
		"fiveChannelMomentary", d.config.HardwareMutePrototype.FiveChannelMomentary,
		"muteFollowsSliderMapping", d.config.HardwareMutePrototype.FollowSliderMapping,
		"chromeMuteTarget", d.config.HardwareMutePrototype.ChromeTarget,
		"foregroundMuteTarget", d.config.HardwareMutePrototype.ForegroundTarget,
		"microphoneAnalogEnabled", d.config.HardwareMutePrototype.Enabled,
		"microphoneInverted", d.config.HardwareMutePrototype.MicrophoneLevelInverted,
		"microphoneDeadbandADC", d.config.HardwareMutePrototype.MicrophoneLevelDeadband,
		"hardwarePollInterval", d.config.HardwareMutePrototype.PollInterval)

	counts := d.sessions.sessionCounts()
	d.logger.Infow("Windows audio endpoint availability",
		"defaultPlayback", counts[masterSessionName] > 0,
		"defaultRecording", counts[inputSessionName] > 0)
	if counts[inputSessionName] == 0 && d.config.HardwareMutePrototype.Enabled {
		d.logger.Warn("Default recording endpoint is unavailable; microphone level/mute controls will recover after an endpoint appears")
	}

	targetSet := make(map[string]struct{})
	d.config.SliderMapping.iterate(func(_ int, targets []string) {
		for _, target := range targets {
			targetSet[strings.ToLower(strings.TrimSpace(target))] = struct{}{}
		}
	})
	if d.config.HardwareMutePrototype.Enabled {
		targetSet[d.config.HardwareMutePrototype.Target] = struct{}{}
		targetSet[masterSessionName] = struct{}{}
		targetSet[inputSessionName] = struct{}{}
		if d.config.HardwareMutePrototype.FiveChannelMomentary {
			targetSet[d.config.HardwareMutePrototype.ChromeTarget] = struct{}{}
			targetSet[d.config.HardwareMutePrototype.ForegroundTarget] = struct{}{}
		}
	}
	targets := make([]string, 0, len(targetSet))
	for target := range targetSet {
		if target != "" {
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)
	for _, target := range targets {
		if strings.HasPrefix(target, specialTargetTransformPrefix) {
			d.logger.Infow("Configured dynamic audio target", "target", target)
			continue
		}
		if counts[target] == 0 {
			d.logger.Warnw("Configured audio target is currently unavailable; mixr will retry during session refresh", "target", target)
		} else {
			d.logger.Infow("Configured audio target available", "target", target, "sessions", counts[target])
		}
	}
}
