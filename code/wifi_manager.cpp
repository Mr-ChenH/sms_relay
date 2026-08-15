#include "wifi_manager.h"
#include "wifi_config.h"
#include "logger.h"
#include "modem.h"
#include "config.h"
#include "terminal_client.h"
#include "web_config.h"

#ifndef WIFI_PROV_SERVICE_PREFIX
#define WIFI_PROV_SERVICE_PREFIX "SMSHub"
#endif

#ifndef WIFI_PROV_FORCE_ON_BOOT
#define WIFI_PROV_FORCE_ON_BOOT 0
#endif

static const char* WIFI_PREF_NAMESPACE = "wifi_runtime";
static const unsigned long WIFI_CONNECT_TIMEOUT_MS = 20000;
static const unsigned long WIFI_PROVISION_WAIT_MS = 30000;  // 首次配网等待上限，避免 setup 永久阻塞

static bool apStarted = false;
static bool pendingCredentials = false;
static bool pendingTerminalConfig = false;
static volatile bool wifiConnectionLost = false;
static String pendingSSID;
static String pendingPassword;

// 配网页/串口命令只暂存配置，由主循环 applyPendingConfiguration 真正写入
// NVS/Preferences（避免在回调/事件上下文并发访问共享资源）。
static bool pendingServerSave = false;
static String pendingServerHost;
static bool pendingMqttSave = false;
static String pendingMqttBroker;
static String pendingMqttUser;
static String pendingMqttPass;

// 设备热点名称：SMSHub-XXXXXX（MAC 后六位）
String wifiProvisioningServiceName() {
  String mac = WiFi.macAddress();
  mac.replace(":", "");
  mac.toUpperCase();
  if (mac.length() > 6) mac = mac.substring(mac.length() - 6);
  return String(WIFI_PROV_SERVICE_PREFIX) + "-" + mac;
}

static void saveHubConfig(const String& mqttBroker, const String& mqttUser, const String& mqttPass);

static bool loadSavedWiFi(String& ssid, String& password) {
  preferences.begin(WIFI_PREF_NAMESPACE, true);
  ssid = preferences.getString("ssid", "");
  password = preferences.getString("pass", "");
  preferences.end();
  return ssid.length() > 0;
}

static void saveWiFiCredentials(const String& ssid, const String& password) {
  preferences.begin(WIFI_PREF_NAMESPACE, false);
  preferences.putString("ssid", ssid);
  preferences.putString("pass", password);
  preferences.end();
}

static String hostFromURL(String value) {
  value.trim();
  value.replace("http://", "");
  value.replace("https://", "");
  value.replace("mqtt://", "");
  int slash = value.indexOf('/');
  if (slash >= 0) value = value.substring(0, slash);
  int colon = value.indexOf(':');
  if (colon > 0) value = value.substring(0, colon);
  value.trim();
  return value;
}

static void saveServerHost(const String& hostValue) {
  String host = hostFromURL(hostValue);
  if (host.length() == 0) return;
  saveHubConfig(String("mqtt://") + host + ":1883", "", "");
}

static void saveHubConfig(const String& mqttBroker, const String& mqttUser, const String& mqttPass) {
  String broker = mqttBroker;
  broker.trim();
  String user = mqttUser;
  user.trim();
  config.smsHubMqttBroker = broker;
  config.smsHubMqttUser = user;
  config.smsHubMqttPass = mqttPass;
  saveConfig();
  configValid = isConfigValid();
  pendingTerminalConfig = true;
}

static void saveHubMqttConfig(const String& mqttBroker, const String& mqttUser, const String& mqttPass) {
  saveHubConfig(mqttBroker, mqttUser, mqttPass);
}

static void clearWiFiCredentials() {
  preferences.begin(WIFI_PREF_NAMESPACE, false);
  preferences.clear();
  preferences.end();
}

static void onWiFiEvent(arduino_event_t* event) {
  switch (event->event_id) {
    case ARDUINO_EVENT_WIFI_STA_GOT_IP:
      logCaptureLn(String("WiFi 已连接, IP: ") + WiFi.localIP().toString());
      break;
    case ARDUINO_EVENT_WIFI_STA_DISCONNECTED:
      wifiConnectionLost = true;
      logCaptureLn(String("WiFi 已断开"));
      break;
    default:
      break;
  }
}

