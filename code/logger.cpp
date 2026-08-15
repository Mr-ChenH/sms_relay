#include "logger.h"
#include <stdarg.h>

// 日志统一直接输出到 USB 串口。终端日志上报由 terminalReportLog 单独负责，
// 不再维护只写不读的 logLine 缓冲（旧实现会随 logCapture 无界增长，
// 长时间无 logCaptureLn 时造成堆碎片/内存耗尽，并在多任务下产生数据竞争）。

void logCapture(const String& msg) {
  Serial.print(msg);
}

void logCapture(const char* msg) {
  Serial.print(msg);
}

void logCaptureF(const char* fmt, ...) {
  char buf[256];
  va_list args;
  va_start(args, fmt);
  vsnprintf(buf, sizeof(buf), fmt, args);
  va_end(args);
  Serial.print(buf);
}

void logCaptureLn(const String& msg) {
  Serial.println(msg);
}

void logCaptureLn(const char* msg) {
  Serial.println(msg);
}
