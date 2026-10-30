//go:build windows
// +build windows

// Command windows-microphone-control directly exercises the current default
// recording endpoint without depending on serial input or playback sessions.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
	"time"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
	"github.com/omriharel/deej/internal/wincom"
)

const eventContextGUID = "{8033cff1-4c1b-4218-94bb-98599bc8d976}"

type endpointState struct {
	id     string
	volume float32
	muted  bool
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(operation string, arguments []string) error {
	// COM apartment initialization and every interface acquired from it must
	// remain on the same OS thread for the lifetime of this command.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := initializeCOM(); err != nil {
		return err
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator,
		&enumerator,
	); err != nil {
		return fmt.Errorf("create MMDevice enumerator: %w", err)
	}
	defer enumerator.Release()

	flags := flag.NewFlagSet("windows-microphone-control "+operation, flag.ContinueOnError)
	value := flags.Float64("value", 0, "absolute normalized volume in the range 0..1")
	interval := flags.Duration("interval", 500*time.Millisecond, "watch polling interval")
	samples := flags.Int("samples", 0, "watch samples before exit; 0 watches until interrupted")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}

	switch operation {
	case "current":
		state, err := readDefaultRecordingEndpoint(enumerator)
		if err != nil {
			return err
		}
		printState(state)
		return nil

	case "set-volume":
		if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1 {
			return fmt.Errorf("volume value must be between 0 and 1")
		}
		return mutateDefaultRecordingEndpoint(enumerator, func(volume *wca.IAudioEndpointVolume) error {
			return wincom.SetEndpointVolumeLevelScalar(volume, float32(*value), ole.NewGUID(eventContextGUID))
		})

	case "mute", "unmute", "toggle-mute":
		return mutateDefaultRecordingEndpoint(enumerator, func(volume *wca.IAudioEndpointVolume) error {
			muted := operation == "mute"
			if operation == "toggle-mute" {
				current, err := wincom.EndpointVolumeMute(volume)
				if err != nil {
					return err
				}
				muted = !current
			}
			return wincom.SetEndpointVolumeMute(volume, muted, ole.NewGUID(eventContextGUID))
		})

	case "watch":
		if *interval < 100*time.Millisecond {
			return fmt.Errorf("watch interval must be at least 100ms")
		}
		if *samples < 0 {
			return fmt.Errorf("watch samples cannot be negative")
		}
		return watchDefaultRecordingEndpoint(enumerator, *interval, *samples)

	default:
		usage()
		return fmt.Errorf("unsupported operation %q", operation)
	}
}

func initializeCOM() error {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err == nil {
		return nil
	}
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) && oleErr.Code() == 1 {
		return nil
	}
	return fmt.Errorf("initialize COM apartment: %w", err)
}

func readDefaultRecordingEndpoint(enumerator *wca.IMMDeviceEnumerator) (endpointState, error) {
	state := endpointState{}
	err := withDefaultRecordingEndpoint(enumerator, func(id string, volume *wca.IAudioEndpointVolume) error {
		level, err := wincom.EndpointVolumeLevelScalar(volume)
		if err != nil {
			return err
		}
		muted, err := wincom.EndpointVolumeMute(volume)
		if err != nil {
			return err
		}
		state = endpointState{id: id, volume: level, muted: muted}
		return nil
	})
	return state, err
}

func mutateDefaultRecordingEndpoint(
	enumerator *wca.IMMDeviceEnumerator,
	mutate func(*wca.IAudioEndpointVolume) error,
) error {
	if err := withDefaultRecordingEndpoint(enumerator, func(_ string, volume *wca.IAudioEndpointVolume) error {
		return mutate(volume)
	}); err != nil {
		return err
	}

	state, err := readDefaultRecordingEndpoint(enumerator)
	if err != nil {
		return fmt.Errorf("read recording endpoint back: %w", err)
	}
	printState(state)
	return nil
}

func withDefaultRecordingEndpoint(
	enumerator *wca.IMMDeviceEnumerator,
	use func(string, *wca.IAudioEndpointVolume) error,
) error {
	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &device); err != nil {
		return fmt.Errorf("default recording endpoint unavailable: %w", err)
	}
	defer device.Release()

	id, err := wincom.DeviceID(device)
	if err != nil {
		return fmt.Errorf("read recording endpoint ID: %w", err)
	}

	var volume *wca.IAudioEndpointVolume
	if err := wincom.Activate(device, wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &volume); err != nil {
		return fmt.Errorf("activate recording endpoint volume: %w", err)
	}
	defer volume.Release()

	return use(id, volume)
}

func watchDefaultRecordingEndpoint(enumerator *wca.IMMDeviceEnumerator, interval time.Duration, samples int) error {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var last endpointState
	var lastErr string
	haveLast := false
	for sample := 0; samples == 0 || sample < samples; sample++ {
		state, err := readDefaultRecordingEndpoint(enumerator)
		if err != nil {
			message := err.Error()
			if message != lastErr {
				fmt.Println("unavailable:", message)
				lastErr = message
				haveLast = false
			}
		} else {
			lastErr = ""
			if !haveLast || state.id != last.id || state.volume != last.volume || state.muted != last.muted {
				printState(state)
				last = state
				haveLast = true
			}
		}

		if samples != 0 && sample+1 >= samples {
			return nil
		}
		select {
		case <-ticker.C:
		case <-interrupt:
			return nil
		}
	}
	return nil
}

func printState(state endpointState) {
	fmt.Printf("endpoint-id: %s\nvolume: %.6f\nmuted: %t\n", state.id, state.volume, state.muted)
}

func usage() {
	fmt.Fprintln(os.Stderr, `Default Windows recording-endpoint diagnostic

Usage:
  windows-microphone-control current
  windows-microphone-control set-volume --value <0..1>
  windows-microphone-control mute
  windows-microphone-control unmute
  windows-microphone-control toggle-mute
  windows-microphone-control watch [--interval 500ms] [--samples 0]

Every command targets only the current EConsole default capture endpoint.
Mutations print state read back from Windows. Watch reacquires the default
capture endpoint on every poll and prints only endpoint, volume, mute, or
availability changes.`)
}
