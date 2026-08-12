package store

import (
	"strings"
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestCreateAndUpdateEsimTask(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "device-1", DeviceID: "terminal-1", Name: "Terminal", Status: "online", LastSeenAt: time.Now()}}

	task, err := s.CreateEsimTask(model.CreateEsimTaskRequest{DeviceID: "device-1", ActivationCode: "LPA:1$smdp.example$matching"})
	if err != nil {
		t.Fatalf("CreateEsimTask() error = %v", err)
	}
	if task.AuditID == "" || task.Status != "pending" {
		t.Fatalf("unexpected task: %+v", task)
	}
	if err := s.UpdateEsimTask(task.ID, "running", "installing", 82); err != nil {
		t.Fatalf("UpdateEsimTask() error = %v", err)
	}
	if got := s.esimTasks[0]; got.Status != "running" || got.Progress != 82 || got.Stage != "installing" {
		t.Fatalf("unexpected updated task: %+v", got)
	}
	if len(s.audit) != 1 || s.audit[0].Result != "running" {
		t.Fatalf("unexpected audit: %+v", s.audit)
	}
}

func TestCreateEsimTaskRejectsInvalidActivationCode(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "device-1", Status: "online", LastSeenAt: time.Now()}}
	_, err := s.CreateEsimTask(model.CreateEsimTaskRequest{DeviceID: "device-1", ActivationCode: "not-lpa"})
	if err == nil || !strings.Contains(err.Error(), "LPA:1$") {
		t.Fatalf("expected activation code error, got %v", err)
	}
}

func TestFindDeviceReturnsMQTTIdentifier(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-101", DeviceID: "esp32-abc"}}
	device, ok := s.FindDevice("dev-101")
	if !ok || device.DeviceID != "esp32-abc" {
		t.Fatalf("FindDevice() = %+v, %v", device, ok)
	}
}

func newEsimTaskTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewSQLiteStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Close() })
	return s
}
