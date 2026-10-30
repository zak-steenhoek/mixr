package deej

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

// SerialIO provides a deej-aware abstraction layer to managing serial I/O.
type SerialIO struct {
	comPort  string
	baudRate uint

	deej   *Deej
	logger *zap.SugaredLogger

	stateMu              sync.RWMutex
	connectMu            sync.Mutex
	writeMu              sync.Mutex
	indicatorMu          sync.Mutex
	sliderMu             sync.Mutex
	connected            bool
	stopping             bool
	reconnectScheduled   bool
	lifecycle            uint64
	connectionGeneration uint64
	connOptions          serial.OpenOptions
	conn                 io.ReadWriteCloser
	openPort             func(serial.OpenOptions) (io.ReadWriteCloser, error)

	prototypeReady             bool
	prototypeEnabled           bool
	lastIndicatorStates        map[string]bool
	lastIndicatorSentAt        map[string]time.Time
	prototypeConsumers         []chan PrototypeSerialEvent
	sliderMoveConsumers        []chan SliderMoveEvent
	lastKnownNumSliders        int
	currentSliderPercentValues []float32
}

// SliderMoveEvent represents a single slider move captured by deej.
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

var expectedLinePattern = regexp.MustCompile(`^\d{1,4}(\|\d{1,4})*\r\n$`)

const prototypeIndicatorResyncInterval = 2 * time.Second

// NewSerialIO creates a SerialIO instance that uses the provided deej
// instance's connection info to establish communications with the Arduino.
func NewSerialIO(deej *Deej, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	sio := &SerialIO{
		deej:                deej,
		logger:              logger,
		lastIndicatorStates: make(map[string]bool),
		lastIndicatorSentAt: make(map[string]time.Time),
		prototypeConsumers:  []chan PrototypeSerialEvent{},
		sliderMoveConsumers: []chan SliderMoveEvent{},
		openPort:            serial.Open,
	}

	logger.Debug("Created serial i/o instance")
	sio.setupOnConfigReload()
	return sio, nil
}

// Start opens the configured serial port. After an established connection is
// lost, SerialIO retries automatically until Stop or a configuration restart.
func (sio *SerialIO) Start() error {
	sio.stateMu.Lock()
	if sio.connected {
		sio.stateMu.Unlock()
		sio.logger.Warn("Already connected, can't start another without closing first")
		return errors.New("serial: connection already active")
	}
	sio.stopping = false
	sio.prototypeEnabled = sio.deej.config.HardwareMutePrototype.Enabled
	sio.lifecycle++
	lifecycle := sio.lifecycle
	sio.stateMu.Unlock()

	return sio.openConnection(lifecycle)
}

func (sio *SerialIO) openConnection(lifecycle uint64) error {
	sio.connectMu.Lock()
	defer sio.connectMu.Unlock()

	sio.stateMu.RLock()
	if sio.stopping || sio.lifecycle != lifecycle {
		sio.stateMu.RUnlock()
		return errors.New("serial: connection attempt canceled")
	}
	if sio.connected {
		sio.stateMu.RUnlock()
		return nil
	}
	sio.stateMu.RUnlock()

	minimumReadSize := 0
	if util.Linux() {
		minimumReadSize = 1
	}
	options := serial.OpenOptions{
		PortName:        sio.deej.config.ConnectionInfo.COMPort,
		BaudRate:        uint(sio.deej.config.ConnectionInfo.BaudRate),
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: uint(minimumReadSize),
	}

	sio.logger.Debugw("Attempting serial connection",
		"comPort", options.PortName,
		"baudRate", options.BaudRate,
		"minReadSize", minimumReadSize)

	opener := sio.openPort
	if opener == nil {
		opener = serial.Open
	}
	conn, err := opener(options)
	if err != nil {
		sio.logger.Warnw("Failed to open serial connection", "error", err)
		return fmt.Errorf("open serial connection: %w", err)
	}

	sio.stateMu.Lock()
	if sio.stopping || sio.lifecycle != lifecycle {
		sio.stateMu.Unlock()
		conn.Close()
		return errors.New("serial: connection attempt canceled")
	}
	sio.connectionGeneration++
	generation := sio.connectionGeneration
	sio.connOptions = options
	sio.conn = conn
	sio.connected = true
	sio.prototypeReady = false
	sio.lastIndicatorStates = make(map[string]bool)
	sio.lastIndicatorSentAt = make(map[string]time.Time)
	sio.stateMu.Unlock()

	namedLogger := sio.logger.Named(strings.ToLower(options.PortName))
	namedLogger.Infow("Connected", "conn", conn, "generation", generation)
	go sio.readLoop(namedLogger, conn, lifecycle, generation)
	sio.stateMu.RLock()
	prototypeEnabled := sio.prototypeEnabled
	sio.stateMu.RUnlock()
	if prototypeEnabled {
		go sio.prototypeHandshake(namedLogger, lifecycle, generation)
	}
	return nil
}

