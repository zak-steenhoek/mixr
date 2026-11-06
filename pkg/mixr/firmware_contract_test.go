package mixr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFiveChannelFirmwareContract(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	sketchPath := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "arduino", "mixr-five-channel-momentary-mute", "mixr-five-channel-momentary-mute.ino"))
	contents, err := os.ReadFile(sketchPath)
	if err != nil {
		t.Fatalf("read five-channel firmware: %v", err)
	}
	sketch := strings.ReplaceAll(string(contents), "\r\n", "\n")

	required := []string{
		"const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3};",
		"const int microphoneLevelPin = A4;",
		"const int buttonPins[NUM_MUTE_CONTROLS] = {2, 4, 6, 8, 10};",
		"const int indicatorPins[NUM_MUTE_CONTROLS] = {3, 5, 7, 9, 11};",
		"\"master-mute\",\n  \"spotify-mute\",\n  \"chrome-mute\",\n  \"foreground-mute\",\n  \"microphone-mute\"",
		"const unsigned long debounceMs = 35;",
		"pinMode(buttonPins[i], INPUT_PULLUP);",
		"Serial.print(\"mixr:v1:event:button:\");",
		"Serial.println(reading == LOW ? \"1\" : \"0\");",
		"baselineButtonsWithoutEvents();",
		"Serial.println(readyMessage);",
		"mixr:v1:event:analog:microphone-level:",
		"mixr:v1:state:indicator:%s:0",
		"mixr:v1:state:indicator:%s:1",
	}
	for _, fragment := range required {
		if !strings.Contains(sketch, fragment) {
			t.Errorf("firmware contract missing %q", fragment)
		}
	}
	if strings.Count(sketch, "stableButtonStates[i] = reading;") != 2 {
		t.Error("firmware must baseline once and update once per distinct stable edge")
	}
	if !strings.Contains(sketch, "if (reading == stableButtonStates[i]) continue;") {
		t.Error("firmware lacks held-button duplicate suppression")
	}
}
