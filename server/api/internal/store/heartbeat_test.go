package store

import (
	"strings"
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestClearLogsPersistsEmptyLogList(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.logs = []model.LogEntry{{ID: "log-1", Message: "test"}}

	s.ClearLogs()
	if logs := s.Logs(); len(logs) != 0 {
		t.Fatalf("Logs() length = %d, want 0", len(logs))
	}
}

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
		ID: "dev-1", DeviceID: "esp32-test", ICCID: "old-iccid", Operator: "old-operator", PhoneNumber: "old-number",
	}}}

	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", ICCID: "new-iccid", CellularRSSI: -85, CellularCSQ: 14, EsimFirmwareVersion: "1.2.3", EsimSVN: "2.2.0", EsimProfileVersion: "2.3.1", EsimGlobalPlatformVersion: "2.3.0", EsimCategory: "medium", EsimSASAccreditationNumber: "SAS-TEST", EsimInstalledApplications: 6, EsimFreeNVMemory: 524288, EsimFreeVolatileMemory: 32768})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.ICCID != "new-iccid" || device.CellularRSSI != -85 || device.CellularCSQ != 14 {
		t.Fatalf("identity or signal was not stored: %+v", device)
	}
	if device.Operator != "" || device.PhoneNumber != "" {
		t.Fatalf("old identity was retained: operator=%q phone=%q", device.Operator, device.PhoneNumber)
	}
}

func TestHeartbeatClearsOldPhoneWhenNewProfileHasNoNumber(t *testing.T) {
	s := &Store{devices: []model.Device{{ID: "dev-1", DeviceID: "esp32-test", ICCID: "old-iccid", Operator: "old-operator", PhoneNumber: "old-number"}}}
	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", ICCID: "new-iccid"})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.Operator != "" || device.PhoneNumber != "" {
		t.Fatalf("new profile retained old identity: operator=%q phone=%q", device.Operator, device.PhoneNumber)
	}
}

func TestHeartbeatKeepsIdentityFieldsWhenNotYetReported(t *testing.T) {
	s := &Store{devices: []model.Device{{ID: "dev-1", DeviceID: "esp32-test", ICCID: "same-iccid", Operator: "known-operator", PhoneNumber: "known-number"}}}
	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", ICCID: "same-iccid"})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.Operator != "known-operator" || device.PhoneNumber != "known-number" {
		t.Fatalf("identity was not preserved: operator=%q phone=%q", device.Operator, device.PhoneNumber)
	}

	device, err = s.Heartbeat(model.TerminalHeartbeatRequest{DeviceID: "esp32-test", ICCID: "same-iccid", Operator: "new-operator", PhoneNumber: "new-number"})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.Operator != "new-operator" || device.PhoneNumber != "new-number" {
		t.Fatalf("identity was not updated: operator=%q phone=%q", device.Operator, device.PhoneNumber)
	}
}
