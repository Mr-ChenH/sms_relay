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
	if task.AuditID == "" || task.Status != "pending" || task.CreatedAt.IsZero() || len(task.History) != 1 {
		t.Fatalf("unexpected task: %+v", task)
	}
	if err := s.UpdateEsimTask(task.ID, "running", "installing", 82); err != nil {
		t.Fatalf("UpdateEsimTask() error = %v", err)
	}
	if err := s.UpdateEsimTask(task.ID, "running", "installing", 82); err != nil {
		t.Fatalf("UpdateEsimTask() duplicate error = %v", err)
	}
	if err := s.UpdateEsimTask(task.ID, "failed", "download failed", 0); err != nil {
		t.Fatalf("UpdateEsimTask() failed update error = %v", err)
	}
	if got := s.esimTasks[0]; got.Status != "failed" || got.Progress != 0 || got.Stage != "download failed" || len(got.History) != 3 {
		t.Fatalf("unexpected updated task: %+v", got)
	}
	if got := s.esimTasks[0].History; got[0].Stage != "等待 LPA 启动" || got[1].Stage != "installing" || got[2].Stage != "download failed" {
		t.Fatalf("unexpected task history: %+v", got)
	}
	if len(s.audit) != 1 || s.audit[0].Result != "failed" {
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
