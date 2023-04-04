#define RAMLEN 1024
#define ROMLEN {ROM_LENGTH}

const char ADDR[] = {52, 50, 48, 46, 44, 42, 40, 38, 36, 34, 32, 30, 28, 26, 24, 22};
const char DATA[] = {53, 51, 49, 47, 45, 43, 41, 39};
const char MEMORY[] = {{MEMORY_ARRAY}};
char RAM[RAMLEN];

#define RW 37
#define CLOCK 2

void onClock() {
  char output[15];
  unsigned int address = 0;
  for (int n = 0; n < 16; n ++) {
    int bit = digitalRead(ADDR[n]) ? 1 : 0;
    address = address + (bit << n);
  }

	bool log_debug = {LOG_DEBUG};
	int reading = digitalRead(RW);
	char rwc = reading ? 'R' : 'W';
  unsigned char data = 0;

	bool with_arduino = ((address < 0x6000) || (address >= 0x8000));

	if (reading && with_arduino) {
		unsigned int offset = address - 0x8000;

		if (address < RAMLEN) {
			data = RAM[address];
		} else if (address == 0xfffc) {
			data = 0x00;
		} else if (address == 0xfffd) {
			data = 0x80;
		} else if (address == 0xfffe) {
			data = 0x10;
		} else if (address == 0xffff) {
			data = 0x80;
		} else if (offset < ROMLEN) {
			data = MEMORY[offset];
		} else {
			data = 0;
		}

		if (log_debug) {
				sprintf(output, "      %c    %04x    %02x", rwc, address, data);
			}
		for (int n = 0; n < 8; n++) {
			pinMode(DATA[n], OUTPUT);
			digitalWrite(DATA[n], data % 2);
			data = data >> 1;
		}
	} else {
		for (int n = 0; n < 8; n ++) {
			pinMode(DATA[n], INPUT);
			int bit = digitalRead(DATA[n]) ? 1 : 0;
			data = data + (bit << n);
		}

		if (log_debug) {
				sprintf(output, "      %c    %04x    %02x", rwc, address, data);
			}
		if (address < RAMLEN) {
			RAM[address] = data;
		}
	}

	if (log_debug) {
			Serial.println(output);
		}
}

void setup() {
  for (int n = 0; n < 16; n++) {
    pinMode(ADDR[n], INPUT);
  }
  pinMode(CLOCK, INPUT);
  attachInterrupt(digitalPinToInterrupt(CLOCK), onClock, RISING);
  Serial.begin(9600);
}

void loop() {
}
