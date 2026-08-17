package main

import (
	"testing"
	"time"
)

func TestConfigureTimezoneUsesTZEnvironment(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	t.Setenv("TZ", "Asia/Shanghai")

	name, err := configureTimezone()
	if err != nil {
		t.Fatal(err)
	}
	if name != "Asia/Shanghai" {
		t.Fatalf("timezone name = %q", name)
	}
	_, offset := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.Local).Zone()
	if offset != 8*60*60 {
		t.Fatalf("timezone offset = %d, want +08:00", offset)
	}
}

func TestConfigureTimezoneRejectsInvalidTZ(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	t.Setenv("TZ", "Not/A-Timezone")
	if _, err := configureTimezone(); err == nil {
		t.Fatal("expected invalid TZ to fail")
	}
}
