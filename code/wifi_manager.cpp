#include "wifi_manager.h"
#include "wifi_config.h"
#include "logger.h"
#include "modem.h"
#include "config.h"
#include "terminal_client.h"
#include <BLEDevice.h>
#include <BLEServer.h>
#include <BLEUtils.h>

#ifndef WIFI_PROV_SERVICE_PREFIX
#define WIFI_PROV_SERVICE_PREFIX "SMSCFG"
#endif

#ifndef WIFI_PROV_FORCE_ON_BOOT
#define WIFI_PROV_FORCE_ON_BOOT 0
#endif

static const char* WIFI_PREF_NAMESPACE = "wifi_runtime";
static const char* BLE_SERVICE_UUID = "7d6d0001-5f36-4f64-8f2b-ec2a7b3d0101";
static const char* BLE_CRED_CHAR_UUID = "7d6d0002-5f36-4f64-8f2b-ec2a7b3d0101";
static const char* BLE_STATUS_CHAR_UUID = "7d6d0003-5f36-4f64-8f2b-ec2a7b3d0101";

static bool provisioningStarted = false;
static bool pendingCredentials = false;
static bool pendingTerminalConfig = false;
static String pendingSSID;
static String pendingPassword;
static BLECharacteristic* statusCharacteristic = nullptr;

static void saveHubConfig(const String& mqttBroker, const String& mqttUser, const String& mqttPass);

String wifiProvisioningPOP() {
  return "none";
}

String wifiProvisioningServiceName() {
  String mac = WiFi.macAddress();
  mac.replace(":", "");
  mac.toUpperCase();
  if (mac.length() > 6) mac = mac.substring(mac.length() - 6);
  return String(WIFI_PROV_SERVICE_PREFIX) + "-" + mac;
}

static void setBLEStatus(const String& status) {
  if (statusCharacteristic) statusCharacteristic->setValue(status.c_str());
}

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
      setBLEStatus(String("connected:") + WiFi.localIP().toString());
      break;
    case ARDUINO_EVENT_WIFI_STA_DISCONNECTED:
      logCaptureLn(String("WiFi 已断开"));
      if (WiFi.status() != WL_CONNECTED) setBLEStatus("wifi_disconnected");
      break;
    default:
      break;
  }
}

static bool connectWiFiWithCredentials(const String& ssid, const String& password, unsigned long timeoutMs) {
  WiFi.mode(WIFI_STA);
  WiFi.setSleep(false);
  WiFi.setAutoReconnect(true);
  WiFi.setScanMethod(WIFI_FAST_SCAN);
  WiFi.setSortMethod(WIFI_CONNECT_AP_BY_SIGNAL);
  WiFi.begin(ssid.c_str(), password.c_str());

  logCaptureLn(String("正在连接 WiFi: ") + ssid);
  setBLEStatus(String("connecting:") + ssid);
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
    setBLEStatus(String("connected:") + WiFi.localIP().toString());
    return true;
  }

  logCaptureLn(String("WiFi 连接失败: ") + ssid);
  setBLEStatus("connect_failed");
  return false;
}

class WiFiConfigCallbacks : public BLECharacteristicCallbacks {
  void onWrite(BLECharacteristic* characteristic) override {
    String value = characteristic->getValue();
    value.trim();

    if (value.startsWith("SERVER|")) {
      String host = value.substring(7);
      host.trim();
      if (host.length() == 0) {
        setBLEStatus("invalid_server_host");
        return;
      }
      saveServerHost(host);
      setBLEStatus(String("server_saved:") + hostFromURL(host));
      return;
    }

    if (value.startsWith("MQTT|")) {
      String rest = value.substring(5);
      int first = rest.indexOf('|');
      int second = first >= 0 ? rest.indexOf('|', first + 1) : -1;
      String broker = first >= 0 ? rest.substring(0, first) : rest;
      String user = first >= 0 && second >= 0 ? rest.substring(first + 1, second) : "";
      String pass = second >= 0 ? rest.substring(second + 1) : "";
      broker.trim();
      if (broker.length() == 0) {
        setBLEStatus("invalid_mqtt_broker");
        return;
      }
      saveHubMqttConfig(broker, user, pass);
      setBLEStatus(String("mqtt_saved:") + config.smsHubMqttBroker);
      return;
    }

    if (value.startsWith("HUB|")) {
      String rest = value.substring(4);
      int first = rest.indexOf('|');
      int second = first >= 0 ? rest.indexOf('|', first + 1) : -1;
      int third = second >= 0 ? rest.indexOf('|', second + 1) : -1;
      String broker = first >= 0 && second >= 0 ? rest.substring(first + 1, second) : "";
      String user = second >= 0 && third >= 0 ? rest.substring(second + 1, third) : "";
      String pass = third >= 0 ? rest.substring(third + 1) : "";
      broker.trim();
      if (broker.length() == 0) {
        setBLEStatus("invalid_mqtt_broker");
        return;
      }
      saveHubMqttConfig(broker, user, pass);
      setBLEStatus(String("hub_saved:") + config.smsHubMqttBroker);
      return;
    }

    int sep = value.indexOf('|');
    if (sep <= 0) sep = value.indexOf('\n');
    if (sep <= 0) {
      setBLEStatus("invalid_format_use_ssid_pipe_password_pipe_api");
      return;
    }

    int mqttSep = value.indexOf('|', sep + 1);
    int userSep = mqttSep > sep ? value.indexOf('|', mqttSep + 1) : -1;
    int passSep = userSep > mqttSep ? value.indexOf('|', userSep + 1) : -1;
    pendingSSID = value.substring(0, sep);
    pendingPassword = mqttSep > sep ? value.substring(sep + 1, mqttSep) : value.substring(sep + 1);
    String pendingMqttBroker = mqttSep > sep ? (userSep > mqttSep ? value.substring(mqttSep + 1, userSep) : value.substring(mqttSep + 1)) : "";
    String pendingMqttUser = userSep > mqttSep ? (passSep > userSep ? value.substring(userSep + 1, passSep) : value.substring(userSep + 1)) : "";
    String pendingMqttPass = passSep > userSep ? value.substring(passSep + 1) : "";
    pendingSSID.trim();
    pendingPassword.trim();
    pendingMqttBroker.trim();
    if (pendingSSID.length() == 0) {
      setBLEStatus("invalid_ssid");
      return;
    }

    if (pendingMqttBroker.length() > 0) {
      saveHubMqttConfig(pendingMqttBroker, pendingMqttUser, pendingMqttPass);
    }
    pendingCredentials = true;
    setBLEStatus(String("received:") + pendingSSID + (pendingMqttBroker.length() > 0 ? ":mqtt" : ":wifi"));
  }
};

