#include <Keypad.h>
/*
https://eleccelerator.com/fusecalc/fusecalc.php?chip=attiny84

avrdude -P /dev/ttyACM0 -b 19200 -c avrisp -p attiny84 -C /.../avrdude.conf -v
Default fuses (before disabling reset pin): 
  avrdude: safemode: lfuse reads as 62
  avrdude: safemode: hfuse reads as D7
  avrdude: safemode: efuse reads as FF
  avrdude: safemode: Fuses OK (E:FF, H:D7, L:62)

avrdude -P /dev/ttyACM0 -b 19200 -c avrisp -p attiny84 -U lfuse:w:0x62:m -U efuse:w:0xFF:m -U hfuse:w:0x57:m -C /.../avrdude.conf -v

*/
const byte ROWS = 4;
const byte COLS = 3;
char keys[ROWS][COLS] = {
  {'1', '2', '3'},
  {'4', '5', '6'},
  {'7', '8', '9'},
  {'#', '0', '*'}
};
byte rowPins[ROWS] = {5, 4, 3, 2};  //connect to the row pinouts of the keypad
byte colPins[COLS] = {7, 6, 8};     //connect to the column pinouts of the keypad
byte outputPins[4] = {0, 1, 9, 10}; // Output pins (least significant bit on first pin in array}
byte selectPin = 11;                // Pin to pull high for readTime time to allow for reading
int readTime = 2;                   // Time in milliseconds during which transfer is allowed

Keypad keypad = Keypad( makeKeymap(keys), rowPins, colPins, ROWS, COLS );

void setup() {
  for (byte p = 0; p < 4; p++) {
    pinMode(outputPins[p], OUTPUT);
    digitalWrite(outputPins[p], LOW);
  }
  pinMode(selectPin, OUTPUT);
  digitalWrite(selectPin, LOW);
}

void loop() {
  char key = keypad.getKey();

  if (key != NO_KEY) {
    byte  nr = 0;
    if (key == '*') {
      nr = 10;
    } else if (key == '#') {
      nr = 11;
    } else {
      nr = key - '0';
    }
    setBits(nr);
  }
}

void setBits(byte nr) {
  // "Write" output to output pins...
  for (byte p = 0; p < 4; p++){
    digitalWrite(outputPins[p], bitRead(nr, p));
  }
  // Now toggle selectPin for readTime milliseconds
  digitalWrite(selectPin, HIGH);  
  delay(readTime);
  digitalWrite(selectPin, LOW);    
}