static bool connectWiFiWithCredentials(const String& ssid, const String& password, unsigned long timeoutMs) {
  WiFi.mode(WIFI_AP_STA);  // 保持 SoftAP 配网热点共存，随时可重新配网
  WiFi.setSleep(false);
  WiFi.setAutoReconnect(true);
  WiFi.setScanMethod(WIFI_FAST_SCAN);
  WiFi.setSortMethod(WIFI_CONNECT_AP_BY_SIGNAL);
  WiFi.begin(ssid.c_str(), password.c_str());

  logCaptureLn(String("正在连接 WiFi: ") + ssid);
  unsigned long start = millis();
  while (WiFi.status() != WL_CONNECTED && millis() - start < timeoutMs) {
    blink_short(200);
  }

  if (WiFi.status() == WL_CONNECTED) {
    logCaptureLn(String("WiFi 已连接"));
    logCapture(String("SSID: "));
    logCaptureLn(WiFi.SSID());
    logCapture(String("IP地址: "));
    logCaptureLn(WiFi.localIP().toString());
    logCapture(String("信号强度(RSSI): "));
    logCaptureLn(String(WiFi.RSSI()) + " dBm");
    return true;
  }

  logCaptureLn(String("WiFi 连接失败: ") + ssid);
  return false;
}

// 启动 SoftAP 配网热点（无密码，局域网内短时开放；正式部署建议后续加 PIN 码保护）
void startProvisioningAP() {
  if (apStarted) return;
  apStarted = true;
  String apName = wifiProvisioningServiceName();
  WiFi.mode(WIFI_AP_STA);
  WiFi.softAP(apName.c_str());
  webConfigInit();
  logCaptureLn(String("SoftAP 配网已启动: 热点 ") + apName + " -> http://192.168.4.1");
}

// 配网页/串口命令统一入口：暂存配置，主循环 applyPendingConfiguration 处理
void saveProvisionedConfig(const String& ssid, const String& password, const String& hub) {
  String s = ssid;
  s.trim();
  if (s.length() > 0) {
    pendingSSID = s;
    pendingPassword = password;
    pendingCredentials = true;
  }
  String h = hub;
  h.trim();
  if (h.length() > 0) {
    // 纯 host 规范化为 mqtt://host:1883；带端口或完整 URL 的保留原样
    if (h.indexOf("://") < 0) {
      if (h.indexOf(':') < 0) h = String("mqtt://") + h + ":1883";
      else h = String("mqtt://") + h;
    }
    pendingMqttBroker = h;
    pendingMqttUser = "";
    pendingMqttPass = "";
    pendingMqttSave = true;
  }
  logCaptureLn(String("收到配网配置: SSID=") + (s.length() > 0 ? s : "(不变)") + ", hub=" + (h.length() > 0 ? h : "(不变)"));
}

static void applyPendingConfiguration() {
  if (wifiConnectionLost) {
    wifiConnectionLost = false;
    terminalClientConfigChanged();
  }

  if (pendingTerminalConfig) {
    pendingTerminalConfig = false;
    terminalClientConfigChanged();
  }

  // 应用配网页/串口暂存的中心服务配置
  if (pendingServerSave) {
    pendingServerSave = false;
    String host = pendingServerHost;
    pendingServerHost = "";
    saveServerHost(host);
  }
  if (pendingMqttSave) {
    pendingMqttSave = false;
    String broker = pendingMqttBroker;
    String user = pendingMqttUser;
    String pass = pendingMqttPass;
    pendingMqttBroker = "";
    pendingMqttUser = "";
    pendingMqttPass = "";
    saveHubMqttConfig(broker, user, pass);
  }

  if (!pendingCredentials) return;
  pendingCredentials = false;
  String previousSSID;
  String previousPassword;
  bool hadPreviousCredentials = loadSavedWiFi(previousSSID, previousPassword);
  WiFi.setAutoReconnect(false);
  WiFi.disconnect(true, false);
  delay(100);
  if (connectWiFiWithCredentials(pendingSSID, pendingPassword, WIFI_CONNECT_TIMEOUT_MS)) {
    saveWiFiCredentials(pendingSSID, pendingPassword);
    return;
  }

  if (hadPreviousCredentials && previousSSID != pendingSSID) {
    logCaptureLn(String("恢复此前 WiFi: ") + previousSSID);
    connectWiFiWithCredentials(previousSSID, previousPassword, WIFI_CONNECT_TIMEOUT_MS);
  }
  WiFi.setAutoReconnect(true);
}

