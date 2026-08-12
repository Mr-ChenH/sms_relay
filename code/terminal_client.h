#ifndef TERMINAL_CLIENT_H
#define TERMINAL_CLIENT_H

#include "globals.h"

void terminalClientInit();
void terminalClientLoop();
void terminalClientService();
void terminalReportSMS(const char* sender, const char* text, const char* timestamp);
void terminalReportLog(const String& level, const String& message);
void terminalSyncEsimProfiles();
bool terminalClientEnabled();

#endif
