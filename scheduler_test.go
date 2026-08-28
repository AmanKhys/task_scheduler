package main

import (
	"testing"
	"time"

	"sheduler/internal/db"
)

func TestShouldFireOneTimeInWindow(t *testing.T) {
	due := time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC)
	rule := db.GetActiveReminderRulesRow{
		TriggerType:   "at_due",
		OffsetMinutes: 0,
		Days:          0,
		DueAt:         due,
	}
	last := due.Add(-15 * time.Second)
	now := due.Add(1 * time.Second)
	if !shouldFire(rule, last, now) {
		t.Fatal("expected one-time at_due to fire inside tick window")
	}
	if shouldFire(rule, due.Add(time.Second), now.Add(time.Minute)) {
		t.Fatal("should not fire after the window")
	}
}

func TestShouldFireBeforeDue(t *testing.T) {
	due := time.Date(2026, 8, 28, 16, 15, 0, 0, time.UTC)
	rule := db.GetActiveReminderRulesRow{
		TriggerType:   "before_due",
		OffsetMinutes: 15,
		Days:          0,
		DueAt:         due,
	}
	fire := due.Add(-15 * time.Minute)
	if !shouldFire(rule, fire.Add(-time.Second), fire.Add(time.Second)) {
		t.Fatal("expected before_due to fire")
	}
}

func TestShouldFireRecurringWeekday(t *testing.T) {
	// Friday Aug 28 2026
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	friday := int32(1 << time.Friday)
	rule := db.GetActiveReminderRulesRow{
		TriggerType: "at_due",
		Days:        friday,
		DueAt:       due,
	}
	if !shouldFire(rule, now.Add(-time.Second), now.Add(time.Second)) {
		t.Fatal("expected recurring Friday rule to fire")
	}
	rule.Days = int32(1 << time.Monday)
	if shouldFire(rule, now.Add(-time.Second), now.Add(time.Second)) {
		t.Fatal("Monday-only rule should not fire on Friday")
	}
}
