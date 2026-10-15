package tools

import (
	"strings"
	"time"
)

const (
	DiscoveryScheduleLocationName   = "Asia/Shanghai"
	DefaultDiscoveryScheduleTime    = "09:00"
	DefaultDiscoveryScheduleWeekday = 1
)

// NormalizeDiscoverySchedule keeps old/manual configs usable while enforcing
// the values consumed by the minute-level scheduler.
func NormalizeDiscoverySchedule(trigger TriggerMode, rawTime string, rawWeekday int) (string, int, error) {
	if trigger == "" {
		trigger = TriggerManual
	}
	switch trigger {
	case TriggerManual, TriggerDaily, TriggerWeekly:
	default:
		return "", 0, &DiscoveryError{Class: ErrorValidation, Message: "triggerMode must be manual, daily or weekly"}
	}

	scheduleTime := strings.TrimSpace(rawTime)
	if scheduleTime == "" {
		scheduleTime = DefaultDiscoveryScheduleTime
	}
	parsed, err := time.Parse("15:04", scheduleTime)
	if err != nil || parsed.Format("15:04") != scheduleTime {
		return "", 0, &DiscoveryError{Class: ErrorValidation, Message: "scheduleTime must use HH:MM"}
	}

	scheduleWeekday := rawWeekday
	if scheduleWeekday == 0 {
		scheduleWeekday = DefaultDiscoveryScheduleWeekday
	}
	if scheduleWeekday < 1 || scheduleWeekday > 7 {
		return "", 0, &DiscoveryError{Class: ErrorValidation, Message: "scheduleWeekday must be between 1 and 7"}
	}
	return scheduleTime, scheduleWeekday, nil
}

func discoveryScheduleLocation() *time.Location {
	loc, err := time.LoadLocation(DiscoveryScheduleLocationName)
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

// IsDiscoveryConfigDue returns true once for the current daily or weekly slot.
// LastRunAt prevents the minute-level cron from starting the same slot twice.
func IsDiscoveryConfigDue(cfg DiscoveryConfig, now time.Time) bool {
	if !cfg.Enabled || (cfg.TriggerMode != TriggerDaily && cfg.TriggerMode != TriggerWeekly) {
		return false
	}
	scheduleTime, scheduleWeekday, err := NormalizeDiscoverySchedule(cfg.TriggerMode, cfg.ScheduleTime, cfg.ScheduleWeekday)
	if err != nil {
		return false
	}
	parsed, _ := time.Parse("15:04", scheduleTime)
	loc := discoveryScheduleLocation()
	localNow := now.In(loc)
	slot := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)

	if cfg.TriggerMode == TriggerWeekly {
		weekday := int(localNow.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if weekday != scheduleWeekday {
			return false
		}
	}
	if localNow.Before(slot) {
		return false
	}
	if cfg.LastRunAt == nil {
		return true
	}
	return cfg.LastRunAt.In(loc).Before(slot)
}
