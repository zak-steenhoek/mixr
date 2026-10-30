// Experimental Track B2 microphone controls prototype.
// Four legacy A0-A3 fields remain unchanged; A4 uses an additive v1 message.

const int NUM_SLIDERS = 4;
const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3};
const int microphoneLevelPin = A4;

const int NUM_MUTE_CONTROLS = 3;
const int switchPins[NUM_MUTE_CONTROLS] = {2, 4, 6};
const int indicatorPins[NUM_MUTE_CONTROLS] = {3, 5, 7};
const char *const controlIDs[NUM_MUTE_CONTROLS] = {
  "master-mute",
  "spotify-mute",
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
int lastSwitchReadings[NUM_MUTE_CONTROLS] = {HIGH, HIGH, HIGH};
int stableSwitchStates[NUM_MUTE_CONTROLS] = {HIGH, HIGH, HIGH};
unsigned long lastSwitchChangeMs[NUM_MUTE_CONTROLS] = {0, 0, 0};
bool switchEventsArmed[NUM_MUTE_CONTROLS] = {false, false, false};

unsigned long lastSliderSendMs = 0;
unsigned long lastMicrophoneSampleMs = 0;
unsigned long lastMicrophoneSendMs = 0;
int lastMicrophoneValueSent = -1;
bool protocolReady = false;

char receiveBuffer[96];
size_t receiveLength = 0;

void setup() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    pinMode(analogInputs[i], INPUT);
  }
  pinMode(microphoneLevelPin, INPUT);
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
    sendLegacySliderValues();
  }
  if (protocolReady && now - lastMicrophoneSampleMs >= microphoneSampleIntervalMs) {
    lastMicrophoneSampleMs = now;
    sendMicrophoneLevelIfNeeded(now);
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
  if (!protocolReady) return;
  unsigned long now = millis();
  for (int i = 0; i < NUM_MUTE_CONTROLS; i++) {
    int reading = digitalRead(switchPins[i]);
    if (reading != lastSwitchReadings[i]) {
      lastSwitchReadings[i] = reading;
      lastSwitchChangeMs[i] = now;
    }
    if (now - lastSwitchChangeMs[i] < debounceMs) continue;
    if (!switchEventsArmed[i]) {
      stableSwitchStates[i] = reading;
      switchEventsArmed[i] = true;
      continue;
    }
    if (reading == stableSwitchStates[i]) continue;

    stableSwitchStates[i] = reading;
    Serial.print("mixr:v1:event:switch:");
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
    baselineSwitchesWithoutEvents();
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