// Stop closes the current serial connection and cancels reconnect attempts.
func (sio *SerialIO) Stop() {
	sio.stateMu.Lock()
	sio.stopping = true
	sio.lifecycle++
	sio.reconnectScheduled = false
	conn := sio.conn
	sio.conn = nil
	wasConnected := sio.connected
	sio.connected = false
	sio.prototypeReady = false
	sio.lastIndicatorStates = make(map[string]bool)
	sio.lastIndicatorSentAt = make(map[string]time.Time)
	sio.stateMu.Unlock()

	if conn == nil {
		sio.logger.Debug("Not currently connected, nothing to stop")
		return
	}
	if wasConnected {
		sio.logger.Debug("Shutting down serial connection")
	}
	if err := conn.Close(); err != nil {
		sio.logger.Warnw("Failed to close serial connection", "error", err)
	}
}

// SubscribeToSliderMoveEvents returns a channel that receives every slider move.
func (sio *SerialIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	sio.sliderMoveConsumers = append(sio.sliderMoveConsumers, ch)
	return ch
}

// SubscribeToPrototypeEvents returns a buffered channel for the experimental
// ready/switch messages. It does not receive legacy slider frames.
func (sio *SerialIO) SubscribeToPrototypeEvents() chan PrototypeSerialEvent {
	ch := make(chan PrototypeSerialEvent, 16)
	sio.prototypeConsumers = append(sio.prototypeConsumers, ch)
	return ch
}

func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.deej.config.SubscribeToChanges()
	const stopDelay = 50 * time.Millisecond

	go func() {
		for range configReloadedChannel {
			go func() {
				<-time.After(stopDelay)
				sio.sliderMu.Lock()
				sio.lastKnownNumSliders = 0
				sio.sliderMu.Unlock()
			}()

			sio.stateMu.RLock()
			oldOptions := sio.connOptions
			sio.stateMu.RUnlock()
			if sio.deej.config.ConnectionInfo.COMPort != oldOptions.PortName ||
				uint(sio.deej.config.ConnectionInfo.BaudRate) != oldOptions.BaudRate {
				sio.logger.Info("Detected change in connection parameters, attempting to renew connection")
				sio.Stop()
				<-time.After(stopDelay)
				if err := sio.Start(); err != nil {
					sio.logger.Warnw("Failed to renew connection after parameter change", "error", err)
				} else {
					sio.logger.Debug("Renewed connection successfully")
				}
			}
		}
	}()
}

func (sio *SerialIO) readLoop(logger *zap.SugaredLogger, conn io.ReadWriteCloser, lifecycle, generation uint64) {
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if sio.deej.Verbose() {
				logger.Warnw("Serial read stopped", "error", err)
			}
			sio.handleDisconnect(logger, conn, lifecycle, generation, err)
			return
		}
		if sio.deej.Verbose() {
			logger.Debugw("Read new line", "line", line)
		}
		sio.handleLine(logger, line)
	}
}

