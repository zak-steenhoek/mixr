package mixr

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

	"github.com/zak-steenhoek/mixr/pkg/mixr/util"
)

// SerialIO provides a mixr-aware abstraction layer to managing serial I/O.
type SerialIO struct {
	comPort  string
	baudRate uint

	mixr   *Mixr
	logger *zap.SugaredLogger

	stateMu              sync.RWMutex
	connectMu            sync.Mutex
	writeMu              sync.Mutex
	indicatorMu          sync.Mutex
	sliderMu             sync.Mutex
	connected            bool
	stopping             bool
	reconnectScheduled   bool
	connectionState      SerialConnectionState
	statusConsumers      []chan SerialConnectionState
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
	reconnectDelay             time.Duration
	lastMalformedWarning       time.Time
	malformedSinceWarning      int
}

// SerialConnectionState is the user-facing lifecycle state shown in the tray.
type SerialConnectionState string

const (
	SerialDisconnected SerialConnectionState = "disconnected"
	SerialReconnecting SerialConnectionState = "reconnecting"
	SerialConnected    SerialConnectionState = "connected"
)

// SliderMoveEvent represents a single slider move captured by mixr.
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

var expectedLinePattern = regexp.MustCompile(`^\d{1,4}(\|\d{1,4})*\r\n$`)

const prototypeIndicatorResyncInterval = 2 * time.Second

// NewSerialIO creates a SerialIO instance that uses the provided mixr client.
// instance's connection info to establish communications with the Arduino.
func NewSerialIO(mixr *Mixr, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	sio := &SerialIO{
		mixr:                mixr,
		logger:              logger,
		lastIndicatorStates: make(map[string]bool),
		lastIndicatorSentAt: make(map[string]time.Time),
		prototypeConsumers:  []chan PrototypeSerialEvent{},
		sliderMoveConsumers: []chan SliderMoveEvent{},
		connectionState:     SerialDisconnected,
		statusConsumers:     []chan SerialConnectionState{},
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
	sio.prototypeEnabled = sio.mixr.config.HardwareMutePrototype.Enabled
	sio.lifecycle++
	lifecycle := sio.lifecycle
	sio.setConnectionStateLocked(SerialReconnecting)
	sio.stateMu.Unlock()

	err := sio.openConnection(lifecycle)
	if err != nil {
		sio.scheduleReconnect(lifecycle)
	}
	return err
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
		PortName:        sio.mixr.config.ConnectionInfo.COMPort,
		BaudRate:        uint(sio.mixr.config.ConnectionInfo.BaudRate),
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
		sio.logger.Debugw("Serial connection attempt failed", "comPort", options.PortName, "error", err)
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
	sio.setConnectionStateLocked(SerialConnected)
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
	sio.setConnectionStateLocked(SerialDisconnected)
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

// SubscribeToConnectionStatus returns a buffered stream whose first value is
// the current state. Slow consumers receive the newest state without blocking
// serial I/O.
func (sio *SerialIO) SubscribeToConnectionStatus() <-chan SerialConnectionState {
	sio.stateMu.Lock()
	defer sio.stateMu.Unlock()
	ch := make(chan SerialConnectionState, 1)
	ch <- sio.connectionState
	sio.statusConsumers = append(sio.statusConsumers, ch)
	return ch
}

func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.mixr.config.SubscribeToChanges()
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
			if sio.mixr.config.ConnectionInfo.COMPort != oldOptions.PortName ||
				uint(sio.mixr.config.ConnectionInfo.BaudRate) != oldOptions.BaudRate {
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
			if sio.mixr.Verbose() {
				logger.Warnw("Serial read stopped", "error", err)
			}
			sio.handleDisconnect(logger, conn, lifecycle, generation, err)
			return
		}
		if sio.mixr.Verbose() {
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
		sio.setConnectionStateLocked(SerialReconnecting)
		go sio.reconnectLoop(lifecycle)
	} else if !shouldReconnect {
		sio.setConnectionStateLocked(SerialDisconnected)
	}
	sio.stateMu.Unlock()

	if err := conn.Close(); err != nil {
		logger.Debugw("Serial close after disconnect failed", "error", err)
	}
	logger.Warnw("Serial connection lost", "error", cause, "reconnecting", shouldReconnect)
}

func (sio *SerialIO) reconnectLoop(lifecycle uint64) {
	retryDelay := sio.reconnectDelay
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	lastWarning := time.Time{}
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
		} else if lastWarning.IsZero() || time.Since(lastWarning) >= 30*time.Second {
			sio.logger.Warnw("Serial device remains unavailable; retrying",
				"comPort", sio.mixr.config.ConnectionInfo.COMPort,
				"retryDelay", retryDelay,
				"error", err)
			lastWarning = time.Now()
		} else if sio.mixr.Verbose() {
			sio.logger.Debugw("Serial reconnect attempt failed", "error", err)
		}
		<-time.After(retryDelay)
	}
}

