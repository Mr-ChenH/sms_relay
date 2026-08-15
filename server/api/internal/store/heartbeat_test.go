package store

import (
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

func TestHeartbeatReplacesProfileIdentityFields(t *testing.T) {
	s := &Store{devices: []model.Device{{
		ID:          "dev-1",
		DeviceID:    "esp32-test",
		ICCID:       "old-iccid",
		Operator:    "old-operator",
		PhoneNumber: "old-number",
	}}}

	device, err := s.Heartbeat(model.TerminalHeartbeatRequest{
		DeviceID: "esp32-test",
		ICCID:    "new-iccid",
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if device.ICCID != "new-iccid" {
		t.Fatalf("ICCID = %q, want new-iccid", device.ICCID)
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
