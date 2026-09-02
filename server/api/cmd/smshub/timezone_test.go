package main

import (
	"testing"
	"time"
)

func TestConfigureTimezoneDefaultsToAsiaShanghai(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	t.Setenv("TZ", "")
	t.Setenv("SMS_HUB_TIMEZONE", "")

	name, err := configureTimezone()
	if err != nil {
		t.Fatal(err)
	}
	if name != "Asia/Shanghai" {
		t.Fatalf("timezone name = %q", name)
	}
	_, offset := time.Now().In(time.Local).Zone()
	if offset != 8*60*60 {
		t.Fatalf("timezone offset = %d, want +08:00", offset)
	}
}

func TestConfigureTimezoneUsesTZEnvironment(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("SMS_HUB_TIMEZONE", "")

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

func TestConfigureTimezoneUsesExplicitHubTimezone(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	t.Setenv("TZ", "UTC")
	t.Setenv("SMS_HUB_TIMEZONE", "Asia/Tokyo")

	name, err := configureTimezone()
	if err != nil {
		t.Fatal(err)
	}
	if name != "Asia/Tokyo" {
		t.Fatalf("timezone name = %q", name)
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
