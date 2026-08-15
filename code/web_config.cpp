#include "web_config.h"
#include "wifi_manager.h"
#include "config.h"
#include "logger.h"
#include <WebServer.h>
#include <WiFi.h>

static WebServer server(80);
static bool active = false;

// 配网页（中文，移动端友好，自包含，无需外部资源）
static const char PAGE_HTML[] PROGMEM = R"rawliteral(<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>短信终端配网</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;max-width:420px;margin:24px auto;padding:0 16px;color:#222}
h1{font-size:20px}
label{display:block;margin:14px 0 4px;font-size:14px;color:#555}
input,select{width:100%;box-sizing:border-box;padding:10px;border:1px solid #ccc;border-radius:6px;font-size:15px}
button{width:100%;padding:12px;margin-top:18px;background:#2b7de9;color:#fff;border:0;border-radius:6px;font-size:16px}
#status{margin:10px 0;padding:8px;border-radius:6px;background:#f4f6f8;font-size:13px;white-space:pre-line}
.hint{font-size:12px;color:#888;margin-top:4px}
</style>
</head>
<body>
<h1>短信终端配网</h1>
<div id="status">正在扫描附近 WiFi...</div>
<form method="post" action="/save">
<label>WiFi 名称</label>
<select name="ssid" id="ssid"><option value="">选择或手动输入</option></select>
<input name="ssid_custom" placeholder="或手动输入 WiFi 名称">
<label>WiFi 密码</label>
<input type="password" name="password" placeholder="密码（开放网络可留空）">
<label>SMS Hub 服务器地址</label>
<input name="hub" placeholder="例如 mqtt://192.168.1.10:1883 或 192.168.1.10">
<div class="hint">留空表示仅配置 WiFi（不修改服务器地址）。</div>
<button type="submit">保存并连接</button>
</form>
<script>
fetch('/scan').then(function(r){return r.json()}).then(function(d){
  var s=document.getElementById('ssid'),st=document.getElementById('status');
  if(d.networks&&d.networks.length){d.networks.forEach(function(n){var o=document.createElement('option');o.value=n;o.text=n;s.appendChild(o)});
    st.textContent='扫描到 '+d.networks.length+' 个 WiFi，请选择或手动输入。';}
  else{st.textContent='未扫描到 WiFi（或扫描失败），请手动输入名称。';}
}).catch(function(){document.getElementById('status').textContent='扫描失败，请手动输入 WiFi 名称。'});
</script>
</body>
</html>
)rawliteral";

static void sendHTML(int code, const String& body) {
  server.sendHeader("Cache-Control", "no-store");
  server.send(code, "text/html; charset=utf-8", body);
}

static void handleRoot() {
  sendHTML(200, FPSTR(PAGE_HTML));
}

static void handleScan() {
  // 同步扫描（配网场景下可接受短暂阻塞），按信号强度排序
  int count = WiFi.scanNetworks();
  String json = "{\"networks\":[";
  bool first = true;
  for (int i = 0; i < count; i++) {
    String ssid = WiFi.SSID(i);
    ssid.trim();
    if (ssid.length() == 0) continue;
    if (!first) json += ",";
    first = false;
    json += "\"" + ssid + "\"";
  }
  json += "]}";
  WiFi.scanDelete();
  server.sendHeader("Cache-Control", "no-store");
  server.send(200, "application/json; charset=utf-8", json);
}

static void handleSave() {
  String ssid = server.arg("ssid");
  String ssidCustom = server.arg("ssid_custom");
  ssid.trim();
  ssidCustom.trim();
  if (ssidCustom.length() > 0) ssid = ssidCustom;  // 手动输入优先
  if (ssid.length() == 0) {
    sendHTML(400, "<meta charset='utf-8'><h2>WiFi 名称不能为空</h2><p><a href='/'>返回</a></p>");
    return;
  }
  String password = server.arg("password");
  String hub = server.arg("hub");
  hub.trim();
  saveProvisionedConfig(ssid, password, hub);
  logCaptureLn(String("配网页已保存: SSID=") + ssid + ", hub=" + hub);

  String body = "<meta charset='utf-8'><h2>配置已保存</h2><p>正在连接 WiFi：<b>" + ssid + "</b></p>";
  if (hub.length() > 0) body += "<p>SMS Hub 服务器：<b>" + hub + "</b></p>";
  body += "<p>连接成功后本页面将无法访问（热点关闭）。若连接失败，热点会保持开启，可重新配置。</p><p><a href='/'>返回配网页</a></p>";
  sendHTML(200, body);
}

void webConfigInit() {
  if (active) return;
  active = true;
  server.on("/", HTTP_GET, handleRoot);
  server.on("/scan", HTTP_GET, handleScan);
  server.on("/save", HTTP_POST, handleSave);
  server.begin();
  logCaptureLn(String("配网页面已启动: http://192.168.4.1"));
}

void webConfigLoop() {
  if (!active) return;
  server.handleClient();
}
