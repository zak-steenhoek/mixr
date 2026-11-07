package mixr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNoTrayEnvironmentCompatibility(t *testing.T) {
	for _, variable := range []string{envNoTray, legacyEnvNoTray} {
		t.Run(variable, func(t *testing.T) {
			if err := os.Unsetenv(envNoTray); err != nil {
				t.Fatal(err)
			}
			if err := os.Unsetenv(legacyEnvNoTray); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Unsetenv(envNoTray)
				_ = os.Unsetenv(legacyEnvNoTray)
			})
			if err := os.Setenv(variable, "1"); err != nil {
				t.Fatal(err)
			}
			if !noTrayRequested() {
				t.Fatalf("%s did not disable the tray", variable)
			}
		})
	}
}

func TestLegacyConfigurationContract(t *testing.T) {
	tests := map[string]string{
		"slider mapping":              configKeySliderMapping,
		"invert sliders":              configKeyInvertSliders,
		"COM port":                    configKeyCOMPort,
		"baud rate":                   configKeyBaudRate,
		"noise reduction":             configKeyNoiseReductionLevel,
		"hardware mute enabled":       configKeyHardwareMuteEnabled,
		"hardware mute target":        configKeyHardwareMuteTarget,
		"five-channel momentary":      configKeyFiveChannelMomentary,
		"Chrome mute target":          configKeyChromeMuteTarget,
		"foreground mute target":      configKeyForegroundMuteTarget,
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
		"five-channel momentary":      "hardware_mute_prototype.five_channel_momentary",
		"Chrome mute target":          "hardware_mute_prototype.chrome_target",
		"foreground mute target":      "hardware_mute_prototype.foreground_target",
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
	config.userConfig.Set(configKeyFiveChannelMomentary, true)
	config.userConfig.Set(configKeyChromeMuteTarget, "chrome.exe")
	config.userConfig.Set(configKeyForegroundMuteTarget, "deej.current")
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
	if !config.HardwareMutePrototype.FiveChannelMomentary || config.HardwareMutePrototype.ChromeTarget != "chrome.exe" || config.HardwareMutePrototype.ForegroundTarget != "deej.current" {
		t.Fatalf("five-channel config = %#v", config.HardwareMutePrototype)
	}
	if config.HardwareMutePrototype.PollInterval != 500*time.Millisecond || config.HardwareMutePrototype.MicrophoneLevelDeadband != 12 {
		t.Fatalf("hardware timing config = %#v", config.HardwareMutePrototype)
	}
}

func TestInvalidConfigurationFallsBackWithActionableWarnings(t *testing.T) {
	config, err := NewConfig(zap.NewNop().Sugar(), nil)
	if err != nil {
		t.Fatal(err)
	}
	config.userConfig.Set(configKeySliderMapping, map[string][]string{
		"bad": {"chrome.exe"},
		"0":   {"master"},
		"1":   {"master", ""},
	})
	config.userConfig.Set(configKeyCOMPort, " ")
	config.userConfig.Set(configKeyBaudRate, -1)
	config.userConfig.Set(configKeyNoiseReductionLevel, "extreme")
	config.userConfig.Set(configKeyHardwareMuteTarget, "")
	config.userConfig.Set(configKeyChromeMuteTarget, "")
	config.userConfig.Set(configKeyForegroundMuteTarget, "")
	config.userConfig.Set(configKeyHardwareMutePollMS, 1)
	config.userConfig.Set(configKeyMicrophoneLevelDeadband, 1024)

	if err := config.populateFromVipers(); err != nil {
		t.Fatal(err)
	}
	if config.ConnectionInfo.COMPort != defaultCOMPort || config.ConnectionInfo.BaudRate != defaultBaudRate {
		t.Fatalf("connection fallback = %#v", config.ConnectionInfo)
	}
	if config.NoiseReductionLevel != defaultNoiseReductionLevel {
		t.Fatalf("noise reduction = %q", config.NoiseReductionLevel)
	}
	if config.HardwareMutePrototype.Target != defaultHardwareMuteTarget ||
		config.HardwareMutePrototype.ChromeTarget != defaultChromeMuteTarget ||
		config.HardwareMutePrototype.ForegroundTarget != defaultForegroundMuteTarget ||
		config.HardwareMutePrototype.PollInterval != defaultHardwareMutePollMS*time.Millisecond ||
		config.HardwareMutePrototype.MicrophoneLevelDeadband != defaultMicrophoneLevelDeadband {
		t.Fatalf("hardware fallback = %#v", config.HardwareMutePrototype)
	}
	if _, present := config.SliderMapping.get(0); !present {
		t.Fatal("valid slider mapping was discarded")
	}
	if len(config.ValidationWarnings) < 7 {
		t.Fatalf("validation warning count = %d, want at least 7: %q", len(config.ValidationWarnings), config.ValidationWarnings)
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

func TestValidatedRepositoryConfigLoads(t *testing.T) {
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(originalDirectory, "..", ".."))
	if err := os.Chdir(repositoryRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	config, err := NewConfig(zap.NewNop().Sugar(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Load(); err != nil {
		t.Fatalf("validated config.yaml did not load: %v", err)
	}
	if config.ConnectionInfo.COMPort != "COM4" || config.ConnectionInfo.BaudRate != 9600 {
		t.Fatalf("validated serial config changed: %#v", config.ConnectionInfo)
	}
	if len(config.ValidationWarnings) != 0 {
		t.Fatalf("repository config has validation warnings: %q", config.ValidationWarnings)
	}
	// These are the repository's D0 example targets, not restrictions on user
	// mappings. Foreground configuration and resolution are tested separately.
	wantTargets := []string{"master", "spotify.exe", "chrome.exe", "discord.exe"}
	mappingCount := 0
	config.SliderMapping.iterate(func(_ int, _ []string) { mappingCount++ })
	if mappingCount != len(wantTargets) {
		t.Fatalf("analog mapping count = %d, want %d (A0-A3 only)", mappingCount, len(wantTargets))
	}
	for index, want := range wantTargets {
		if targets, ok := config.SliderMapping.get(index); !ok || len(targets) != 1 || targets[0] != want {
			t.Errorf("A%d mapping = %q present=%t, want [%q]", index, targets, ok, want)
		}
	}
	if !config.HardwareMutePrototype.Enabled || !config.HardwareMutePrototype.FiveChannelMomentary || !config.HardwareMutePrototype.FollowSliderMapping {
		t.Fatalf("validated five-channel mute config changed: %#v", config.HardwareMutePrototype)
	}
	if config.HardwareMutePrototype.PollInterval != 500*time.Millisecond || config.HardwareMutePrototype.MicrophoneLevelInverted || config.HardwareMutePrototype.MicrophoneLevelDeadband != 12 {
		t.Fatalf("validated microphone/timing config changed: %#v", config.HardwareMutePrototype)
	}
	// Construct controls without starting serial/audio I/O. A4 must remain the
	// dedicated default microphone rather than a fifth slider_mapping entry.
	prototype := newHardwareMutePrototype(&Mixr{config: config, serial: &SerialIO{}}, zap.NewNop().Sugar())
	if _, present := config.SliderMapping.get(4); present {
		t.Fatal("A4 must remain separate from slider_mapping")
	}
	if len(prototype.controlOrder) != 5 || prototype.controls[prototypeMicrophoneMuteControl].target != inputSessionName {
		t.Fatal("five-channel controls must retain the separate default microphone")
	}
}
