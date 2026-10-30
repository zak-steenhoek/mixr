package deej

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLegacyConfigurationContract(t *testing.T) {
	tests := map[string]string{
		"slider mapping":              configKeySliderMapping,
		"invert sliders":              configKeyInvertSliders,
		"COM port":                    configKeyCOMPort,
		"baud rate":                   configKeyBaudRate,
		"noise reduction":             configKeyNoiseReductionLevel,
		"hardware mute enabled":       configKeyHardwareMuteEnabled,
		"hardware mute target":        configKeyHardwareMuteTarget,
		"hardware mute poll interval": configKeyHardwareMutePollMS,
		"microphone inversion":        configKeyMicrophoneLevelInverted,
		"microphone deadband":         configKeyMicrophoneLevelDeadband,
	}
	want := map[string]string{
		"slider mapping":              "slider_mapping",
		"invert sliders":              "invert_sliders",
		"COM port":                    "com_port",
		"baud rate":                   "baud_rate",
		"noise reduction":             "noise_reduction",
		"hardware mute enabled":       "hardware_mute_prototype.enabled",
		"hardware mute target":        "hardware_mute_prototype.target",
		"hardware mute poll interval": "hardware_mute_prototype.poll_interval_ms",
		"microphone inversion":        "hardware_mute_prototype.microphone_level_inverted",
		"microphone deadband":         "hardware_mute_prototype.microphone_level_deadband",
	}
	for name, got := range tests {
		if got != want[name] {
			t.Errorf("%s key = %q, want %q", name, got, want[name])
		}
	}
	if userConfigFilepath != "config.yaml" {
		t.Errorf("user config path = %q, want config.yaml", userConfigFilepath)
	}
	if defaultBaudRate != 9600 {
		t.Errorf("default baud rate = %d, want 9600", defaultBaudRate)
	}
}

func TestLegacySpecialTargetNamesRemainAccepted(t *testing.T) {
	if specialTargetTransformPrefix != "deej." {
		t.Fatalf("special target prefix = %q, want deej.", specialTargetTransformPrefix)
	}

	sessions := []Session{
		&fakeCommandSession{key: "chrome.exe"},
		&fakeCommandSession{key: "spotify.exe"},
	}
	got := resolveAudioTargetNames("DEEJ.UNMAPPED", sessions)
	if len(got) != 2 || got[0] != "chrome.exe" || got[1] != "spotify.exe" {
		t.Fatalf("deej.unmapped resolved to %q", got)
	}
}

func TestExistingConfigurationValuesPopulateUnchanged(t *testing.T) {
	config, err := NewConfig(zap.NewNop().Sugar(), nil)
	if err != nil {
		t.Fatal(err)
	}
	config.userConfig.Set(configKeySliderMapping, map[string][]string{
		"0": {"master"},
		"1": {"spotify.exe"},
		"2": {"chrome.exe"},
		"3": {"deej.current"},
	})
	config.userConfig.Set(configKeyCOMPort, "COM4")
	config.userConfig.Set(configKeyBaudRate, 9600)
	config.userConfig.Set(configKeyNoiseReductionLevel, "default")
	config.userConfig.Set(configKeyHardwareMuteEnabled, true)
	config.userConfig.Set(configKeyHardwareMuteTarget, "spotify.exe")
	config.userConfig.Set(configKeyHardwareMutePollMS, 500)
	config.userConfig.Set(configKeyMicrophoneLevelInverted, false)
	config.userConfig.Set(configKeyMicrophoneLevelDeadband, 12)

	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	if got, ok := config.SliderMapping.get(3); !ok || len(got) != 1 || got[0] != "deej.current" {
		t.Fatalf("foreground mapping = %q, present=%t", got, ok)
	}
	if config.ConnectionInfo.COMPort != "COM4" || config.ConnectionInfo.BaudRate != 9600 {
		t.Fatalf("connection info = %#v", config.ConnectionInfo)
	}
	if !config.HardwareMutePrototype.Enabled || config.HardwareMutePrototype.Target != "spotify.exe" {
		t.Fatalf("hardware mute config = %#v", config.HardwareMutePrototype)
	}
	if config.HardwareMutePrototype.PollInterval != 500*time.Millisecond || config.HardwareMutePrototype.MicrophoneLevelDeadband != 12 {
		t.Fatalf("hardware timing config = %#v", config.HardwareMutePrototype)
	}
}

func TestValidatedSerialProtocolIdentity(t *testing.T) {
	if prototypeProtocolPrefix != "mixr:v1:" {
		t.Fatalf("protocol prefix = %q, want mixr:v1:", prototypeProtocolPrefix)
	}
	if prototypeHelloMessage != "mixr:v1:hello" {
		t.Fatalf("hello message = %q", prototypeHelloMessage)
	}
}
