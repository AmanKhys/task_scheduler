package main

import (
	"context"
	"fmt"
	"time"

	"sheduler/internal/db"

	log "github.com/sirupsen/logrus"
)

func seedIfEmpty(ctx context.Context, q *db.Queries) error {
	n, err := q.CountTasks(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	now := time.Now().UTC()
	allDays := int32(127)

	type seedTask struct {
		title       string
		description string
		dueAt       time.Time
		status      string
		rules       []db.CreateReminderRuleParams
	}

	tasks := []seedTask{
		{
			title:       "Submit weekly report",
			description: "One-time reminder at due time",
			dueAt:       now.Add(45 * time.Second),
			status:      "pending",
			rules: []db.CreateReminderRuleParams{{
				Name:          "Due now",
				Days:          0,
				TriggerType:   "at_due",
				OffsetMinutes: 0,
			}},
		},
		{
			title:       "Call the dentist",
			description: "Remind 15 minutes before due",
			dueAt:       now.Add(15 * time.Minute),
			status:      "pending",
			rules: []db.CreateReminderRuleParams{{
				Name:          "15m before",
				Days:          0,
				TriggerType:   "before_due",
				OffsetMinutes: 15,
			}},
		},
		{
			title:       "Pay rent",
			description: "Remind 5 minutes after due",
			dueAt:       now.Add(-5 * time.Minute),
			status:      "pending",
			rules: []db.CreateReminderRuleParams{{
				Name:          "5m after due",
				Days:          0,
				TriggerType:   "after_due",
				OffsetMinutes: 5,
			}},
		},
		{
			title:       "Daily standup notes",
			description: "Recurring every day at due time-of-day",
			dueAt:       now.Add(30 * time.Second),
			status:      "pending",
			rules: []db.CreateReminderRuleParams{{
				Name:          "Every day",
				Days:          allDays,
				TriggerType:   "at_due",
				OffsetMinutes: 0,
			}},
		},
		{
			title:       "Archive last sprint",
			description: "Completed task should never fire",
			dueAt:       now.Add(time.Hour),
			status:      "completed",
			rules: []db.CreateReminderRuleParams{{
				Name:          "Should not fire",
				Days:          0,
				TriggerType:   "at_due",
				OffsetMinutes: 0,
			}},
		},
	}

	for _, st := range tasks {
		task, err := q.CreateTask(ctx, db.CreateTaskParams{
			Title:       st.title,
			Description: st.description,
			DueAt:       st.dueAt,
			Status:      st.status,
		})
		if err != nil {
			return fmt.Errorf("seed task %q: %w", st.title, err)
		}
		if err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
			EventType: "task_created",
			TaskID:    task.ID,
			Details:   []byte(`{"seeded":true}`),
		}); err != nil {
			return err
		}
		for _, ruleParams := range st.rules {
			ruleParams.TaskID = task.ID
			rule, err := q.CreateReminderRule(ctx, ruleParams)
			if err != nil {
				return fmt.Errorf("seed rule %q: %w", ruleParams.Name, err)
			}
			if err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
				EventType: "rule_created",
				RuleID:    rule.ID,
				TaskID:    task.ID,
				Details:   []byte(`{"seeded":true}`),
			}); err != nil {
				return err
			}
		}
	}

	log.Info("seeded 5 sample tasks and reminder rules")
	return nil
}
