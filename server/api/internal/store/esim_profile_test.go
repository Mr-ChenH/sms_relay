package store

import (
	"testing"
	"time"

	"sms-forwarding/server/api/internal/model"
)

func TestReplaceTerminalEsimProfilesMarksMissingAndKeepsSubscription(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "Terminal 1"}}

	profiles, err := s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{DeviceID: "terminal-1", Profiles: []model.TerminalEsimProfileInput{{ICCID: "8901", State: "enabled"}}})
	if err != nil || len(profiles) != 1 {
		t.Fatalf("initial sync = %+v, %v", profiles, err)
	}
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", ProfileID: profiles[0].ID, DeviceID: "dev-1", Enabled: true, Status: "scheduled"}}

	if _, err := s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{DeviceID: "terminal-1"}); err != nil {
		t.Fatal(err)
	}
	if len(s.esimProfiles) != 1 || s.esimProfiles[0].Available || s.esimProfiles[0].State != "missing" || s.esimProfiles[0].MissingSince.IsZero() {
		t.Fatalf("profile should be retained as missing: %+v", s.esimProfiles)
	}
	if s.esimSubscriptions[0].Status != "profile_missing" {
		t.Fatalf("subscription should report missing profile: %+v", s.esimSubscriptions[0])
	}
}

func TestReplaceTerminalEsimProfilesMovesProfileAndSubscription(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{
		{ID: "dev-1", DeviceID: "terminal-1", Name: "Terminal 1"},
		{ID: "dev-2", DeviceID: "terminal-2", Name: "Terminal 2"},
	}
	profiles, err := s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{DeviceID: "terminal-1", Profiles: []model.TerminalEsimProfileInput{{ICCID: "8901", State: "disabled"}}})
	if err != nil {
		t.Fatal(err)
	}
	originalID := profiles[0].ID
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", ProfileID: originalID, DeviceID: "dev-1", DeviceName: "Terminal 1", Enabled: true, Status: "scheduled"}}

	moved, err := s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{DeviceID: "terminal-2", Profiles: []model.TerminalEsimProfileInput{{ICCID: "8901", State: "enabled"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.esimProfiles) != 1 || moved[0].ID != originalID || moved[0].DeviceID != "dev-2" || !moved[0].Available {
		t.Fatalf("profile was not moved in place: %+v", s.esimProfiles)
	}
	if sub := s.esimSubscriptions[0]; sub.DeviceID != "dev-2" || sub.DeviceName != "Terminal 2" || sub.Status != "scheduled" {
		t.Fatalf("subscription did not follow profile: %+v", sub)
	}
}

func TestReplaceTerminalEsimProfilesUsesHeartbeatICCIDAsActiveState(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.devices = []model.Device{{ID: "dev-1", DeviceID: "terminal-1", Name: "Terminal 1", ICCID: "8931083924053625066F"}}

	profiles, err := s.ReplaceTerminalEsimProfiles(model.TerminalEsimProfilesRequest{DeviceID: "terminal-1", Profiles: []model.TerminalEsimProfileInput{
		{ICCID: "8931083924053625066", State: "disabled"},
		{ICCID: "89636626000100558682", State: "disabled"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if profiles[0].State != "enabled" || profiles[1].State != "disabled" {
		t.Fatalf("profile states = %q/%q, want enabled/disabled", profiles[0].State, profiles[1].State)
	}
}

func TestDueEsimSubscriptionsSkipsMissingProfile(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", Enabled: true, Status: "profile_missing", NextRunAt: time.Now().Add(-time.Hour), IntervalDays: 30}}
	if due := s.ClaimDueEsimSubscriptions(time.Now()); len(due) != 0 {
		t.Fatalf("missing profile subscription should not run: %+v", due)
	}
}

func TestUpdateEsimProfileMetadata(t *testing.T) {
	s := newEsimTaskTestStore(t)
	s.esimProfiles = []model.EsimProfile{{ID: "profile-1", ICCID: "8901", Available: true, LastSeenAt: time.Now()}}
	s.esimSubscriptions = []model.EsimSubscription{{ID: "sub-1", ProfileID: "profile-1"}}

	profile, err := s.UpdateEsimProfile("profile-1", model.UpdateEsimProfileRequest{Country: "中国", PhoneNumber: "+8613800138000"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Country != "中国" || profile.PhoneNumber != "+8613800138000" || s.esimSubscriptions[0].Country != "中国" {
		t.Fatalf("metadata not updated: %+v / %+v", profile, s.esimSubscriptions[0])
	}
}