func (sio *SerialIO) handleDisconnect(logger *zap.SugaredLogger, conn io.ReadWriteCloser, lifecycle, generation uint64, cause error) {
	sio.stateMu.Lock()
	if sio.lifecycle != lifecycle || sio.connectionGeneration != generation || sio.conn != conn {
		sio.stateMu.Unlock()
		return
	}
	sio.conn = nil
	sio.connected = false
	sio.prototypeReady = false
	sio.lastIndicatorStates = make(map[string]bool)
	sio.lastIndicatorSentAt = make(map[string]time.Time)
	shouldReconnect := !sio.stopping
	if shouldReconnect && !sio.reconnectScheduled {
		sio.reconnectScheduled = true
		go sio.reconnectLoop(lifecycle)
	}
	sio.stateMu.Unlock()

	if err := conn.Close(); err != nil {
		logger.Debugw("Serial close after disconnect failed", "error", err)
	}
	logger.Warnw("Serial connection lost", "error", cause, "reconnecting", shouldReconnect)
}

func (sio *SerialIO) reconnectLoop(lifecycle uint64) {
	const retryDelay = time.Second
	for {
		sio.stateMu.RLock()
		canceled := sio.stopping || sio.lifecycle != lifecycle
		connected := sio.connected
		sio.stateMu.RUnlock()
		if canceled || connected {
			sio.finishReconnect(lifecycle)
			return
		}

		if err := sio.openConnection(lifecycle); err == nil {
			sio.logger.Info("Reconnected serial device")
			sio.finishReconnect(lifecycle)
			return
		} else if sio.deej.Verbose() {
			sio.logger.Debugw("Serial reconnect attempt failed", "error", err)
		}
		<-time.After(retryDelay)
	}
}

func (sio *SerialIO) finishReconnect(lifecycle uint64) {
	sio.stateMu.Lock()
	if sio.lifecycle == lifecycle {
		sio.reconnectScheduled = false
	}
	sio.stateMu.Unlock()
}

func (sio *SerialIO) prototypeHandshake(logger *zap.SugaredLogger, lifecycle, generation uint64) {
	const retryInterval = 750 * time.Millisecond
	for {
		sio.stateMu.RLock()
		active := !sio.stopping && sio.connected && sio.lifecycle == lifecycle && sio.connectionGeneration == generation
		ready := sio.prototypeReady
		sio.stateMu.RUnlock()
		if !active || ready {
			return
		}

		if err := sio.writePrototypeLine(lifecycle, generation, prototypeHelloMessage); err != nil {
			logger.Debugw("Prototype hello write failed", "error", err)
			return
		}
		if sio.deej.Verbose() {
			logger.Debug("Sent prototype hello")
		}
		<-time.After(retryInterval)
	}
}

func (sio *SerialIO) writePrototypeLine(lifecycle, generation uint64, message string) error {
	sio.stateMu.RLock()
	if sio.stopping || !sio.connected || sio.lifecycle != lifecycle || sio.connectionGeneration != generation || sio.conn == nil {
		sio.stateMu.RUnlock()
		return errors.New("serial: connection unavailable")
	}
	conn := sio.conn
	sio.stateMu.RUnlock()

	payload := []byte(message + "\n")
	sio.writeMu.Lock()
	written, err := conn.Write(payload)
	sio.writeMu.Unlock()
	if err == nil && written != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		sio.handleDisconnect(sio.logger, conn, lifecycle, generation, err)
		return fmt.Errorf("write serial message: %w", err)
	}
	return nil
}

// setPrototypeIndicator writes observed state changes immediately and repeats
// unchanged state at a low rate so one lost firmware command self-heals. The
// cache resets on every connection, so ready always synchronizes immediately.
func (sio *SerialIO) setPrototypeIndicator(controlID string, on bool) error {
	sio.indicatorMu.Lock()
	defer sio.indicatorMu.Unlock()

	sio.stateMu.RLock()
	if !sio.prototypeReady || !sio.connected {
		sio.stateMu.RUnlock()
		return nil
	}
	last, sent := sio.lastIndicatorStates[controlID]
	lastSentAt := sio.lastIndicatorSentAt[controlID]
	now := time.Now()
	if sent && last == on && now.Sub(lastSentAt) < prototypeIndicatorResyncInterval {
		sio.stateMu.RUnlock()
		return nil
	}
	lifecycle := sio.lifecycle
	generation := sio.connectionGeneration
	sio.stateMu.RUnlock()

	if err := sio.writePrototypeLine(lifecycle, generation, prototypeIndicatorMessage(controlID, on)); err != nil {
		return err
	}
	sio.stateMu.Lock()
	if sio.lifecycle == lifecycle && sio.connectionGeneration == generation && sio.prototypeReady {
		if sio.lastIndicatorSentAt == nil {
			sio.lastIndicatorSentAt = make(map[string]time.Time)
		}
		sio.lastIndicatorStates[controlID] = on
		sio.lastIndicatorSentAt[controlID] = now
	}
	sio.stateMu.Unlock()
	return nil
}

