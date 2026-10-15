package tools

import (
	"testing"
	"time"
)

func TestNormalizeDiscoverySchedule(t *testing.T) {
	if gotTime, gotWeekday, err := NormalizeDiscoverySchedule(TriggerDaily, "", 0); err != nil || gotTime != "09:00" || gotWeekday != 1 {
		t.Fatalf("default schedule = %q/%d, err=%v", gotTime, gotWeekday, err)
	}
	if _, _, err := NormalizeDiscoverySchedule(TriggerDaily, "9:00", 1); err == nil {
		t.Fatal("non-zero-padded schedule time should be rejected")
	}
	if _, _, err := NormalizeDiscoverySchedule(TriggerWeekly, "23:59", 7); err != nil {
		t.Fatalf("valid weekly schedule rejected: %v", err)
	}
	if _, _, err := NormalizeDiscoverySchedule(TriggerWeekly, "24:00", 1); err == nil {
		t.Fatal("invalid hour should be rejected")
	}
	if _, _, err := NormalizeDiscoverySchedule(TriggerWeekly, "08:00", 8); err == nil {
		t.Fatal("invalid weekday should be rejected")
	}
}

func TestIsDiscoveryConfigDueUsesBeijingDailyAndWeeklySlots(t *testing.T) {
	loc := discoveryScheduleLocation()
	dailyNow := time.Date(2026, 9, 22, 8, 59, 0, 0, loc)
	daily := DiscoveryConfig{
		TriggerMode:     TriggerDaily,
		ScheduleTime:    "09:00",
		ScheduleWeekday: 1,
		Enabled:         true,
	}
	if IsDiscoveryConfigDue(daily, dailyNow) {
		t.Fatal("daily config should not run before its time")
	}
	dailyNow = time.Date(2026, 9, 22, 9, 0, 0, 0, loc)
	if !IsDiscoveryConfigDue(daily, dailyNow) {
		t.Fatal("daily config should run at its time")
	}
	lastRun := time.Date(2026, 9, 22, 9, 1, 0, 0, loc)
	daily.LastRunAt = &lastRun
	if IsDiscoveryConfigDue(daily, dailyNow) {
		t.Fatal("daily config should not run twice in the same slot")
	}
	dailyNow = time.Date(2026, 9, 23, 9, 0, 0, 0, loc)
	if !IsDiscoveryConfigDue(daily, dailyNow) {
		t.Fatal("daily config should run again on the next day")
	}

	weekly := DiscoveryConfig{
		TriggerMode:     TriggerWeekly,
		ScheduleTime:    "10:30",
		ScheduleWeekday: 2, // Tuesday
		Enabled:         true,
	}
	if IsDiscoveryConfigDue(weekly, time.Date(2026, 9, 21, 11, 0, 0, 0, loc)) {
		t.Fatal("weekly config should not run on Monday")
	}
	if IsDiscoveryConfigDue(weekly, time.Date(2026, 9, 22, 10, 29, 0, 0, loc)) {
		t.Fatal("weekly config should not run before its time")
	}
	if !IsDiscoveryConfigDue(weekly, time.Date(2026, 9, 22, 10, 30, 0, 0, loc)) {
		t.Fatal("weekly config should run on the configured weekday and time")
	}
}

func TestIsDiscoveryConfigDueIgnoresManualDisabledAndInvalidConfigs(t *testing.T) {
	loc := discoveryScheduleLocation()
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, loc)
	for _, cfg := range []DiscoveryConfig{
		{TriggerMode: TriggerManual, Enabled: true},
		{TriggerMode: TriggerDaily, ScheduleTime: "invalid", Enabled: true},
		{TriggerMode: TriggerDaily, ScheduleTime: "09:00", Enabled: false},
	} {
		if IsDiscoveryConfigDue(cfg, now) {
			t.Fatalf("config should not be due: %+v", cfg)
		}
	}
}
