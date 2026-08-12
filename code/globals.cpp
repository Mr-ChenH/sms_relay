#include "globals.h"

Config config;
Preferences preferences;
PDU pdu = PDU(4096);
bool configValid = false;
bool modemReady = false;
unsigned long lastPrintTime = 0;
ConcatSms concatBuffer[MAX_CONCAT_MESSAGES];
