package main

import (
	"context"
	"encoding/json"
	"time"

	"sheduler/internal/db"

	log "github.com/sirupsen/logrus"
)

type Scheduler struct {
	q        *db.Queries
	interval time.Duration
}

func (s *Scheduler) Run(ctx context.Context) {
	lastTick := time.Now().UTC().Add(-2 * time.Minute)
	s.tick(ctx, lastTick, time.Now().UTC())
	lastTick = time.Now().UTC()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			now := t.UTC()
			s.tick(ctx, lastTick, now)
			lastTick = now
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, lastTick, now time.Time) {
	rules, err := s.q.GetActiveReminderRules(ctx)
	if err != nil {
		log.WithError(err).Error("scheduler: load active reminder rules")
		return
	}
	for _, rule := range rules {
		if !shouldFire(rule, lastTick, now) {
			continue
		}
		since := time.Unix(0, 0).UTC()
		if rule.Days != 0 {
			y, m, d := now.Date()
			since = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		}
		already, err := s.q.HasReminderTriggeredSince(ctx, db.HasReminderTriggeredSinceParams{
			RuleID: rule.ID,
			Since:  since,
		})
		if err != nil {
			log.WithError(err).WithField("rule_id", rule.ID).Error("scheduler: check audit")
			continue
		}
		if already {
			continue
		}
		fire := scheduledAt(rule, now)
		log.Infof(
			"REMINDER [%s] task=%q due=%s trigger=%s offset=%dm scheduled=%s",
			rule.Name,
			rule.TaskTitle,
			rule.DueAt.Format(time.RFC3339),
			rule.TriggerType,
			rule.OffsetMinutes,
			fire.Format(time.RFC3339),
		)
		details, _ := json.Marshal(map[string]any{
			"rule_name":      rule.Name,
			"task_title":     rule.TaskTitle,
			"due_at":         rule.DueAt,
			"trigger_type":   rule.TriggerType,
			"offset_minutes": rule.OffsetMinutes,
			"days":           rule.Days,
			"scheduled_at":   fire,
		})
		if err := s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
			EventType: "reminder_triggered",
			RuleID:    rule.ID,
			TaskID:    rule.TaskID,
			Details:   details,
		}); err != nil {
			log.WithError(err).Error("scheduler: write reminder_triggered")
		}
	}
}

func fireInstant(due time.Time, triggerType string, offsetMinutes int32) time.Time {
	offset := time.Duration(offsetMinutes) * time.Minute
	switch triggerType {
	case "before_due":
		return due.Add(-offset)
	case "after_due":
		return due.Add(offset)
	default:
		return due
	}
}

func weekdayBit(t time.Time) int32 {
	return 1 << int32(t.Weekday())
}

func inWindow(fire, lastTick, now time.Time) bool {
	return lastTick.Before(fire) && !now.Before(fire)
}

func scheduledAt(rule db.GetActiveReminderRulesRow, now time.Time) time.Time {
	instant := fireInstant(rule.DueAt, rule.TriggerType, rule.OffsetMinutes)
	if rule.Days == 0 {
		return instant
	}
	return time.Date(now.Year(), now.Month(), now.Day(), instant.Hour(), instant.Minute(), instant.Second(), instant.Nanosecond(), now.Location())
}

func shouldFire(rule db.GetActiveReminderRulesRow, lastTick, now time.Time) bool {
	if rule.Days != 0 && weekdayBit(now)&rule.Days == 0 {
		return false
	}
	return inWindow(scheduledAt(rule, now), lastTick, now)
}
