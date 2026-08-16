# SMS Hub Server

Go API + Vue 3 management console for the centralized SMS/eSIM hub.

## Structure

```text
server/
  api/       Go HTTP API service
  web/       Vue 3 + Vite management console
  apprise/   optional configuration files for a separately deployed Apprise API
  data/      bind-mounted SQLite database directory
```

## Run With Docker Compose

### Docker Hub image

The default Compose file pulls `xmoli/sms-relay:latest`. The image contains the Go API, production Vue application, embedded MQTT broker, and lpac.

```bash
cd server
docker compose pull
docker compose up -d
```

### Build from source

Use the separate build configuration to create `sms-relay:local` from the current checkout:

```bash
cd server
docker compose -f docker-compose.build.yml up -d --build
```

Both configurations expose the same ports and use the same `server/data` bind-mounted directory, so do not run them at the same time.

Services:

- SMS Hub Web and API: `http://localhost:8080`
- Embedded MQTT broker: `mqtt://localhost:1883`

The default Compose deployment does not start Apprise. Configure a separately deployed Apprise API in the management console when notification forwarding is needed.

API data is persisted in the bind-mounted `server/data` directory, so the SQLite files remain directly visible on the host. The image starts through a root entrypoint that creates `/data`, repairs ownership when necessary, and then drops privileges to UID/GID `10001` before starting SMS Hub. A fresh deployment therefore only requires `docker compose pull` followed by `docker compose up -d`; do not manually create `smshub.db` or run `chown`.

When upgrading an older deployment, pull the current image before starting it. Do not set Compose `user:`, because the entrypoint needs its initial root process to initialize bind-mount permissions.

## Publish the Docker Image

From the repository root, publish `xmoli/sms-relay:latest` with:

```bash
./scripts/publish-docker.sh
```

Pass a version to publish both `latest` and that version, for example `./scripts/publish-docker.sh 1.0.2`. The script requires a clean branch synchronized with GitHub and an existing Docker Hub login. See the root README for configuration variables and verification details.

## Run API Locally

Install the pinned lpac release once when eSIM Profile download is needed:

```bash
cd server
mise run lpac:install
```

Then start the API:

```bash
cd server/api
go run ./cmd/smshub
```

Default API address: `http://localhost:8080`.

SQLite data is persisted by default to:

- `server/data/smshub.db` when running from `server/`
- `server/data/smshub.db` when running `go run` from `server/api`

Override with:

```bash
SMS_HUB_DB_PATH=/path/to/smshub.db go run ./cmd/smshub
```

Apprise API defaults to `http://localhost:8000`. Override with:

```bash
APPRISE_BASE_URL=http://localhost:8000 go run ./cmd/smshub
```

Health check:

```bash
curl http://localhost:8080/api/health
```

## Run Web

```bash
cd server/web
npm install
npm run dev
```

Default Web address: `http://localhost:5173`.

The Vite dev server proxies `/api` to `http://localhost:8080`.

## Current Scope

This is the first runnable skeleton based on the project-factory design:

- SQLite-backed persistence for devices, SMS, commands, Apprise services/targets, routing rules, eSIM profiles/tasks/subscriptions, logs, and audit state.
- No startup demo/debug data is inserted; a new database starts empty and is populated by real terminal/API actions.
- Vue 3 console pages for overview, devices, historical SMS, sending SMS, distribution, eSIM, diagnostics, logs, and audit.
- `POST /api/admin/outbound-sms` creates a real pending device command in the persisted command queue.
- Terminal protocol endpoints are available for registration, heartbeat, SMS upload, log upload, command polling, and command result upload.
- SMS upload dispatches notifications through enabled routing rules. A rule can filter by sender substring, any body keyword, device, and SMS tag, then select one or more Apprise targets. Conditions across fields are ANDed; keywords within one rule are ORed. Multiple matching rules union and deduplicate their targets. When no structured rule is enabled, notifications continue to fan out to all enabled targets for backward compatibility. Apprise failures do not block SMS storage.

## API Surface

Admin endpoints:

