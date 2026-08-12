package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestStoreTerminalSMSSnapshotsRecipient(t *testing.T) {
	s := &Store{devices: []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "terminal", PhoneNumber: "+8613800138000"}}}

	message, inserted, err := s.StoreTerminalSMS(model.TerminalSMSRequest{
		DeviceID:          "dev-1",
		TerminalMessageID: "message-1",
		Sender:            "10086",
		Body:              "test",
		Timestamp:         time.Now(),
	})
	if err != nil {
		t.Fatalf("StoreTerminalSMS() error = %v", err)
	}
	if !inserted || message.Recipient != "+8613800138000" {
		t.Fatalf("StoreTerminalSMS() = %#v, inserted=%v", message, inserted)
	}

	s.devices[0].PhoneNumber = "+8613900139000"
	list := s.SMS("", 1, 10)
	if len(list.Items) != 1 || list.Items[0].Recipient != "+8613800138000" {
		t.Fatalf("SMS() recipient changed after device update: %#v", list.Items)
	}
}

func TestSMSPopulatesLegacyRecipientAndSearchesIt(t *testing.T) {
	s := &Store{
		devices: []model.Device{{ID: "dev-1", Name: "terminal", PhoneNumber: "+8613800138000"}},
		sms:     []model.SMSMessage{{ID: "sms-1", DeviceID: "dev-1", DeviceName: "terminal", Sender: "10086", Body: "test", Timestamp: time.Now()}},
	}

	list := s.SMS("13800138000", 1, 10)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Recipient != "+8613800138000" {
		t.Fatalf("SMS() legacy recipient search = %#v", list)
	}
	if s.sms[0].Recipient != "" {
		t.Fatalf("SMS() mutated persisted legacy message: %#v", s.sms[0])
	}
}
