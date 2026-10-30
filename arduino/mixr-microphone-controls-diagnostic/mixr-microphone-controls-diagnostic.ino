// Standalone Track B2 hardware diagnostic for the Nano R4 prototype.
//
// This sketch intentionally does not implement the mixr host protocol. Each
// LED follows its paired switch only so the wiring can be checked before any
// Windows audio integration is attempted.

const int NUM_CONTROLS = 3;

const int switchPins[NUM_CONTROLS] = {2, 4, 6};
const int indicatorPins[NUM_CONTROLS] = {3, 5, 7};
const char *const controlIDs[NUM_CONTROLS] = {
  "master-mute",
  "spotify-mute",
  "microphone-mute"
};

const int microphoneLevelPin = A4;

const unsigned long debounceMs = 35;
const unsigned long microphoneReportIntervalMs = 200;

int lastSwitchReadings[NUM_CONTROLS];
int stableSwitchStates[NUM_CONTROLS];
unsigned long lastSwitchChangeMs[NUM_CONTROLS];
unsigned long lastMicrophoneReportMs = 0;

void setup() {
  Serial.begin(9600);
  pinMode(microphoneLevelPin, INPUT);

  unsigned long now = millis();
  for (int i = 0; i < NUM_CONTROLS; i++) {
    pinMode(switchPins[i], INPUT_PULLUP);
    pinMode(indicatorPins[i], OUTPUT);

    int initialState = digitalRead(switchPins[i]);
    lastSwitchReadings[i] = initialState;
    stableSwitchStates[i] = initialState;
    lastSwitchChangeMs[i] = now;
    setDiagnosticIndicator(i, initialState);
  }

  Serial.println("diag:mixr-microphone-controls:ready");
  for (int i = 0; i < NUM_CONTROLS; i++) {
    reportSwitchState(i);
  }
}

void loop() {
  updateDebouncedSwitches();
  reportMicrophoneLevelAtBoundedRate();
}

void updateDebouncedSwitches() {
  unsigned long now = millis();

  for (int i = 0; i < NUM_CONTROLS; i++) {
    int reading = digitalRead(switchPins[i]);
    if (reading != lastSwitchReadings[i]) {
      lastSwitchReadings[i] = reading;
      lastSwitchChangeMs[i] = now;
    }

    if (now - lastSwitchChangeMs[i] < debounceMs) {
      continue;
    }
    if (reading == stableSwitchStates[i]) {
      continue;
    }

    stableSwitchStates[i] = reading;
    setDiagnosticIndicator(i, reading);
    reportSwitchState(i);
  }
}

void setDiagnosticIndicator(int controlIndex, int switchState) {
  // INPUT_PULLUP reads LOW when the switch is closed to GND. For this
  // diagnostic only, the paired LED is on while that switch is closed.
  digitalWrite(indicatorPins[controlIndex], switchState == LOW ? HIGH : LOW);
}

void reportSwitchState(int controlIndex) {
  Serial.print("diag:switch:");
  Serial.print(controlIDs[controlIndex]);
  Serial.print(":");
  Serial.println(stableSwitchStates[controlIndex] == LOW ? "closed" : "open");
}

void reportMicrophoneLevelAtBoundedRate() {
  unsigned long now = millis();
  if (now - lastMicrophoneReportMs < microphoneReportIntervalMs) {
    return;
  }

  lastMicrophoneReportMs = now;
  Serial.print("diag:a4:");
  Serial.println(analogRead(microphoneLevelPin));
}
