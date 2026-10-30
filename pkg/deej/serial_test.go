package deej

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"
)

func TestSerialLineParsingKeepsLegacyFramesSeparate(t *testing.T) {
	config := &CanonicalConfig{}
	config.NoiseReductionLevel = "default"
	config.HardwareMutePrototype.Enabled = true
	deej := &Deej{config: config}
	sliderEvents := make(chan SliderMoveEvent, 8)
	prototypeEvents := make(chan PrototypeSerialEvent, 8)
	sio := &SerialIO{
		deej:                deej,
		logger:              zap.NewNop().Sugar(),
		lastIndicatorStates: make(map[string]bool),
		sliderMoveConsumers: []chan SliderMoveEvent{sliderEvents},
		prototypeConsumers:  []chan PrototypeSerialEvent{prototypeEvents},
		prototypeReady:      false,
		prototypeEnabled:    true,
	}

	sio.handleLine(sio.logger, "mixr:v1:event:switch:spotify-mute:1\r\n")
	if len(prototypeEvents) != 0 {
		t.Fatal("pre-ready switch event was delivered")
	}

	sio.handleLine(sio.logger, "mixr:v1:ready\r\n")
	sio.handleLine(sio.logger, "0|256|512|1023\r\n")
	sio.handleLine(sio.logger, "mixr:v1:event:switch:spotify-mute:1\r\n")

	if len(sliderEvents) != 4 {
		t.Fatalf("legacy slider event count = %d, want 4", len(sliderEvents))
	}
	if len(prototypeEvents) != 2 {
		t.Fatalf("prototype event count = %d, want ready + switch", len(prototypeEvents))
	}
	ready := <-prototypeEvents
	switchEvent := <-prototypeEvents
	if ready.Kind != PrototypeSerialReady {
		t.Fatalf("first prototype event = %#v, want ready", ready)
	}
	if switchEvent.Kind != PrototypeSerialSwitchEvent || !switchEvent.SwitchClosed {
		t.Fatalf("second prototype event = %#v, want closed switch", switchEvent)
	}
}

func TestPrototypeIndicatorSuppressesDuplicateWrites(t *testing.T) {
	conn := &recordingSerialConn{}
	sio := &SerialIO{
		logger:               zap.NewNop().Sugar(),
		connected:            true,
		prototypeReady:       true,
		lifecycle:            1,
		connectionGeneration: 1,
		conn:                 conn,
		lastIndicatorStates:  make(map[string]bool),
	}

	if err := sio.setPrototypeIndicator(prototypeSpotifyMuteControl, false); err != nil {
		t.Fatal(err)
	}
	if err := sio.setPrototypeIndicator(prototypeSpotifyMuteControl, false); err != nil {
		t.Fatal(err)
	}
	if err := sio.setPrototypeIndicator(prototypeSpotifyMuteControl, true); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"mixr:v1:state:indicator:spotify-mute:0\n",
		"mixr:v1:state:indicator:spotify-mute:1\n",
	}
	if len(conn.writes) != len(want) {
		t.Fatalf("write count = %d, want %d (%q)", len(conn.writes), len(want), conn.writes)
	}
	for index := range want {
		if conn.writes[index] != want[index] {
			t.Fatalf("write %d = %q, want %q", index, conn.writes[index], want[index])
		}
	}
}

func TestSerialReconnectsAfterEstablishedConnectionLoss(t *testing.T) {
	config := &CanonicalConfig{}
	config.ConnectionInfo.COMPort = "TEST"
	config.ConnectionInfo.BaudRate = 9600
	deej := &Deej{config: config}
	first := newControlledSerialConn()
	second := newControlledSerialConn()
	var openerMu sync.Mutex
	openCount := 0
	opener := func(serial.OpenOptions) (io.ReadWriteCloser, error) {
		openerMu.Lock()
		defer openerMu.Unlock()
		openCount++
		if openCount == 1 {
			return first, nil
		}
		return second, nil
	}
	sio := &SerialIO{
		deej:                deej,
		logger:              zap.NewNop().Sugar(),
		lastIndicatorStates: make(map[string]bool),
		openPort:            opener,
	}

	if err := sio.Start(); err != nil {
		t.Fatal(err)
	}
	sio.stateMu.Lock()
	sio.prototypeReady = true
	sio.lastIndicatorStates[prototypeSpotifyMuteControl] = true
	sio.stateMu.Unlock()
	first.fail(io.ErrUnexpectedEOF)
	deadline := time.Now().Add(2 * time.Second)
	for {
		sio.stateMu.RLock()
		generation := sio.connectionGeneration
		connected := sio.connected
		sio.stateMu.RUnlock()
		sio.stateMu.RLock()
		ready := sio.prototypeReady
		indicatorCount := len(sio.lastIndicatorStates)
		sio.stateMu.RUnlock()
		if connected && generation == 2 {
			if ready || indicatorCount != 0 {
				t.Fatalf("reconnect retained prototype state: ready=%t indicators=%d", ready, indicatorCount)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("serial did not reconnect: connected=%t generation=%d", connected, generation)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sio.Stop()
	openerMu.Lock()
	gotOpenCount := openCount
	openerMu.Unlock()
	if gotOpenCount != 2 {
		t.Fatalf("open count = %d, want initial + one reconnect", gotOpenCount)
	}
}

type recordingSerialConn struct {
	writes []string
}

func (c *recordingSerialConn) Read([]byte) (int, error) { return 0, io.EOF }
func (c *recordingSerialConn) Write(data []byte) (int, error) {
	c.writes = append(c.writes, string(data))
	return len(data), nil
}
func (c *recordingSerialConn) Close() error { return nil }

var _ io.ReadWriteCloser = (*recordingSerialConn)(nil)

type controlledSerialConn struct {
	readResult chan error
	closeOnce  sync.Once
}

func newControlledSerialConn() *controlledSerialConn {
	return &controlledSerialConn{readResult: make(chan error, 1)}
}

func (c *controlledSerialConn) Read([]byte) (int, error) { return 0, <-c.readResult }
func (c *controlledSerialConn) Write(data []byte) (int, error) {
	return len(data), nil
}
func (c *controlledSerialConn) Close() error {
	c.closeOnce.Do(func() { c.fail(io.EOF) })
	return nil
}
func (c *controlledSerialConn) fail(err error) {
	select {
	case c.readResult <- err:
	default:
	}
}

var _ io.ReadWriteCloser = (*controlledSerialConn)(nil)
