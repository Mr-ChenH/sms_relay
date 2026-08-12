package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestNextSubscriptionRun(t *testing.T) {
	start := time.Date(2026, time.August, 1, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{name: "next interval", now: start, want: start.Add(30 * 24 * time.Hour)},
		{name: "skip expired intervals", now: start.Add(75 * 24 * time.Hour), want: start.Add(90 * 24 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextSubscriptionRun(start, 30, tt.now)
			if !got.Equal(tt.want) {
				t.Fatalf("nextSubscriptionRun() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestKeepaliveRunSwitchesSendsAndRestores(t *testing.T) {
	s := newKeepaliveTestStore("original")
	run, err := s.StartKeepaliveRun(keepaliveTestSubscription())
	if err != nil {
		t.Fatalf("StartKeepaliveRun() error = %v", err)
	}

	s.AdvanceKeepaliveRuns()
	run = s.keepaliveRuns[0]
	assertRunStage(t, s, run, "switching_to_target", "esim_enable_profile")
	completeCommandForTest(s, run.CommandID, "succeeded", "switched")
	s.devices[0].ICCID = "target"

	s.AdvanceKeepaliveRuns()
	s.AdvanceKeepaliveRuns()
	run = s.keepaliveRuns[0]
	assertRunStage(t, s, run, "sending_sms", "send_sms")
	completeCommandForTest(s, run.CommandID, "succeeded", "sms sent")

	s.AdvanceKeepaliveRuns()
	s.AdvanceKeepaliveRuns()
	run = s.keepaliveRuns[0]
	assertRunStage(t, s, run, "restoring_profile", "esim_enable_profile")
	completeCommandForTest(s, run.CommandID, "succeeded", "restored")
	s.devices[0].ICCID = "original"

	completed := s.AdvanceKeepaliveRuns()
	if len(completed) != 1 || completed[0].Stage != "completed" {
		t.Fatalf("completed runs = %#v", completed)
	}
	if len(s.commands) != 3 {
		t.Fatalf("commands = %d, want switch/send/restore", len(s.commands))
	}
}

func TestKeepaliveRunRestoresAfterSMSFailure(t *testing.T) {
	s := newKeepaliveTestStore("target")
	_, err := s.StartKeepaliveRun(keepaliveTestSubscription())
	if err != nil {
		t.Fatalf("StartKeepaliveRun() error = %v", err)
	}
	s.keepaliveRuns[0].OriginalICCID = "original"

	s.AdvanceKeepaliveRuns()
	run := s.keepaliveRuns[0]
	completeCommandForTest(s, run.CommandID, "succeeded", "switched")
	s.AdvanceKeepaliveRuns()
	s.AdvanceKeepaliveRuns()
	run = s.keepaliveRuns[0]
	completeCommandForTest(s, run.CommandID, "failed", "network rejected")

	s.AdvanceKeepaliveRuns()
	s.AdvanceKeepaliveRuns()
	run = s.keepaliveRuns[0]
	if run.Stage != "restoring_profile" || run.Error == "" {
		t.Fatalf("run after SMS failure = %#v", run)
	}
}

func newKeepaliveTestStore(iccid string) *Store {
	return &Store{devices: []model.Device{{ID: "dev-1", Name: "terminal", ICCID: iccid}}}
}

func keepaliveTestSubscription() model.EsimSubscription {
	return model.EsimSubscription{ID: "sub-1", ProfileID: "profile-1", DeviceID: "dev-1", ICCID: "target", KeepaliveNumber: "10086", KeepaliveMessage: "CXLL", TargetIDs: []string{"target-1"}}
}

func completeCommandForTest(s *Store, id, status, result string) {
	for i := range s.commands {
		if s.commands[i].ID == id {
			s.commands[i].Status = status
			s.commands[i].Result = result
			return
		}
	}
}

func assertRunStage(t *testing.T, s *Store, run model.EsimKeepaliveRun, stage, commandType string) {
	t.Helper()
	if run.Stage != stage {
		t.Fatalf("stage = %s, want %s", run.Stage, stage)
	}
	if run.CommandID == "" {
		t.Fatal("command ID is empty")
	}
	command, ok := s.findCommandLocked(run.CommandID)
	if !ok || command.Type != commandType {
		t.Fatalf("command = %#v, want type %s", command, commandType)
	}
}
