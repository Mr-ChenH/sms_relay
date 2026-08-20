#ifndef ESIM_AT_H
#define ESIM_AT_H

#include <Arduino.h>

String esimSendAT(const char* command, unsigned long timeout);
String esimCompactATResponse(const String& response);
bool esimParseATPayload(const String& response, const char* prefix, String* payload);
bool esimParseRequiredATPayload(const String& response, const char* prefix, String* payload);
bool esimParseCGLAHexPayload(const String& response, String* hex);

#endif