- `GET /api/admin/dashboard`
- `GET /api/admin/devices`
- `GET /api/admin/sms?page=1&pageSize=50&q=...`
- `POST /api/admin/outbound-sms`
- `GET /api/admin/commands`
- `POST /api/admin/commands`
- `GET /api/admin/apprise-services`
- `POST /api/admin/apprise-services`
- `PUT /api/admin/apprise-services/{id}`
- `DELETE /api/admin/apprise-services/{id}`
- `POST /api/admin/apprise-services/test`
- `GET /api/admin/apprise-targets`
- `POST /api/admin/apprise-targets`
- `PUT /api/admin/apprise-targets/{id}`
- `DELETE /api/admin/apprise-targets/{id}`
- `POST /api/admin/notify-test`
- `GET /api/admin/routing-rules`
- `POST /api/admin/routing-rules`
- `PUT /api/admin/routing-rules/{id}`
- `DELETE /api/admin/routing-rules/{id}`
- `GET /api/admin/esim/profiles`
- `GET /api/admin/esim/tasks`
- `POST /api/admin/esim/tasks`
- `GET /api/admin/esim/subscriptions`
- `POST /api/admin/esim/subscriptions`
- `PUT /api/admin/esim/subscriptions/{id}`
- `GET /api/admin/logs`
- `GET /api/admin/audit`

Terminal MQTT topics:

SMS Hub starts an embedded MQTT broker by default, so a normal deployment only needs the `smshub` API process. The same process listens on the HTTP API address, default `:8080`, and MQTT address, default `:1883`.

Terminal SoftAP provisioning needs the local WiFi credentials and server host/IP. Power on the terminal, connect to its `SMSHub-XXXXXX` hotspot, then open `http://192.168.4.1`. Select the local WiFi, enter its password, and set the SMS Hub server address. The terminal derives `http://SERVER_HOST:8080` and `mqtt://SERVER_HOST:1883` automatically. The hotspot closes after a successful connection and stays available when connection fails.

Set `SMS_HUB_EMBEDDED_MQTT=false` and `SMS_HUB_MQTT_BROKER=tcp://host:1883` only when using an external broker.

- `sms-hub/devices/{deviceId}/register`
- `sms-hub/devices/{deviceId}/heartbeat`
- `sms-hub/devices/{deviceId}/sms`
- `sms-hub/devices/{deviceId}/logs`
- `sms-hub/devices/{deviceId}/esim/profiles`
- `sms-hub/devices/{deviceId}/commands/{commandId}/result`
- `sms-hub/devices/{deviceId}/status`
- `sms-hub/devices/{deviceId}/commands` for server-to-terminal command delivery

## WiFi OTA 固件升级

固件使用 `code/partitions.csv` 自定义双 OTA 分区。首次迁移现有终端时仍需通过 USB 烧录一次分区表和应用；NVS 起始地址和大小保持不变，因此 WiFi 与 SMS Hub 配置会保留。之后可在管理台“诊断工具 -> 终端固件”上传 `.bin` 并对在线终端执行升级。中心会从固件内置 metadata 自动识别版本和硬件型号，无需在页面手工输入；发布新版本时只需修改 `code/firmware_version.h` 中的 `SMSHUB_FIRMWARE_VERSION`。

```powershell
arduino-cli compile --fqbn esp32:esp32:makergo_c3_supermini --build-path .pi/build/firmware code
arduino-cli upload --fqbn esp32:esp32:makergo_c3_supermini --port COM4 --input-dir .pi/build/firmware code
```

中心服务将固件保存在 `/data/firmware`，下载使用 30 分钟有效且绑定终端 ID 的随机令牌。部署时必须正确设置 `SMS_HUB_PUBLIC_BASE_URL`，确保终端能访问生成的固件 URL，例如 `http://172.16.0.9:8082`。

Next phases should add terminal/admin authentication, migrate from snapshot persistence to normalized relational tables if query/reporting needs grow, implement async delivery and eSIM maintenance workers, and expand ESP32 terminal command execution coverage.
