// Experimental Track B1 hardware mute proof of concept.
//
// Legacy compatibility: four potentiometer readings are still emitted as the
// original newline-delimited 0..1023 pipe-separated frame at 9600 baud.

const int NUM_SLIDERS = 4;
const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3};
const int switchPin = 2;
const int indicatorPin = 3;

const unsigned long sliderIntervalMs = 10;
const unsigned long debounceMs = 35;

const char helloMessage[] = "mixr:v1:hello";
const char readyMessage[] = "mixr:v1:ready";
const char indicatorOffMessage[] = "mixr:v1:state:indicator:spotify-mute:0";
const char indicatorOnMessage[] = "mixr:v1:state:indicator:spotify-mute:1";

int analogSliderValues[NUM_SLIDERS];
int lastSwitchReading = HIGH;
int stableSwitchState = HIGH;
unsigned long lastSwitchChangeMs = 0;
unsigned long lastSliderSendMs = 0;
bool protocolReady = false;
bool switchEventsArmed = false;

char receiveBuffer[96];
size_t receiveLength = 0;

void setup() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    pinMode(analogInputs[i], INPUT);
  }
  pinMode(switchPin, INPUT_PULLUP);
  pinMode(indicatorPin, OUTPUT);
  digitalWrite(indicatorPin, LOW);

  baselineSwitchWithoutEvent();
  Serial.begin(9600);
}

void loop() {
  receiveHostMessages();
  updateDebouncedSwitch();

  unsigned long now = millis();
  if (now - lastSliderSendMs >= sliderIntervalMs) {
    lastSliderSendMs = now;
    updateSliderValues();
    sendSliderValues();
  }
}

void baselineSwitchWithoutEvent() {
  int current = digitalRead(switchPin);
  lastSwitchReading = current;
  stableSwitchState = current;
  lastSwitchChangeMs = millis();
  switchEventsArmed = false;
}

void updateDebouncedSwitch() {
  if (!protocolReady) {
    return;
  }

  int reading = digitalRead(switchPin);
  unsigned long now = millis();
  if (reading != lastSwitchReading) {
    lastSwitchReading = reading;
    lastSwitchChangeMs = now;
  }
  if (now - lastSwitchChangeMs < debounceMs) {
    return;
  }

  if (!switchEventsArmed) {
    stableSwitchState = reading;
    switchEventsArmed = true;
    return;
  }
  if (reading == stableSwitchState) {
    return;
  }

  stableSwitchState = reading;
  Serial.print("mixr:v1:event:switch:spotify-mute:");
  Serial.println(stableSwitchState == LOW ? "1" : "0");
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
    // Re-baseline on every host connection. The physical state at connection
    // time is not an action and must never toggle Windows mute.
    baselineSwitchWithoutEvent();
    protocolReady = true;
    Serial.println(readyMessage);
    return;
  }
  if (strcmp(message, indicatorOffMessage) == 0) {
    digitalWrite(indicatorPin, LOW);
    return;
  }
  if (strcmp(message, indicatorOnMessage) == 0) {
    digitalWrite(indicatorPin, HIGH);
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