static void beginCustomBLEProvisioning() {
  if (provisioningStarted) return;
  provisioningStarted = true;

  String serviceName = wifiProvisioningServiceName();
  logCaptureLn(String("BLE WiFi 配网已启动"));
  logCaptureLn(String("BLE 设备名: ") + serviceName);
  logCaptureLn(String("写入格式: SSID|PASSWORD|SERVER_HOST"));
  logCaptureLn(String("仅修改服务器: SERVER|SERVER_HOST"));
  logCaptureLn(String("兼容格式: SSID|PASSWORD|MQTT_BROKER|MQTT_USER|MQTT_PASS"));

  if (!BLEDevice::init(serviceName.c_str())) {
    logCaptureLn(String("BLE 初始化失败"));
    return;
  }

  BLEServer* server = BLEDevice::createServer();
  BLEService* service = server->createService(BLE_SERVICE_UUID);

  BLECharacteristic* credCharacteristic = service->createCharacteristic(
    BLE_CRED_CHAR_UUID,
    BLECharacteristic::PROPERTY_READ | BLECharacteristic::PROPERTY_WRITE
  );
  credCharacteristic->setValue("write SSID|PASSWORD|SERVER_HOST");
  credCharacteristic->setCallbacks(new WiFiConfigCallbacks());

  statusCharacteristic = service->createCharacteristic(
    BLE_STATUS_CHAR_UUID,
    BLECharacteristic::PROPERTY_READ
  );
  setBLEStatus("waiting_for_credentials");

  service->start();
  BLEAdvertising* advertising = BLEDevice::getAdvertising();
  advertising->addServiceUUID(BLE_SERVICE_UUID);
  advertising->setScanResponse(true);
  advertising->setMinPreferred(0x06);
  advertising->setMaxPreferred(0x12);
  BLEDevice::startAdvertising();
  logCaptureLn(String("BLE 广播已启动"));
}

static void applyPendingConfiguration() {
  if (pendingTerminalConfig) {
    pendingTerminalConfig = false;
    terminalClientConfigChanged();
  }

  if (!pendingCredentials) return;
  pendingCredentials = false;
  String previousSSID;
  String previousPassword;
  bool hadPreviousCredentials = loadSavedWiFi(previousSSID, previousPassword);
  WiFi.setAutoReconnect(false);
  WiFi.disconnect(true, false);
  delay(100);
  if (connectWiFiWithCredentials(pendingSSID, pendingPassword, 20000)) {
    saveWiFiCredentials(pendingSSID, pendingPassword);
    return;
  }

  if (hadPreviousCredentials && previousSSID != pendingSSID) {
    logCaptureLn(String("恢复此前 WiFi: ") + previousSSID);
    setBLEStatus(String("restoring:") + previousSSID);
    connectWiFiWithCredentials(previousSSID, previousPassword, 20000);
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
#if !WIFI_PROV_FORCE_ON_BOOT
  if (reconnectConfiguredWiFi(20000)) {
    beginCustomBLEProvisioning();
    return true;
  }
#else
  logCaptureLn(String("强制启动 BLE WiFi 配网模式"));
  WiFi.disconnect(true, true);
  clearWiFiCredentials();
#endif

  beginCustomBLEProvisioning();
  while (WiFi.status() != WL_CONNECTED) {
    applyPendingConfiguration();
    blink_short(500);
  }
  return true;
}

void wifiManagerLoop() {
  applyPendingConfiguration();
}

void resetWiFiProvisioning() {
  logCaptureLn(String("清除已保存 WiFi 凭据并重新启动 BLE 配网"));
  WiFi.disconnect(true, true);
  clearWiFiCredentials();
  delay(500);
  beginCustomBLEProvisioning();
}