func (sio *SerialIO) handleLine(logger *zap.SugaredLogger, line string) {
	if event, ok := parsePrototypeSerialLine(line); ok {
		sio.handlePrototypeEvent(logger, event)
		return
	}

	// Legacy frames retain their exact parser and delivery path.
	if !expectedLinePattern.MatchString(line) {
		return
	}
	sio.handleSliderLine(logger, line)
}

func (sio *SerialIO) handlePrototypeEvent(logger *zap.SugaredLogger, event PrototypeSerialEvent) {
	sio.stateMu.RLock()
	prototypeEnabled := sio.prototypeEnabled
	sio.stateMu.RUnlock()
	if !prototypeEnabled {
		return
	}

	sio.stateMu.Lock()
	ready := sio.prototypeReady
	if event.Kind == PrototypeSerialReady {
		if ready {
			sio.stateMu.Unlock()
			return
		}
		sio.prototypeReady = true
		sio.lastIndicatorStates = make(map[string]bool)
		sio.lastIndicatorSentAt = make(map[string]time.Time)
		ready = true
	}
	sio.stateMu.Unlock()

	if !ready {
		if sio.deej.Verbose() {
			logger.Debugw("Ignoring prototype event before ready", "event", event)
		}
		return
	}
	if sio.deej.Verbose() {
		logger.Debugw("Accepted prototype serial event", "event", event)
	}
	for _, consumer := range sio.prototypeConsumers {
		select {
		case consumer <- event:
		default:
			logger.Warnw("Dropping prototype event for slow consumer", "event", event)
		}
	}
}

func (sio *SerialIO) handleSliderLine(logger *zap.SugaredLogger, line string) {
	sio.sliderMu.Lock()
	line = strings.TrimSuffix(line, "\r\n")
	splitLine := strings.Split(line, "|")
	numSliders := len(splitLine)

	if numSliders != sio.lastKnownNumSliders {
		logger.Infow("Detected sliders", "amount", numSliders)
		sio.lastKnownNumSliders = numSliders
		sio.currentSliderPercentValues = make([]float32, numSliders)
		for idx := range sio.currentSliderPercentValues {
			sio.currentSliderPercentValues[idx] = -1.0
		}
	}

	moveEvents := []SliderMoveEvent{}
	for sliderIdx, stringValue := range splitLine {
		number, _ := strconv.Atoi(stringValue)
		if sliderIdx == 0 && number > 1023 {
			sio.logger.Debugw("Got malformed line from serial, ignoring", "line", line)
			sio.sliderMu.Unlock()
			return
		}

		dirtyFloat := float32(number) / 1023.0
		normalizedScalar := util.NormalizeScalar(dirtyFloat)
		if sio.deej.config.InvertSliders {
			normalizedScalar = 1 - normalizedScalar
		}

		if util.SignificantlyDifferent(sio.currentSliderPercentValues[sliderIdx], normalizedScalar, sio.deej.config.NoiseReductionLevel) {
			sio.currentSliderPercentValues[sliderIdx] = normalizedScalar
			moveEvents = append(moveEvents, SliderMoveEvent{SliderID: sliderIdx, PercentValue: normalizedScalar})
			if sio.deej.Verbose() {
				logger.Debugw("Slider moved", "event", moveEvents[len(moveEvents)-1])
			}
		}
	}
	sio.sliderMu.Unlock()

	for _, consumer := range sio.sliderMoveConsumers {
		for _, moveEvent := range moveEvents {
			consumer <- moveEvent
		}
	}
}
