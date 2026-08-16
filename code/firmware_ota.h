#pragma once

#include <Arduino.h>

bool firmwareOTAExecute(const String& url, const String& version, size_t expectedSize,
                        const String& expectedSHA256, const String& expectedHardware,
                        String& result);
void firmwareOTAMarkValid();