func (sio *SerialIO) scheduleReconnect(lifecycle uint64) {
	sio.stateMu.Lock()
	defer sio.stateMu.Unlock()
	if sio.stopping || sio.lifecycle != lifecycle || sio.connected || sio.reconnectScheduled {
		return
	}
	sio.reconnectScheduled = true
	sio.setConnectionStateLocked(SerialReconnecting)
	go sio.reconnectLoop(lifecycle)
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
	started := time.Now()
	lastWarning := time.Time{}
	for {
		sio.stateMu.RLock()
		active := !sio.stopping && sio.connected && sio.lifecycle == lifecycle && sio.connectionGeneration == generation
		ready := sio.prototypeReady
		sio.stateMu.RUnlock()
		if !active {
			return
		}
		if ready {
			logger.Infow("Prototype handshake complete", "generation", generation, "elapsed", time.Since(started).Round(time.Millisecond))
			return
		}

		if err := sio.writePrototypeLine(lifecycle, generation, prototypeHelloMessage); err != nil {
			logger.Debugw("Prototype hello write failed", "error", err)
			return
		}
		if sio.mixr.Verbose() {
			logger.Debug("Sent prototype hello")
		}
		if time.Since(started) >= 5*time.Second && (lastWarning.IsZero() || time.Since(lastWarning) >= 30*time.Second) {
			logger.Warnw("Waiting for prototype ready response; legacy slider frames remain supported",
				"generation", generation,
				"elapsed", time.Since(started).Round(time.Second))
			lastWarning = time.Now()
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
		sio.reportMalformedLine(logger, line)
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
		if sio.mixr.Verbose() {
			logger.Debugw("Ignoring prototype event before ready", "event", event)
		}
		return
	}
	if sio.mixr.Verbose() {
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

	values := make([]int, numSliders)
	for sliderIdx, stringValue := range splitLine {
		number, err := strconv.Atoi(stringValue)
		if err != nil || number < 0 || number > 1023 {
			sio.sliderMu.Unlock()
			sio.reportMalformedLine(logger, line)
			return
		}
		values[sliderIdx] = number
	}

	if numSliders != sio.lastKnownNumSliders {
		logger.Infow("Detected sliders", "amount", numSliders)
		sio.lastKnownNumSliders = numSliders
		sio.currentSliderPercentValues = make([]float32, numSliders)
		for idx := range sio.currentSliderPercentValues {
			sio.currentSliderPercentValues[idx] = -1.0
		}
	}

	moveEvents := []SliderMoveEvent{}
	for sliderIdx, number := range values {

		dirtyFloat := float32(number) / 1023.0
		normalizedScalar := util.NormalizeScalar(dirtyFloat)
		if sio.mixr.config.InvertSliders {
			normalizedScalar = 1 - normalizedScalar
		}

		if util.SignificantlyDifferent(sio.currentSliderPercentValues[sliderIdx], normalizedScalar, sio.mixr.config.NoiseReductionLevel) {
			sio.currentSliderPercentValues[sliderIdx] = normalizedScalar
			moveEvents = append(moveEvents, SliderMoveEvent{SliderID: sliderIdx, PercentValue: normalizedScalar})
			if sio.mixr.Verbose() {
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

func (sio *SerialIO) reportMalformedLine(logger *zap.SugaredLogger, line string) {
	sio.stateMu.Lock()
	sio.malformedSinceWarning++
	now := time.Now()
	if !sio.lastMalformedWarning.IsZero() && now.Sub(sio.lastMalformedWarning) < 10*time.Second {
		sio.stateMu.Unlock()
		return
	}
	count := sio.malformedSinceWarning
	sio.malformedSinceWarning = 0
	sio.lastMalformedWarning = now
	sio.stateMu.Unlock()

	logger.Warnw("Ignored malformed serial message",
		"messagesSinceLastWarning", count,
		"bytes", len(line),
		"expected", "legacy 0..1023 pipe-delimited frame or recognized mixr:v1 message")
}

func (sio *SerialIO) setConnectionStateLocked(state SerialConnectionState) {
	if sio.connectionState == state {
		return
	}
	sio.connectionState = state
	for _, consumer := range sio.statusConsumers {
		select {
		case consumer <- state:
		default:
			select {
			case <-consumer:
			default:
			}
			select {
			case consumer <- state:
			default:
			}
		}
	}
}
