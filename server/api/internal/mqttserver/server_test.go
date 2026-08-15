package mqttserver

import (
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"

	"sms-forwarding/server/api/internal/model"
	"sms-forwarding/server/api/internal/store"
)

func TestDisconnectDoesNotImmediatelyMarkTerminalOffline(t *testing.T) {
	s := newTestStore(t)
	device, err := s.RegisterTerminal(model.TerminalRegisterRequest{DeviceID: "esp32-test"})
	if err != nil {
		t.Fatal(err)
	}

	hook := &logHook{store: s}
	hook.OnDisconnect(&mqtt.Client{ID: "esp32-test"}, nil, false)

	devices := s.Devices()
	if len(devices) != 1 || devices[0].ID != device.ID || devices[0].Status != "online" {
		t.Fatalf("brief disconnect should remain online: %+v", devices)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewSQLiteStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
