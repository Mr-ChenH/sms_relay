# SMS Hub：ESP32-C3 多终端短信与 eSIM 管理系统

基于 **ESP32-C3 + ML307A/ML307R-DC** 的自托管短信管理系统。ESP32 终端负责接收和发送短信、执行模组命令与管理 eUICC Profile；SMS Hub 服务端负责多终端管理、短信持久化、Apprise 消息分发、远程诊断、保号任务、日志与审计。

项目适合需要长期运行低成本短信终端，并希望在一个 Web 管理台中管理多台设备、多个号码和 eSIM Profile 的个人或小团队。

> 本项目涉及硬件接线、ESP32 固件编译、AT 指令和蜂窝模组调试，不是开箱即用的消费级产品。部署前请阅读[能力边界](#能力边界与已知限制)与[安全说明](#安全说明)。

![SMS Hub 管理台总览](assets/Overview.png)

## 目录

- [核心能力](#核心能力)
- [系统架构](#系统架构)
- [快速开始](#快速开始)
- [硬件准备与接线](#硬件准备与接线)
- [固件编译](#固件编译)
- [BLE 配网与终端接入](#ble-配网与终端接入)
- [管理台使用流程](#管理台使用流程)
- [消息分发](#消息分发)
- [MCP 服务](#mcp-服务)
- [eSIM 与保号](#esim-与保号)
- [配置参考](#配置参考)
- [本地开发与测试](#本地开发与测试)
- [项目目录](#项目目录)
- [故障排查](#故障排查)
- [安全说明](#安全说明)
- [能力边界与已知限制](#能力边界与已知限制)
- [贡献与许可证](#贡献与许可证)

## 核心能力

| 模块 | 能力 |
| --- | --- |
| 多终端管理 | 终端注册、在线状态、最后心跳、固件/硬件信息、号码、ICCID、EID、运营商、信号和资源状态 |
| 短信接收 | PDU 短信解析、长短信合并、断网持久队列、幂等上报、发送方与接收方号码快照 |
| 短信历史 | SQLite 持久化、发送方/接收方/内容搜索、分页、详情、CSV 导出 |
| 远程发送 | 从管理台选择终端创建发送任务，由终端领取命令并回传执行结果 |
| 消息分发 | 对接自托管 Apprise API；支持多服务、多 Target、标签、模板和结构化路由规则 |
| 诊断工具 | ATI、信号、SIM、网络注册、Ping、自定义 AT 指令、飞行模式和模组硬重启 |
| eSIM 管理 | EID/Profile 同步、当前 Profile 识别、Profile 启用与删除、任务与命令状态展示 |
| 订阅保活 | 充值提醒、短信保活、按周期切换目标 Profile、发送短信并恢复原 Profile |
| 运维能力 | 终端日志、级别/终端筛选、分页、导出；敏感操作审计、状态追踪和 CSV 导出 |
| 中心通信 | 服务端内置 MQTT Broker，终端通过 MQTT 上报状态并领取命令 |

## 系统架构

```text
┌──────────────────────────────┐
│ ESP32-C3 终端                │
│                              │
│  BLE 配网                    │
│  UART / AT / PDU             │
│  短信持久队列                │
│  eSIM ES10x / APDU           │
└──────────────┬───────────────┘
               │ MQTT :1883
               │ 注册、心跳、短信、日志、Profile、命令结果
               ▼
┌────────────────────────────────────────────────────┐
│ SMS Hub Go 服务                                    │
│                                                    │
│  HTTP API :8080       内置 MQTT Broker :1883       │
│  SQLite 持久化        命令队列 / 调度 / 审计       │
└──────────────┬───────────────────────┬─────────────┘
               │                       │ HTTP（可选）
               │ /api                  ▼
               │              ┌───────────────────┐
               │              │ 外部 Apprise API  │
               │              │ Bark / Telegram   │
               │              │ 邮件 / ntfy / ... │
               │              └───────────────────┘
               ▼
┌──────────────────────────────┐
│ Vue 3 管理台 :8080           │
│ 总览 / 终端 / 短信 / eSIM    │
│ 分发 / 诊断 / 日志 / 审计    │
└──────────────────────────────┘
```

### 关键数据流

1. ESP32 通过 UART 监听模组 URC，并使用 `pdulib` 解码 PDU 短信。
2. 长短信在终端合并；断网时写入 LittleFS 持久队列。
3. 终端向 `sms-hub/devices/{deviceId}/sms` 发布短信。
4. 服务端按 `deviceId + terminalMessageId` 去重并写入 SQLite。
5. 服务端匹配路由规则，通过 Apprise Target 分发通知。
6. 管理台创建短信、诊断或 eSIM 命令后，服务端通过 MQTT 下发。
7. 终端执行命令并回传状态，命令列表和审计记录自动更新。

## 快速开始

最快的部署方式是使用 Docker Compose 拉取 `xmoli/sms-relay:latest`。生产镜像已包含 Go API、Vue 管理台、内置 MQTT Broker 和 lpac。Apprise 是可选的外部通知服务，不随默认 Compose 启动。

### 前置条件

- Docker Engine 及 Docker Compose v2
- 可被 ESP32 访问的局域网 IP
- 空闲端口：`8080`、`1883`

### 1. 确认服务端地址

ESP32 不能访问服务器容器中的 `localhost`。记录运行 Docker 的主机局域网 IP，例如 `192.168.1.10`；后续通过 BLE 将这个主机地址写入终端。管理台中的公开配置默认根据浏览器访问地址自动推导，无需在 Compose 中设置环境变量。

### 2. 从 Docker Hub 启动

默认 Compose 文件直接拉取 `xmoli/sms-relay:latest`：

```bash
docker compose pull
docker compose up -d
```

需要基于当前源码构建时，使用独立的 build 配置：

```bash
docker compose -f docker-compose.build.yml up -d --build
```

两种方式使用相同的服务端口和 `server/data` 数据目录，不要同时启动。启动后访问：

| 服务 | 地址 | 用途 |
| --- | --- | --- |
| Web 管理台与 API | <http://localhost:8080> | 日常管理入口、管理 API 与健康检查 |
| MQTT Broker | `mqtt://localhost:1883` | ESP32 终端通信 |

默认 Compose 不启动 Apprise；需要通知转发时可另行部署 Apprise API 并在管理台配置服务地址。

健康检查：

```bash
curl http://localhost:8080/api/health
```

SQLite 数据默认保存在 Compose 文件同级的 `data/smshub.db` 中，宿主机可以直接查看和备份。容器入口会在首次启动时创建目录、修正绑定挂载权限，然后降权运行应用，无需手动执行 `mkdir`、`chown` 或 `chmod`。

> 自动权限初始化由当前镜像入口提供。升级旧部署时请先执行 `docker compose pull`；如果显式设置了 Compose `user:`，请删除该设置，否则入口无法修正挂载目录权限。

停止服务：

```bash
docker compose down
```

> `docker compose down` 不会删除 `data/` 下的 SQLite 数据。备份系统时应备份该目录；使用 Apprise 时还应备份其配置。

## 硬件准备与接线

### 推荐硬件

- ESP32-C3 SuperMini 或兼容开发板
- ML307A / ML307R-DC 核心板
- 4G/LTE 天线
- Nano SIM，或目标模组支持的 eUICC/eSIM 硬件
- 稳定的 5V 电源
- USB 数据线

实物参考：

<img src="assets/photo.png" width="240" alt="ESP32-C3 与蜂窝模组实物" />

### 默认引脚

默认定义位于 `code/globals.h`：

| ESP32-C3 | 模组 | 用途 |
| --- | --- | --- |
| GPIO3 / TX | RX | ESP32 向模组发送 AT 指令 |
| GPIO4 / RX | TX | ESP32 接收模组响应与 URC |
| GPIO5 | EN | 控制模组上下电 |
| GND | GND | 共地 |
| 5V | VCC | 模组供电 |

```text
ESP32-C3 SuperMini                 ML307A / ML307R-DC

GPIO3 (TX)  --------------------> RX
GPIO4 (RX)  <-------------------- TX
GPIO5       --------------------> EN
GND         --------------------- GND
5V          --------------------- VCC
```

注意事项：

- ESP32 与模组必须共地。
- 蜂窝模组发射瞬间电流较大，供电不足会导致随机重启、掉网或短信发送失败。
- 不要再将模组 EN 固定短接；当前固件通过 GPIO5 控制模组硬重启。
- 不同开发板的串口和 LED 引脚可能不同，烧录前检查 `code/globals.h`。

## 固件编译

当前固件是 Arduino 工程，入口文件为 `code/code.ino`。

### 环境

1. 安装 [Arduino IDE](https://www.arduino.cc/en/software)。
2. 安装 [Arduino ESP32](https://docs.espressif.com/projects/arduino-esp32/en/latest/installing.html) 开发板支持。
3. 选择 `MakerGO ESP32 C3 SuperMini` 或与你硬件匹配的 ESP32-C3 板型。
4. 将 `code/` 目录作为 Arduino sketch 打开。

国内镜像可选：

```text
https://jihulab.com/esp-mirror/espressif/arduino-esp32/-/raw/gh-pages/package_esp32_index_cn.json
```

### 依赖库

| 库 | 用途 |
| --- | --- |
| `pdulib` by David Henry | PDU 短信编解码 |
| `PubSubClient` by Nick O'Leary | MQTT 通信 |
| `ArduinoJson` | MQTT JSON 编解码和持久队列 |

BLE、WiFi、Preferences 和 LittleFS 来自 Arduino ESP32 框架。

### 烧录前检查

- `RXD`、`TXD`、`MODEM_EN` 是否与接线一致。
- 模组 UART 波特率是否为 `115200`。
- 串口监视器波特率设置为 `115200`。
- 天线已连接，SIM/eUICC 状态正常。

烧录后，终端会优先尝试已保存的 WiFi；没有有效凭据时启动 BLE 配网。为了现场修改服务器配置，BLE 广播在 WiFi 已连接时也会启动。

## BLE 配网与终端接入

### BLE 服务

| 项目 | 值 |
| --- | --- |
| 设备名 | `SMSCFG-XXXXXX`，后六位来自 MAC 地址 |
| Service UUID | `7d6d0001-5f36-4f64-8f2b-ec2a7b3d0101` |
| 写入 Characteristic | `7d6d0002-5f36-4f64-8f2b-ec2a7b3d0101` |
| 状态 Characteristic | `7d6d0003-5f36-4f64-8f2b-ec2a7b3d0101` |

可以使用 nRF Connect、LightBlue 等 BLE 调试工具连接并写入 UTF-8 文本。

### 推荐配置格式

同时设置 WiFi 与 SMS Hub 主机：

```text
SSID|PASSWORD|192.168.1.10
```

固件会自动生成：

```text
mqtt://192.168.1.10:1883
```

仅修改服务端主机：

```text
SERVER|192.168.1.10
```

使用外部 MQTT 或认证 Broker：

```text
MQTT|mqtt://broker.example.com:1883|USERNAME|PASSWORD
```

兼容的一次性完整格式：

```text
SSID|PASSWORD|mqtt://broker.example.com:1883|USERNAME|PASSWORD
```

### 常见状态返回

| 状态 | 含义 |
| --- | --- |
| `waiting_for_credentials` | 等待写入配置 |
| `received:SSID:mqtt` | 已收到 WiFi 与 MQTT 配置 |
| `connecting:SSID` | 正在连接 WiFi |
| `connected:IP` | WiFi 已连接 |
| `connect_failed` | WiFi 连接失败 |
| `server_saved:HOST` | 已保存 SMS Hub 主机 |
| `mqtt_saved:BROKER` | 已保存 MQTT Broker |

终端成功接入后，应在管理台“终端”页面看到设备在线，并逐步显示号码、ICCID、EID、运营商和信号信息。

## 管理台使用流程

总览页集中展示在线终端、当日短信、分发失败和 eSIM 任务，并通过订阅保活日历查看充值提醒、短信保活计划和近期提醒。

![SMS Hub 管理台总览](assets/Overview.png)

### 1. 确认终端在线

进入“终端”页面检查：

- 在线状态与最后心跳
- 当前号码、ICCID 和 EID
- 运营商和 RSSI
- 固件、硬件和资源信息

如果号码显示为空，部分 SIM/eSIM 可能无法通过模组读取本机号码。此时仍可通过 ICCID、EID 和运营商识别卡片。

终端页同时提供 BLE 接入参数、服务端地址检查、终端诊断入口和 eSIM/命令操作。

![SMS Hub 终端管理页面](assets/Terminal.png)

### 2. 接收与查询短信

短信入库后可在“历史短信”查看：

- 接收时间
- 接收终端
- 发送方号码
- 接收方号码快照
- 短信正文、标签和分发状态

接收号码在终端入队时保存。即使之后切换 SIM/eSIM，新短信也不会改写旧记录中的接收号码。旧版本固件产生的历史记录没有号码快照时，服务端会回退显示该终端当前已知号码。

历史短信页支持全文筛选、分页、当前页导出和右侧详情查看；详情中可核对完整内容、长短信状态及分发结果。

![SMS Hub 历史短信页面](assets/Historical%20SMS.png)

### 3. 远程发送短信

进入“发送短信”：

1. 选择发送终端，并核对当前号码、运营商与 ICCID。
2. 输入目标号码和短信内容。
3. 创建发送任务。
4. 在诊断工具或审计页面查看命令状态与结果。

终端离线时命令会保留在队列中，等待终端重新上线。

### 4. 诊断终端

“诊断工具”提供：

- 模组信息 `ATI`
- 信号查询 `AT+CSQ`
- SIM 信息和网络注册状态
- 自定义主机 Ping
- 任意 AT 指令
- 飞行模式切换
- 模组硬重启

飞行模式、硬重启和未知 AT 指令可能中断网络或当前任务。管理台会记录审计信息，但仍应谨慎操作。

## 消息分发

SMS Hub 可选使用自托管 [Apprise API](https://github.com/caronc/apprise-api) 连接通知服务。默认 Compose 不包含 Apprise；不部署时只影响通知转发，短信接收、入库、管理和发送等核心功能仍可正常使用。实际可用渠道由 Apprise 支持范围和你的 Apprise 配置决定，例如 Bark、Telegram、邮件、ntfy、Gotify、飞书兼容 Webhook 等。

配置顺序：

1. 在 Apprise API 中创建 Config Key 并配置通知 URL。
2. 在 SMS Hub“消息分发”中新增 Apprise 服务。
3. 测试服务连接。
4. 新增 Target，填写 Config Key、Tags 和消息模板。
5. 发送测试通知。
6. 按发送方、正文关键词、终端或标签创建路由规则。

### 路由语义

- 同一规则中的不同字段使用 **AND**。
- 一组正文关键词内部使用 **OR**。
- 多条匹配规则的 Target 会合并并去重。
- 没有启用结构化规则时，为兼容旧配置，短信发送到全部已启用 Target。
- Apprise 发送失败不会阻止短信写入数据库。

### Target 模板变量

```text
{{sender}}     发送方号码
{{body}}       短信正文
{{device}}     接收终端
{{timestamp}}  接收时间
```

不要将 Apprise URL、Token、Webhook 密钥或 SMTP 密码提交到 Git。`server/apprise/` 仅供单独部署 Apprise 时保存本地挂载配置，默认 Compose 不使用该目录。

## MCP 服务

SMS Hub 提供基于最新正式 MCP `2026-07-28` 的 Stateless Streamable HTTP 端点，同时兼容官方 Go SDK 支持的旧版 MCP 客户端。端点默认关闭，配置 Bearer Token 后在 `/mcp` 启用。

```dotenv
SMS_HUB_MCP_TOKEN=replace-with-a-long-random-token
SMS_HUB_MCP_ALLOW_WRITE=false
```

Docker Compose 启动后，MCP 地址为：

```text
http://localhost:8080/mcp
```

客户端必须在每次请求中发送：

```http
Authorization: Bearer replace-with-a-long-random-token
```

当前工具：

| 工具 | 类型 | 说明 |
| --- | --- | --- |
| `get_overview` | 只读 | 获取在线终端、短信流量、分发失败及 eSIM 任务概览 |
| `list_devices` | 只读 | 查询终端状态、SIM/eSIM、运营商、信号和最后在线时间 |
| `search_sms` | 只读 | 按正文、发送方、接收方或短信 ID 分页搜索历史短信 |
| `list_esim_profiles` | 只读 | 查询指定终端的 Profile、ICCID、运营商及启用状态 |
| `get_command_status` | 只读 | 按命令 ID 查询终端领取和执行结果 |
| `send_sms` | 写入 | 创建发送短信任务；仅在 `SMS_HUB_MCP_ALLOW_WRITE=true` 时可用 |
| `switch_esim_profile` | 写入 | 按已登记 ICCID 切换当前 Profile；要求终端在线并启用写操作 |
| `refresh_device_status` | 写入 | 下发标准终端状态查询命令；启用写操作后可用 |

MCP 可以访问短信正文、号码和终端信息，因此即使只启用查询工具也必须使用强随机 Token，并通过 HTTPS 或可信内网访问。不要将 `/mcp` 无鉴权暴露到公网。发送短信会产生真实费用，建议默认保持写操作关闭。

切换 Profile 前应先调用 `list_esim_profiles` 获取该终端已登记的 ICCID。`switch_esim_profile` 只接受属于目标终端且尚未启用的 Profile，并拒绝离线终端；操作会短暂中断蜂窝连接。工具返回命令 ID 后，使用 `get_command_status` 查询终端执行结果。

## eSIM 与保号

### 已支持

- 终端 EID 和 Profile 列表同步
- 当前启用 Profile 识别
- Profile 启用/切换及心跳确认
- 非当前 Profile 删除
- eSIM 操作任务、命令记录和审计
- 充值周期提醒
- 短信保活周期
- 保活时切换目标 Profile、发送短信并恢复原 Profile

### 添加/下载 Profile

Profile 下载由服务端运行的 `lpac` LPA 完成。Go API 通过 MQTT APDU 隧道访问 ESP32-C3 后面的 ML307/eUICC；`lpac` 执行 SGP.22 ES9+ HTTPS 与 ES10b 流程，包括双方认证、确认码处理、Bound Profile Package 下载、分段安装和失败会话取消。

运行前必须在 **Linux 服务端**安装带 `stdio` APDU backend 的 `lpac`。原生 Linux 开发可在 `server/` 目录执行 `mise run lpac:install`，项目会下载并校验与 Docker 镜像相同的固定版本，API 会自动发现 `server/tools/lpac/lpac`；也可以自行安装后通过 `LPAC_PATH` 指定可执行文件。Docker Compose 会在 API 镜像中构建固定版本。Windows 服务端不支持 Profile 下载，但仍支持 Profile 查询、启用、删除及其他管理功能。部署和许可证说明见 [`server/LPAC.md`](server/LPAC.md)。

下载期间服务端、MQTT 和终端必须持续在线。激活码通常只能使用一次，首次验证应使用测试 Profile，且安装期间不要断电。当前集成以 lpac 的 SGP.22 v2.2.2 兼容能力为准；更高版本新增的可选特性取决于 lpac、eUICC 和 SM-DP+ 的支持情况。

### 短信保活流程

```text
到达计划时间
  -> 记录原 ICCID
  -> 切换到目标 Profile（如需要）
  -> 等待终端心跳确认
  -> 发送保活短信
  -> 恢复原 Profile（如需要）
  -> 通过 Apprise 通知结果
```

保活操作涉及真实短信费用和 Profile 切换风险。建议先在测试卡上验证，并确保目标 Profile、保活号码和消息内容正确。

## 配置参考

### 服务端环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SMS_HUB_API_ADDR` | `:8080` | HTTP API 监听地址 |
| `SMS_HUB_DB_PATH` | 根据工作目录推导 | SQLite 数据库路径 |
| `APPRISE_BASE_URL` | `http://localhost:8000` | 默认 Apprise API 地址 |
| `SMS_HUB_EMBEDDED_MQTT` | `true` | 是否启动内置 MQTT Broker |
| `SMS_HUB_MQTT_ADDR` | `:1883` | 内置 MQTT 监听地址 |
| `SMS_HUB_MQTT_BROKER` | 内置时为 `tcp://127.0.0.1:1883` | Go MQTT Bridge 连接地址 |
| `SMS_HUB_MQTT_CLIENT_ID` | 自动生成 | MQTT Bridge Client ID |
| `SMS_HUB_MQTT_USERNAME` | 空 | 外部 MQTT 用户名 |
| `SMS_HUB_MQTT_PASSWORD` | 空 | 外部 MQTT 密码 |
| `SMS_HUB_PUBLIC_BASE_URL` | 空 | 管理台展示给终端的公开 API 地址 |
| `SMS_HUB_PUBLIC_MQTT_BROKER` | 空 | 管理台展示给终端的公开 MQTT 地址 |
| `LPAC_PATH` | `lpac` | Linux 服务端 lpac 可执行文件路径；Windows 不支持下载 |
| `TZ` | 使用系统时区；Docker Compose 默认为 `Asia/Shanghai` | 转发消息等服务端时间的本地时区，例如 `Asia/Shanghai`、`America/New_York` |
| `SMS_HUB_MCP_TOKEN` | 空 | MCP Bearer Token；为空时 `/mcp` 不启用 |
| `SMS_HUB_MCP_ALLOW_WRITE` | `false` | 是否允许 MCP 调用 `send_sms` 等写操作 |

使用外部 MQTT Broker 时：

```bash
export SMS_HUB_EMBEDDED_MQTT=false
export SMS_HUB_MQTT_BROKER=tcp://mqtt.example.com:1883
export SMS_HUB_MQTT_USERNAME=sms-hub
export SMS_HUB_MQTT_PASSWORD=change-me
```

### MQTT Topic

终端上行：

```text
sms-hub/devices/{deviceId}/register
sms-hub/devices/{deviceId}/heartbeat
sms-hub/devices/{deviceId}/sms
sms-hub/devices/{deviceId}/logs
sms-hub/devices/{deviceId}/esim/profiles
sms-hub/devices/{deviceId}/commands/{commandId}/result
sms-hub/devices/{deviceId}/status
```

服务端下行：

```text
sms-hub/devices/{deviceId}/commands
```

终端本机 HTTP 管理页面已移除。ESP32 与中心服务之间只使用 MQTT；HTTP API 仅服务管理台和管理员操作。

## 本地开发与测试

### 版本要求

- Go `1.25`
- Node.js `20`
- npm
- 可选：[mise](https://mise.jdx.dev/)

### 使用 mise

```bash
cd server
mise install
mise run install
mise run dev
```

常用任务：

```bash
mise run test   # Go 测试 + Web 类型检查/构建
mise run build  # 构建 server/smshub 与 Web dist
mise run check  # gofmt、go vet、Go 测试、Web 构建
mise run up     # 从 Docker Hub 启动 xmoli/sms-relay:latest
mise run up:build # 从当前源码构建并启动
mise run down   # 停止默认 Docker Hub 部署
```

### 不使用 mise

启动 API 和内置 MQTT：

```bash
cd server/api
go mod download
go run ./cmd/smshub
```

启动 Web 开发服务器：

```bash
cd server/web
npm ci
npm run dev
```

Vite 会将 `/api` 代理到 `http://localhost:8080`。

### 测试

```bash
# 后端
cd server/api
go test ./...
go vet ./...

# 前端
cd server/web
npm run build
```

前端没有独立的单元测试脚本；`npm run build` 会先执行 `vue-tsc` 类型检查，再执行 Vite 生产构建。

## 项目目录

```text
sms_forwarding/
├── code/                         ESP32-C3 Arduino 固件
│   ├── code.ino                  固件入口
│   ├── modem.*                   模组初始化、AT 和短信发送
│   ├── sms_process.*             PDU 接收与长短信合并
│   ├── sms_queue.*               LittleFS 短信持久队列
│   ├── terminal_client.*         MQTT 终端协议与命令执行
│   ├── wifi_manager.*            WiFi 与 BLE 配网
│   └── esim*                     eUICC Profile / ES10x / APDU
├── server/
│   ├── api/                      Go HTTP API、MQTT Bridge、Broker、SQLite
│   ├── web/                      Vue 3 + TypeScript + Vite 管理台
│   ├── apprise/                  可选的外部 Apprise 本地配置
│   ├── data/                     SQLite 数据目录
│   ├── docker-compose.yml        使用 Docker Hub 镜像部署
│   ├── docker-compose.build.yml  从当前源码构建部署
│   └── mise.toml                 开发任务与工具版本
├── docs/project-factory/         产品、界面和技术设计文档
├── dev_doc/                      早期固件架构参考文档
├── assets/                       README 图片
├── scripts/                      辅助脚本
└── LICENSE                       MIT License
```

> `dev_doc/` 中部分文档描述的是终端本机 Web 页面和旧推送架构，仅用于理解历史实现。当前架构以根 README、`server/README.md` 和源码为准。

## 故障排查

### 数据库无法打开或显示只读

当前镜像会在启动时自动初始化 `./data:/data` 的所有权，然后以 UID/GID `10001` 运行应用。如果日志出现 `unable to open database file` 或 `attempt to write a readonly database`：

1. 执行 `docker compose pull`，确认使用包含自动权限初始化入口的最新镜像。
2. 检查 Compose 中没有设置 `user:` 或 `read_only: true`。
3. 检查挂载保持为 `./data:/data`，且宿主文件系统允许容器 root 执行 `chown`。
4. 执行 `docker compose up -d --force-recreate` 后重新查看日志。

正常部署不需要手动创建 `data/smshub.db`，也不需要在宿主机运行 `chown` 或 `chmod`。

### 管理台显示公开地址为 localhost

不要通过 `http://localhost:8080` 打开管理台，改用 ESP32 可访问的服务器地址，例如 `http://192.168.1.10:8080`。服务端会根据浏览器请求地址自动推导公开 API 和 MQTT 地址。

只有在反向代理导致自动推导不正确时，才需要为 API 容器额外设置：

```yaml
environment:
  TZ: "${TZ:-Asia/Shanghai}"
  SMS_HUB_PUBLIC_BASE_URL: "http://服务器局域网IP:8080"
  SMS_HUB_PUBLIC_MQTT_BROKER: "mqtt://服务器局域网IP:1883"
```

修改后重启 Compose。

### 管理台能打开，但终端始终离线

检查：

1. ESP32 与服务器是否处于可互通网络。
2. 服务器防火墙是否放行 TCP `1883`。
3. BLE 中保存的 Broker 是否为服务器真实 IP，而不是 `localhost`。
4. ESP32 串口是否出现 `MQTT 已连接`。
5. 设备名是否为预期的 `SMSCFG-XXXXXX`。

### 终端在线，但号码为空

部分 SIM 不在卡内保存 MSISDN，模组无法通过 AT 指令读取号码。这不是服务端故障。可使用 ICCID、EID、运营商和 Profile 名称识别卡片。

### 短信已入库，但没有通知

按顺序检查：

1. 外部 Apprise API 是否已经单独部署并可访问。
2. SMS Hub 中配置的 Apprise 服务地址是否正确，连接测试是否成功。
3. Target 的 Config Key 和 Tags 是否存在。
4. Target 是否启用。
5. 路由规则是否匹配发送方、关键词、终端和标签。
6. 日志页面是否有 Apprise HTTP 错误。

### 命令一直等待领取

- 终端必须在线并订阅 `sms-hub/devices/{deviceId}/commands`。
- 检查 MQTT Broker 与认证配置。
- 查看终端串口、管理台日志和审计。
- 终端短暂离线时命令会等待重连，不会立即丢弃。

### 模组频繁掉线或重启

优先检查电源、天线和接地，而不是软件：

- 使用稳定 5V 供电。
- 避免仅依赖电流能力不足的 USB 转串口供电。
- 缩短供电线并确保共地。
- 确认天线连接可靠。

### eSIM 下载任务失败

按任务中心显示的阶段定位：

- Windows 平台提示不支持：将 API 部署到 Linux 或使用 Docker Compose。
- `lpac executable not found`：在 `server/` 执行 `mise run lpac:install` 后重启 API，或设置 `LPAC_PATH`。
- APDU timeout：检查终端在线状态、MQTT 连通性和 ML307 串口。
- ES9+ / SM-DP+ 错误：检查服务器 DNS、时间、系统 CA、激活码和确认码。
- ES10b 安装错误：检查 eUICC 剩余空间、Profile 策略和兼容性。

完整部署要求与安全限制见 [`server/LPAC.md`](server/LPAC.md)。

## 安全说明

当前项目默认面向受信任局域网和开发环境：

- 管理 API 尚未实现管理员认证和角色权限。
- MQTT 默认可无认证运行。
- Docker Compose 默认将 API、MQTT 和 Web 端口暴露到主机；另行部署 Apprise 时也应限制其访问范围。
- AT 指令、飞行模式、硬重启、Profile 删除和短信发送属于敏感操作。

不要在未增加保护的情况下直接暴露到公网。生产部署至少应：

1. 使用防火墙限制来源 IP。
2. 在 Web/API 前增加 HTTPS 反向代理和身份认证。
3. 为 MQTT 启用 TLS、用户名、密码和 ACL；仅允许 API 使用终端 APDU request topic。
4. Docker 部署需保护 `server/data/smshub.db`；原生部署同样需要保护实际配置的数据库文件；使用 Apprise 时还要保护其配置目录。
5. 定期备份 SQLite 与通知配置。
6. 不在日志、Issue 或截图中公开短信内容、激活码、Webhook 和 Token。

## 能力边界与已知限制

- 当前主要目标是短信接收、转发、发送、诊断、多终端管理和 eSIM Profile 运维，不支持语音通话或拨号业务。
- eSIM Profile 下载仅支持 Linux/Docker 服务端，并依赖 lpac、稳定 MQTT/APDU 隧道及目标 eUICC/SM-DP+ 对 SGP.22 流程的兼容性；Windows 服务端仅支持现有 Profile 管理。
- SQLite 当前保存整个应用状态快照。适合个人和小规模终端；当查询量和并发显著增长时，应迁移为规范化数据表。
- 管理端与终端认证尚未完成，不建议裸露公网。
- 真实短信、eSIM 和模组行为受运营商、SIM、固件版本和硬件实现影响，必须使用目标硬件验证。
- 长短信最多缓存 10 个分段，默认等待 30 秒后处理超时。
- 终端短信持久队列容量为 32 条；长时间离线且持续收信可能耗尽队列。
- 当前固件针对 ESP32-C3 与 ML307 系列编写，其他模组需要适配 AT、URC、PDU 和 eSIM 命令。

## 贡献与许可证

欢迎提交 Issue 和 Pull Request。提交代码前建议运行：

```bash
cd server/api
gofmt -w ./cmd ./internal
go vet ./...
go test ./...

cd ../web
npm run build
```

涉及固件的变更请说明：

- 使用的 ESP32 板型和 Arduino ESP32 版本
- 模组型号与固件版本
- SIM/eSIM 类型
- 串口日志和复现步骤
- 是否完成真实短信收发或 eSIM 硬件验证

本项目基于 [chenxuuu/sms_forwarding](https://github.com/chenxuuu/sms_forwarding) 演进，感谢原项目作者和贡献者。

项目使用 [MIT License](LICENSE)。
