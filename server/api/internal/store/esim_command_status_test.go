package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestUpdateCommandStatusMarksClaimedWithoutCompleting(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "esp32-1", LastSeenAt: time.Now()}}
	s.commands = []model.DeviceCommand{{ID: "cmd-1", DeviceID: "dev-1", Type: "esim_enable_profile", Status: "pending"}}

	command, err := s.UpdateCommandStatus("cmd-1", model.TerminalCommandResultRequest{DeviceID: "esp32-1", Status: "claimed"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Status != "claimed" || command.ClaimedAt == nil || command.CompletedAt != nil {
		t.Fatalf("unexpected claimed command: %+v", command)
	}

	command, err = s.CompleteCommand("cmd-1", model.TerminalCommandResultRequest{DeviceID: "esp32-1", Status: "succeeded", Result: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Status != "succeeded" || command.CompletedAt == nil {
		t.Fatalf("unexpected completed command: %+v", command)
	}
}
