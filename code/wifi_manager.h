#ifndef WIFI_MANAGER_H
#define WIFI_MANAGER_H

#include "globals.h"

bool connectWiFiOrStartProvisioning();
bool reconnectConfiguredWiFi(unsigned long timeoutMs);
void wifiManagerLoop();
void resetWiFiProvisioning();
String wifiProvisioningServiceName();
String wifiProvisioningPOP();

#endif
