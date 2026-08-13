package store

import (
	"testing"

	"sms-forwarding/server/api/internal/model"
)

func TestUpdateDeviceNameUpdatesCachedDisplayNames(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "terminal-1"}}
	s.sms = []model.SMSMessage{{ID: "sms-1", DeviceID: "dev-1", DeviceName: "terminal-1"}}
	s.logs = []model.LogEntry{{ID: "log-1", DeviceID: "dev-1", DeviceName: "terminal-1"}}
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", DeviceID: "dev-1", DeviceName: "terminal-1"}}

	device, err := s.UpdateDevice("dev-1", model.UpdateDeviceRequest{Name: "办公室终端"})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name != "办公室终端" || s.sms[0].DeviceName != "办公室终端" || s.logs[0].DeviceName != "办公室终端" || s.esimSubscriptions[0].DeviceName != "办公室终端" {
		t.Fatalf("cached names were not updated: %+v %+v %+v %+v", device, s.sms[0], s.logs[0], s.esimSubscriptions[0])
	}
}

func TestRegisterTerminalDoesNotOverwriteAdminName(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "办公室终端"}}

	device, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "terminal-1", Name: "terminal-1"})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name != "办公室终端" {
		t.Fatalf("admin name was overwritten: %+v", device)
	}
}

func TestUpdateDeviceRejectsEmptyName(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "terminal-1"}}
	if _, err := s.UpdateDevice("dev-1", model.UpdateDeviceRequest{Name: "  "}); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
}
