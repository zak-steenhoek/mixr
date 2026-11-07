// Experimental Track D0 five-channel momentary-mute prototype for Nano R4.
// A0-A3 retain the legacy frame. A4 is the additive microphone-level event.

const int NUM_SLIDERS = 4;
const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3};
const int microphoneLevelPin = A4;

const int NUM_MUTE_CONTROLS = 5;
const int buttonPins[NUM_MUTE_CONTROLS] = {2, 4, 6, 8, 10};
const int indicatorPins[NUM_MUTE_CONTROLS] = {3, 5, 7, 9, 11};
const char *const controlIDs[NUM_MUTE_CONTROLS] = {
  "master-mute",
  "spotify-mute",
  "chrome-mute",
  "foreground-mute",
  "microphone-mute"
};

const unsigned long sliderIntervalMs = 10;
const unsigned long microphoneSampleIntervalMs = 200;
const unsigned long microphoneHeartbeatMs = 1000;
const unsigned long debounceMs = 35;
const int microphoneFirmwareDeadband = 4;

const char helloMessage[] = "mixr:v1:hello";
const char readyMessage[] = "mixr:v1:ready";

int analogSliderValues[NUM_SLIDERS];
int lastButtonReadings[NUM_MUTE_CONTROLS] = {HIGH, HIGH, HIGH, HIGH, HIGH};
int stableButtonStates[NUM_MUTE_CONTROLS] = {HIGH, HIGH, HIGH, HIGH, HIGH};
unsigned long lastButtonChangeMs[NUM_MUTE_CONTROLS] = {0, 0, 0, 0, 0};
bool buttonEventsArmed[NUM_MUTE_CONTROLS] = {false, false, false, false, false};

unsigned long lastSliderSendMs = 0;
unsigned long lastMicrophoneSampleMs = 0;
unsigned long lastMicrophoneSendMs = 0;
int lastMicrophoneValueSent = -1;
bool protocolReady = false;

char receiveBuffer[96];
size_t receiveLength = 0;

void setup() {
  for (int i = 0; i < NUM_SLIDERS; i++) pinMode(analogInputs[i], INPUT);
  pinMode(microphoneLevelPin, INPUT);
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    pinMode(buttonPins[i], INPUT_PULLUP);
    pinMode(indicatorPins[i], OUTPUT);
    digitalWrite(indicatorPins[i], LOW);
  }
  baselineButtonsWithoutEvents();
  Serial.begin(9600);
}

void loop() {
  receiveHostMessages();
  updateDebouncedButtons();

  unsigned long now = millis();
  if (now - lastSliderSendMs >= sliderIntervalMs) {
    lastSliderSendMs = now;
    sendLegacySliderValues();
  }
  if (protocolReady && now - lastMicrophoneSampleMs >= microphoneSampleIntervalMs) {
    lastMicrophoneSampleMs = now;
    sendMicrophoneLevelIfNeeded(now);
  }
}

void baselineButtonsWithoutEvents() {
  unsigned long now = millis();
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    int current = digitalRead(buttonPins[i]);
    lastButtonReadings[i] = current;
    stableButtonStates[i] = current;
    lastButtonChangeMs[i] = now;
    buttonEventsArmed[i] = false;
  }
}

void updateDebouncedButtons() {
  if (!protocolReady) return;
  unsigned long now = millis();
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    int reading = digitalRead(buttonPins[i]);
    if (reading != lastButtonReadings[i]) {
      lastButtonReadings[i] = reading;
      lastButtonChangeMs[i] = now;
    }
    if (now - lastButtonChangeMs[i] < debounceMs) continue;
    if (!buttonEventsArmed[i]) {
      stableButtonStates[i] = reading;
      buttonEventsArmed[i] = true;
      continue;
    }
    if (reading == stableButtonStates[i]) continue;

    stableButtonStates[i] = reading;
    Serial.print("mixr:v1:event:button:");
    Serial.print(controlIDs[i]);
    Serial.print(":");
    Serial.println(reading == LOW ? "1" : "0");
  }
}

void sendMicrophoneLevelIfNeeded(unsigned long now) {
  int value = analogRead(microphoneLevelPin);
  bool moved = lastMicrophoneValueSent < 0 || abs(value - lastMicrophoneValueSent) >= microphoneFirmwareDeadband;
  bool heartbeatDue = now - lastMicrophoneSendMs >= microphoneHeartbeatMs;
  if (!moved && !heartbeatDue) return;

  Serial.print("mixr:v1:event:analog:microphone-level:");
  Serial.println(value);
  lastMicrophoneValueSent = value;
  lastMicrophoneSendMs = now;
}

void receiveHostMessages() {
  while (Serial.available() > 0) {
    char next = (char)Serial.read();
    if (next == '\r') continue;
    if (next == '\n') {
      receiveBuffer[receiveLength] = '\0';
      handleHostMessage(receiveBuffer);
      receiveLength = 0;
      continue;
    }
    if (receiveLength < sizeof(receiveBuffer) - 1) {
      receiveBuffer[receiveLength++] = next;
    } else {
      receiveLength = 0;
    }
  }
}

void handleHostMessage(const char *message) {
  if (strcmp(message, helloMessage) == 0) {
    protocolReady = false;
    baselineButtonsWithoutEvents();
    lastMicrophoneValueSent = -1;
    protocolReady = true;
    Serial.println(readyMessage);
    return;
  }
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    char expected[64];
    snprintf(expected, sizeof(expected), "mixr:v1:state:indicator:%s:0", controlIDs[i]);
    if (strcmp(message, expected) == 0) {
      digitalWrite(indicatorPins[i], LOW);
      return;
    }
    snprintf(expected, sizeof(expected), "mixr:v1:state:indicator:%s:1", controlIDs[i]);
    if (strcmp(message, expected) == 0) {
      digitalWrite(indicatorPins[i], HIGH);
      return;
    }
  }
}

void sendLegacySliderValues() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    analogSliderValues[i] = analogRead(analogInputs[i]);
    Serial.print(analogSliderValues[i]);
    if (i < NUM_SLIDERS - 1) Serial.print("|");
  }
  Serial.println();
}
