#include "globals.h"
#include "wifi_config.h"
#include "config.h"
#include "logger.h"
#include "wifi_manager.h"
#include "terminal_client.h"
#include "modem.h"
#include "sms_process.h"
#include "esim.h"

SET_LOOP_TASK_STACK_SIZE(16 * 1024);

void setup() {
  pinMode(LED_BUILTIN, OUTPUT);
  digitalWrite(LED_BUILTIN, HIGH);
  Serial.begin(115200);
  // 缩短初始化延时，WiFi连接会处理自己的超时
  delay(200);
  Serial1.begin(115200, SERIAL_8N1, RXD, TXD);
  Serial1.setRxBufferSize(SERIAL_BUFFER_SIZE);
  while (Serial1.available()) Serial1.read();
  initConcatBuffer();
  loadConfig();
  configValid = isConfigValid();

  // ---- WiFi 连接 / SoftAP 配网 ----
  connectWiFiOrStartProvisioning();

  // ---- 中心服务终端客户端 ----
  // WiFi 可用后立即上线，模组和 eSIM 初始化不能阻塞管理链路。
  terminalClientInit();

  // ---- NTP 时间同步 ----
  logCaptureLn(String("正在同步NTP时间..."));
  configTime(0, 0, "ntp.ntsc.ac.cn", "ntp.aliyun.com", "pool.ntp.org");
  int ntpRetry = 0;
  while (time(nullptr) < 100000 && ntpRetry < 100) {
    delay(1);
    ntpRetry++;
  }
  if (time(nullptr) >= 100000) {
    time_t now = time(nullptr);
    logCapture(String("当前UTC时间戳: "));
    logCaptureLn(String(now));
  } else {
    logCaptureLn(String("NTP时间同步失败，将使用设备时间"));
  }

  digitalWrite(LED_BUILTIN, LOW);

  // ---- 模组初始化 ----
  modemPowerCycle();
  while (Serial1.available()) Serial1.read();
  bool modemInitialized = modemInit();

  // ---- eSIM初始化 ----
  logCaptureLn(String("初始化eSIM..."));
  bool esimInitialized = false;
  if (modemInitialized && esimInit()) {
    esimInitialized = true;
    logCaptureLn(String("eSIM初始化成功"));
    char eid[40];
    if (esimGetEID(eid, sizeof(eid))) {
      logCapture(String("EID: "));
      logCaptureLn(eid);
    }
  } else {
    logCaptureLn(String("eSIM初始化失败或模组未就绪"));
  }

  // MQTT 会在模组初始化前上线；无论模组初始化是否完全成功
  // （如启动时网络尚未注册导致 CEREG 超时、modemInit 返回 false），
  // 都应尽早补发身份信息：AT+CNUM 等不依赖网络注册，号码可立即读取；
  // 运营商在网络注册后由 settle 窗口内的定期刷新补上。
  terminalClientIdentityReady();
  if (!esimInitialized) logCaptureLn(String("已按物理 SIM 信息完成首次身份上报"));

}

void loop() {
  if (!configValid) {
    if (millis() - lastPrintTime >= 1000) {
      lastPrintTime = millis();
      logCaptureLn(String("请连接热点 SMSHub-XXXXXX 并打开 http://192.168.4.1 配置终端参数"));
    }
  }
  checkConcatTimeout();
  wifiManagerLoop();
  terminalClientLoop();
  handleSerialConsole();
  checkSerial1URC();
}
