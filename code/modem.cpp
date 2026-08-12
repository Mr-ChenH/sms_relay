#include "modem.h"
#include "logger.h"
#include "terminal_client.h"

// 发送AT命令并获取响应
String sendATCommand(const char* cmd, unsigned long timeout) {
  while (Serial1.available()) Serial1.read();
  Serial1.println(cmd);
  
  unsigned long start = millis();
  String resp = "";
  while (millis() - start < timeout) {
    terminalClientService();
    if (Serial1.available()) {
      char c = Serial1.read();
      resp += c;
      if (resp.indexOf("OK") >= 0 || resp.indexOf("ERROR") >= 0) {
        // 读取剩余数据（最多 50ms）
        unsigned long t = millis();
        while (millis() - t < 50) {
          if (Serial1.available()) resp += (char)Serial1.read();

        }
        return resp;
      }
    }

  }
  return resp;
}

// 新增"模组断电重启"函数
bool modemPowerCycle() {
  pinMode(MODEM_EN_PIN, OUTPUT);

  logCaptureLn(String("EN 拉低：关闭模组"));
  digitalWrite(MODEM_EN_PIN, LOW);
  delay(1200);

  logCaptureLn(String("EN 拉高：开启模组"));
  digitalWrite(MODEM_EN_PIN, HIGH);
  delay(6000);
  return true;
}

// 重启模组（EN引脚断电重启 + 有界初始化）
bool resetModule() {
  logCaptureLn(String("正在硬重启模组（EN 断电重启）..."));
  modemPowerCycle();
  return modemInit();
}

// 模组初始化必须有界，失败后由主循环继续维持 MQTT，不能永久阻塞终端。
bool modemInit() {
  while (Serial1.available()) Serial1.read();

  bool atReady = false;
  for (int attempt = 0; attempt < 20; attempt++) {
    if (sendATandWaitOK("AT", 1000)) {
      atReady = true;
      break;
    }
    logCaptureLn(String("AT未响应，重试 ") + String(attempt + 1) + "/20");
    blink_short(200);
  }
  if (!atReady) {
    logCaptureLn(String("模组AT初始化超时"));
    modemReady = false;
    return false;
  }
  logCaptureLn(String("模组AT响应正常"));

  //判断型号，做一些特定操作
  bool need_set_CGACT = true;
  String resp = sendATCommand("ATI", 2000);
  logCaptureLn(String("ATI响应: " + resp));
  if (resp.indexOf("OK") >= 0) {
    // 解析ATI响应
    String manufacturer = "未知";
    String model = "未知";
    String version = "未知";
    
    // 按行解析
    int lineStart = 0;
    int lineNum = 0;
    for (int i = 0; i < resp.length(); i++) {
      if (resp.charAt(i) == '\n' || i == resp.length() - 1) {
        String line = resp.substring(lineStart, i);
        line.trim();
        if (line.length() > 0 && line != "ATI" && line != "OK") {
          lineNum++;
          if (lineNum == 1) manufacturer = line;
          else if (lineNum == 2) model = line;
          else if (lineNum == 3) version = line;
        }
        lineStart = i + 1;
      }
    }
    //这个模组这条命令有bug
    if(model == "ML307Y") need_set_CGACT = false;
  }

  if (need_set_CGACT) {
    bool cgactConfigured = false;
    for (int attempt = 0; attempt < 3; attempt++) {
      if (sendATandWaitOK("AT+CGACT=0,1", 5000)) {
        cgactConfigured = true;
        break;
      }
      logCaptureLn(String("设置CGACT失败，重试 ") + String(attempt + 1) + "/3");
      blink_short(200);
    }
    if (cgactConfigured) {
      logCaptureLn(String("已禁用数据连接(AT+CGACT=0,1)，防止流量消耗"));
    } else {
      logCaptureLn(String("设置CGACT失败，继续初始化"));
    }
  } else {
    logCaptureLn(String("该型号无法配置(AT+CGACT=0,1)，跳过该命令"));
  }

  bool smsConfigured = false;
  for (int attempt = 0; attempt < 3; attempt++) {
    if (sendATandWaitOK("AT+CNMI=2,2,0,0,0", 1000) &&
        sendATandWaitOK("AT+CMGF=0", 1000)) {
      smsConfigured = true;
      break;
    }
    logCaptureLn(String("短信参数设置失败，重试 ") + String(attempt + 1) + "/3");
    blink_short(200);
  }
  if (!smsConfigured) {
    logCaptureLn(String("短信参数初始化失败"));
    modemReady = false;
    return false;
  }
  logCaptureLn(String("短信参数设置完成"));

  int ceregRetry = 0;
  while (!waitCEREG() && ceregRetry < 15) {
    ceregRetry++;
    logCaptureLn(String("等待网络注册 ") + String(ceregRetry) + "/15");
    blink_short(200);
  }
  if (ceregRetry < 15) {
    logCaptureLn(String("网络已注册"));
    modemReady = true;
    return true;
  }

  logCaptureLn(String("网络注册超时，终端控制链路继续运行"));
  modemReady = false;
  return false;
}

