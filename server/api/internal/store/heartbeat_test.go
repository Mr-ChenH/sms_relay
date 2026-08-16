package store

import (
	"strings"
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestDeviceStatusAllowsBriefHeartbeatGap(t *testing.T) {
	now := time.Now()
	device := model.Device{LastSeenAt: now.Add(-deviceOnlineTimeout + time.Second)}
	if status := deviceStatusAt(device, now); status != "online" {
		t.Fatalf("status = %q, want online within grace period", status)
	}
	device.LastSeenAt = now.Add(-deviceOnlineTimeout - time.Second)
	if status := deviceStatusAt(device, now); status != "offline" {
		t.Fatalf("status = %q, want offline after grace period", status)
	}
}

func TestDeviceStatusTransitionLogsOnce(t *testing.T) {
	s := newEsimTaskTestStore(t)
	now := time.Now()
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "esp32-test", Name: "Terminal", Status: "online", IP: "172.16.0.108", RSSI: -78, LastSeenAt: now.Add(-deviceOnlineTimeout - time.Second)}}

	if devices := s.Devices(); devices[0].Status != "offline" {
		t.Fatalf("device status = %q, want offline", devices[0].Status)
	}
	if len(s.logs) != 1 || s.logs[0].Level != "warn" || !strings.Contains(s.logs[0].Message, "terminal offline") {
		t.Fatalf("offline transition log = %+v", s.logs)
	}
	_ = s.Devices()
	if len(s.logs) != 1 {
		t.Fatalf("repeated status reads should not duplicate logs: %+v", s.logs)
	}

	if _, err := s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", IP: "172.16.0.108", RSSI: -72}); err != nil {
		t.Fatal(err)
	}
	if len(s.logs) != 2 || s.logs[0].Level != "info" || !strings.Contains(s.logs[0].Message, "terminal online") {
		t.Fatalf("online transition log = %+v", s.logs)
	}
	if _, err := s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", RSSI: -72}); err != nil {
		t.Fatal(err)
	}
	if len(s.logs) != 2 {
		t.Fatalf("normal heartbeat should not duplicate online logs: %+v", s.logs)
	}
}

func TestHeartbeatReplacesProfileIdentityFields(t *testing.T) {
	s := &Store{devices: []model.Device{{
		ID:          "dev-1",
		DeviceID:    "esp32-test",
		ICCID:       "old-iccid",
		Operator:    "old-operator",
		PhoneNumber: "old-number",
	}}}

	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{
		DeviceID:                   "esp32-test",
		ICCID:                      "new-iccid",
		CellularRSSI:               -85,
		CellularCSQ:                14,
		EsimFirmwareVersion:        "1.2.3",
		EsimSVN:                    "2.2.0",
		EsimProfileVersion:         "2.3.1",
		EsimGlobalPlatformVersion:  "2.3.0",
		EsimCategory:               "medium",
		EsimSASAccreditationNumber: "SAS-TEST",
		EsimInstalledApplications:  6,
		EsimFreeNVMemory:           524288,
		EsimFreeVolatileMemory:     32768,
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.ICCID != "new-iccid" {
		t.Fatalf("ICCID = %q, want new-iccid", device.ICCID)
	}
	if device.CellularRSSI != -85 || device.CellularCSQ != 14 {
		t.Fatalf("cellular signal = %d dBm / CSQ %d, want -85 / 14", device.CellularRSSI, device.CellularCSQ)
	}
	if device.EsimFirmwareVersion != "1.2.3" || device.EsimSVN != "2.2.0" || device.EsimProfileVersion != "2.3.1" ||
		device.EsimGlobalPlatformVersion != "2.3.0" || device.EsimCategory != "medium" || device.EsimSASAccreditationNumber != "SAS-TEST" ||
		device.EsimInstalledApplications != 6 || device.EsimFreeNVMemory != 524288 || device.EsimFreeVolatileMemory != 32768 {
		t.Fatalf("eSIM info was not stored: %+v", device)
	}
	if device.Operator != "" {
		t.Fatalf("Operator = %q, want empty while new profile registers", device.Operator)
	}
	if device.PhoneNumber != "" {
		t.Fatalf("PhoneNumber = %q, want empty while new profile is queried", device.PhoneNumber)
	}
}

func TestHeartbeatKeepsIdentityFieldsWhenNotYetReported(t *testing.T) {
	s := &Store{devices: []model.Device{{
		ID:          "dev-1",
		DeviceID:    "esp32-test",
		ICCID:       "same-iccid",
		Operator:    "known-operator",
		PhoneNumber: "known-number",
	}}}

	// 终端启动初期心跳可能暂不携带号码/运营商，空值应保留旧数据而非覆盖为空白
	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{
		DeviceID: "esp32-test",
		ICCID:    "same-iccid",
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.Operator != "known-operator" {
		t.Fatalf("Operator = %q, want kept known-operator", device.Operator)
	}
	if device.PhoneNumber != "known-number" {
		t.Fatalf("PhoneNumber = %q, want kept known-number", device.PhoneNumber)
	}

	// 就绪后推送的新值应正常覆盖
	device, err = s.Heartbeat(model.TerminalHeartbeatRequest{
		DeviceID:    "esp32-test",
		ICCID:       "same-iccid",
		Operator:    "new-operator",
		PhoneNumber: "new-number",
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.Operator != "new-operator" || device.PhoneNumber != "new-number" {
		t.Fatalf("Operator/PhoneNumber = %q/%q, want new values", device.Operator, device.PhoneNumber)
	}
}
