package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestCompleteCommandUpdatesAuditResult(t *testing.T) {
	s := &Store{devices: []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "terminal"}}}
	command, err := s.CreateDeviceCommand(model.CreateDeviceCommandRequest{
		DeviceID: "dev-1",
		Type:     "query_signal",
		Payload:  map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("CreateDeviceCommand() error = %v", err)
	}

	_, err = s.CompleteCommand(command.ID, model.TerminalCommandResultRequest{
		DeviceID: "dev-1",
		Status:   "succeeded",
		Result:   "signal ok",
	})
	if err != nil {
		t.Fatalf("CompleteCommand() error = %v", err)
	}

	audit := s.Audit()
	if len(audit) != 1 {
		t.Fatalf("Audit() length = %d, want 1", len(audit))
	}
	if audit[0].CommandID != command.ID || audit[0].Result != "succeeded" {
		t.Fatalf("Audit() row = %#v, want command %s succeeded", audit[0], command.ID)
	}
}

func TestAuditResolvesLegacyPendingCommand(t *testing.T) {
	createdAt := time.Now()
	s := &Store{
		devices: []model.Device{{ID: "dev-1", Name: "terminal"}},
		commands: []model.DeviceCommand{{
			ID: "cmd-legacy", DeviceID: "dev-1", Type: "send_sms",
			Status: "failed", CreatedAt: createdAt,
		}},
		audit: []model.AuditLog{{
			ID: "audit-legacy", Actor: "admin", DeviceName: "terminal",
			Action: "send_sms", Result: "pending", CreatedAt: createdAt,
		}},
	}

	audit := s.Audit()
	if len(audit) != 1 || audit[0].CommandID != "cmd-legacy" || audit[0].Result != "failed" {
		t.Fatalf("Audit() legacy row = %#v", audit)
	}
}
