#include "logger.h"
#include <stdarg.h>

static String logLine;

static void logCommit() {
  logLine = "";
}

void logCapture(const String& msg) {
  Serial.print(msg);
  logLine += msg;
}

void logCapture(const char* msg) {
  Serial.print(msg);
  logLine += msg;
}

void logCaptureF(const char* fmt, ...) {
  char buf[256];
  va_list args;
  va_start(args, fmt);
  vsnprintf(buf, sizeof(buf), fmt, args);
  va_end(args);
  Serial.print(buf);
  logLine += buf;
  size_t len = strlen(buf);
  if (len > 0 && buf[len - 1] == '\n') {
    logLine.trim();
    logCommit();
  }
}

void logCaptureLn(const String& msg) {
  Serial.println(msg);
  logLine += msg;
  logCommit();
}

void logCaptureLn(const char* msg) {
  Serial.println(msg);
  logLine += msg;
  logCommit();
}
