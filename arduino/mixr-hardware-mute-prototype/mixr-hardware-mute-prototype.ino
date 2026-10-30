// Experimental Track B dual hardware mute proof of concept.
//
// Legacy compatibility: four potentiometer readings are still emitted as the
// original newline-delimited 0..1023 pipe-separated frame at 9600 baud. A4 is
// intentionally unused and reserved for the microphone slider experiment.

const int NUM_SLIDERS = 4;
const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3};

const int NUM_MUTE_CONTROLS = 2;
const int switchPins[NUM_MUTE_CONTROLS] = {2, 4};
const int indicatorPins[NUM_MUTE_CONTROLS] = {3, 5};
const char *const controlIDs[NUM_MUTE_CONTROLS] = {
  "spotify-mute",
  "master-mute"
};

const unsigned long sliderIntervalMs = 10;
const unsigned long debounceMs = 35;

const char helloMessage[] = "mixr:v1:hello";
const char readyMessage[] = "mixr:v1:ready";
const char spotifyIndicatorOffMessage[] = "mixr:v1:state:indicator:spotify-mute:0";
const char spotifyIndicatorOnMessage[] = "mixr:v1:state:indicator:spotify-mute:1";
const char masterIndicatorOffMessage[] = "mixr:v1:state:indicator:master-mute:0";
const char masterIndicatorOnMessage[] = "mixr:v1:state:indicator:master-mute:1";

int analogSliderValues[NUM_SLIDERS];
int lastSwitchReadings[NUM_MUTE_CONTROLS] = {HIGH, HIGH};
int stableSwitchStates[NUM_MUTE_CONTROLS] = {HIGH, HIGH};
unsigned long lastSwitchChangeMs[NUM_MUTE_CONTROLS] = {0, 0};
bool switchEventsArmed[NUM_MUTE_CONTROLS] = {false, false};

unsigned long lastSliderSendMs = 0;
bool protocolReady = false;

char receiveBuffer[96];
size_t receiveLength = 0;

void setup() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    pinMode(analogInputs[i], INPUT);
  }
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    pinMode(switchPins[i], INPUT_PULLUP);
    pinMode(indicatorPins[i], OUTPUT);
    digitalWrite(indicatorPins[i], LOW);
  }

  baselineSwitchesWithoutEvents();
  Serial.begin(9600);
}

void loop() {
  receiveHostMessages();
  updateDebouncedSwitches();

  unsigned long now = millis();
  if (now - lastSliderSendMs >= sliderIntervalMs) {
    lastSliderSendMs = now;
    updateSliderValues();
    sendSliderValues();
  }
}

void baselineSwitchesWithoutEvents() {
  unsigned long now = millis();
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    int current = digitalRead(switchPins[i]);
    lastSwitchReadings[i] = current;
    stableSwitchStates[i] = current;
    lastSwitchChangeMs[i] = now;
    switchEventsArmed[i] = false;
  }
}

void updateDebouncedSwitches() {
  if (!protocolReady) {
    return;
  }

  unsigned long now = millis();
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    int reading = digitalRead(switchPins[i]);
    if (reading != lastSwitchReadings[i]) {
      lastSwitchReadings[i] = reading;
      lastSwitchChangeMs[i] = now;
    }
    if (now - lastSwitchChangeMs[i] < debounceMs) {
      continue;
    }

    if (!switchEventsArmed[i]) {
      stableSwitchStates[i] = reading;
      switchEventsArmed[i] = true;
      continue;
    }
    if (reading == stableSwitchStates[i]) {
      continue;
    }

    stableSwitchStates[i] = reading;
    Serial.print("mixr:v1:event:switch:");
    Serial.print(controlIDs[i]);
    Serial.print(":");
    Serial.println(stableSwitchStates[i] == LOW ? "1" : "0");
  }
}

void receiveHostMessages() {
  while (Serial.available() > 0) {
    char next = (char)Serial.read();
    if (next == '\r') {
      continue;
    }
    if (next == '\n') {
      receiveBuffer[receiveLength] = '\0';
      handleHostMessage(receiveBuffer);
      receiveLength = 0;
      continue;
    }
    if (receiveLength < sizeof(receiveBuffer) - 1) {
      receiveBuffer[receiveLength++] = next;
    } else {
      // Drop an oversized/unknown frame without interpreting a suffix.
      receiveLength = 0;
    }
  }
}

void handleHostMessage(const char *message) {
  if (strcmp(message, helloMessage) == 0) {
    // Re-baseline both switches on every host connection. Physical state at
    // connection time is not an action and must never toggle Windows mute.
    protocolReady = false;
    baselineSwitchesWithoutEvents();
    protocolReady = true;
    Serial.println(readyMessage);
    return;
  }
  if (strcmp(message, spotifyIndicatorOffMessage) == 0) {
    digitalWrite(indicatorPins[0], LOW);
    return;
  }
  if (strcmp(message, spotifyIndicatorOnMessage) == 0) {
    digitalWrite(indicatorPins[0], HIGH);
    return;
  }
  if (strcmp(message, masterIndicatorOffMessage) == 0) {
    digitalWrite(indicatorPins[1], LOW);
    return;
  }
  if (strcmp(message, masterIndicatorOnMessage) == 0) {
    digitalWrite(indicatorPins[1], HIGH);
  }
}

void updateSliderValues() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    analogSliderValues[i] = analogRead(analogInputs[i]);
  }
}

void sendSliderValues() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    Serial.print(analogSliderValues[i]);
    if (i < NUM_SLIDERS - 1) {
      Serial.print("|");
    }
  }
  Serial.println();
}