void blink_short(unsigned long gap_time) {
  digitalWrite(LED_BUILTIN, LOW);
  delay(50);
  digitalWrite(LED_BUILTIN, HIGH);
  delay(gap_time);
}

bool sendATandWaitOK(const char* cmd, unsigned long timeout) {
  while (Serial1.available()) Serial1.read();
  Serial1.println(cmd);
  unsigned long start = millis();
  String resp = "";
  while (millis() - start < timeout) {
    terminalClientService();
    if (Serial1.available()) {
      char c = Serial1.read();
      resp += c;
      if (resp.indexOf("OK") >= 0) return true;
      if (resp.indexOf("ERROR") >= 0) return false;
    }

  }
  return false;
}

// 检测网络注册状态（LTE/4G）
// CEREG状态: 1=已注册本地, 5=已注册漫游
bool waitCEREG() {
  Serial1.println("AT+CEREG?");
  unsigned long start = millis();
  String resp = "";
  while (millis() - start < 2000) {
    terminalClientService();
    if (Serial1.available()) {
      char c = Serial1.read();
      resp += c;
      if (resp.indexOf("+CEREG:") >= 0) {
        if (resp.indexOf(",1") >= 0 || resp.indexOf(",5") >= 0) return true;
        if (resp.indexOf(",0") >= 0 || resp.indexOf(",2") >= 0 || 
            resp.indexOf(",3") >= 0 || resp.indexOf(",4") >= 0) return false;
      }
    }

  }
  return false;
}

// 发送短信（PDU模式）
bool sendSMS(const char* phoneNumber, const char* message) {
  logCaptureLn(String("准备发送短信..."));
  logCapture(String("目标号码: ")); logCaptureLn(String(phoneNumber));
  logCapture(String("短信内容: ")); logCaptureLn(String(message));

  // 使用pdulib编码PDU
  pdu.setSCAnumber();  // 使用默认短信中心
  int pduLen = pdu.encodePDU(phoneNumber, message);
  
  if (pduLen < 0) {
    logCapture(String("PDU编码失败，错误码: "));
    logCaptureLn(String(pduLen));
    return false;
  }
  
  logCapture(String("PDU数据: ")); logCaptureLn(String(pdu.getSMS()));
  logCapture(String("PDU长度: ")); logCaptureLn(String(pduLen));
  
  // 发送AT+CMGS命令
  String cmgsCmd = "AT+CMGS=";
  cmgsCmd += pduLen;
  
  while (Serial1.available()) Serial1.read();
  Serial1.println(cmgsCmd);
  
  // 等待 > 提示符
  unsigned long start = millis();
  bool gotPrompt = false;
  while (millis() - start < 5000) {
    if (Serial1.available()) {
      char c = Serial1.read();
      logCapture(String(c));
      if (c == '>') {
        gotPrompt = true;
        break;
      }
    }

  }
  
  if (!gotPrompt) {
    logCaptureLn(String("未收到>提示符"));
    return false;
  }
  
  // 发送PDU数据
  Serial1.print(pdu.getSMS());
  Serial1.write(0x1A);  // Ctrl+Z 结束
  
  // 等待响应
  start = millis();
  String resp = "";
  while (millis() - start < 30000) {
    while (Serial1.available()) {
      char c = Serial1.read();
      resp += c;
      logCapture(String(c));
      if (resp.indexOf("OK") >= 0) {
        logCaptureLn(String("\n短信发送成功"));
        return true;
      }
      if (resp.indexOf("ERROR") >= 0) {
        logCaptureLn(String("\n短信发送失败"));
        return false;
      }
    }

  }
  logCaptureLn(String("短信发送超时"));
  return false;
}