bool reconnectConfiguredWiFi(unsigned long timeoutMs) {
  String ssid;
  String password;
  if (!loadSavedWiFi(ssid, password)) {
    logCaptureLn(String("未保存 WiFi 凭据"));
    return false;
  }
  return connectWiFiWithCredentials(ssid, password, timeoutMs);
}

bool connectWiFiOrStartProvisioning() {
  WiFi.onEvent(onWiFiEvent);
  startProvisioningAP();  // 热点常开，任何时刻都能通过配网页修改配置

#if !WIFI_PROV_FORCE_ON_BOOT
  if (reconnectConfiguredWiFi(WIFI_CONNECT_TIMEOUT_MS)) {
    return true;
  }
#else
  logCaptureLn(String("强制配网模式：清除已保存凭据"));
  WiFi.disconnect(true, true);
  clearWiFiCredentials();
#endif

  // 有界等待配网：首次使用或已保存凭据暂时不可达时，给配网页留一段时间。
  // 超时后继续启动模组与 MQTT（WiFi 驱动会按已保存凭据在后台自动重连，
  // 主循环的 wifiManagerLoop 持续处理配网页/串口写入的新配置），设备不会卡死。
  unsigned long provisionStart = millis();
  while (WiFi.status() != WL_CONNECTED && millis() - provisionStart < WIFI_PROVISION_WAIT_MS) {
    applyPendingConfiguration();
    webConfigLoop();
    blink_short(500);
  }
  return true;
}

void wifiManagerLoop() {
  applyPendingConfiguration();
  webConfigLoop();
}

void resetWiFiProvisioning() {
  logCaptureLn(String("清除已保存 WiFi 凭据，SoftAP 配网保持可用"));
  WiFi.disconnect(true, false);
  clearWiFiCredentials();
}

// 串口配网命令（在 eSIM 命令之前调用）：
//   wifi <SSID> <密码>     保存并连接 WiFi
//   server <host>          保存 SMS Hub 服务器地址（mqtt://host:1883 或 host）
//   status                 查看 WiFi/服务器配置状态
//   wifi reset             清除已保存的 WiFi 凭据
bool provisionSerialCommand(const String& cmd) {
  String line = cmd;
  line.trim();
  if (line.length() == 0) return false;

  if (line == "status") {
    String ssid;
    String pass;
    bool has = loadSavedWiFi(ssid, pass);
    Serial.println("--- 终端状态 ---");
    Serial.println(String("WiFi 连接: ") + (WiFi.status() == WL_CONNECTED ? WiFi.SSID() + " (" + WiFi.localIP().toString() + ")" : "未连接"));
    Serial.println(String("已保存 WiFi: ") + (has ? ssid : "(无)"));
    Serial.println(String("SMS Hub 服务器: ") + (config.smsHubMqttBroker.length() > 0 ? config.smsHubMqttBroker : "(未配置)"));
    Serial.println(String("配网热点: ") + wifiProvisioningServiceName() + " -> http://192.168.4.1");
    return true;
  }

  if (line == "wifi reset") {
    clearWiFiCredentials();
    logCaptureLn(String("WiFi 凭据已清除"));
    Serial.println("WiFi 凭据已清除");
    return true;
  }

  if (line.startsWith("wifi")) {
    String rest = line.substring(4);
    rest.trim();
    if (rest.startsWith("set ")) rest = rest.substring(4);
    int sp = rest.indexOf(' ');
    String ssid = sp >= 0 ? rest.substring(0, sp) : rest;
    String pass = sp >= 0 ? rest.substring(sp + 1) : "";
    ssid.trim();
    if (ssid.length() == 0) {
      Serial.println("用法: wifi <SSID> <密码>  （密码可留空）");
      return true;
    }
    saveProvisionedConfig(ssid, pass, "");
    Serial.println(String("WiFi 配置已保存: ") + ssid);
    return true;
  }

  if (line.startsWith("server")) {
    String rest = line.substring(6);
    rest.trim();
    if (rest.startsWith("set ")) rest = rest.substring(4);
    if (rest.length() == 0) {
      Serial.println(String("当前服务器: ") + (config.smsHubMqttBroker.length() > 0 ? config.smsHubMqttBroker : "(未配置)"));
      return true;
    }
    saveProvisionedConfig("", "", rest);
    Serial.println(String("服务器已保存: ") + rest);
    return true;
  }

  return false;
}
