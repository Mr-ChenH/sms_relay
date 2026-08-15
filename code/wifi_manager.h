#ifndef WIFI_MANAGER_H
#define WIFI_MANAGER_H

#include "globals.h"

// WiFi 连接与配网管理：
// - 设备热点 SMSHub-XXXXXX 常开（SoftAP），配网页 http://192.168.4.1
// - 串口命令：wifi <SSID> <密码> / server <host> / status / wifi reset
bool connectWiFiOrStartProvisioning();
bool reconnectConfiguredWiFi(unsigned long timeoutMs);
void wifiManagerLoop();
void resetWiFiProvisioning();
String wifiProvisioningServiceName();

// 供配网页/串口命令调用：暂存配置，由主循环统一写入 NVS 并触发连接。
// hub 为空表示不修改服务器地址；hub 可为 "mqtt://host:1883" 或 "host"。
void saveProvisionedConfig(const String& ssid, const String& password, const String& hub);

// 串口配网命令入口，返回 true 表示命令已处理。在 eSIM 命令之前调用。
bool provisionSerialCommand(const String& cmd);

#endif
