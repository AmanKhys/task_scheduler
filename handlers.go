package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sheduler/internal/db"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	log "github.com/sirupsen/logrus"
)

type Handler struct {
	q *db.Queries
}

type createTaskRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueAt       time.Time `json:"due_at"`
	Status      string    `json:"status"`
}

type updateTaskRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	Status      *string    `json:"status"`
}

type reminderRuleRequest struct {
	Name          string `json:"name"`
	Days          int32  `json:"days"`
	TriggerType   string `json:"trigger_type"`
	OffsetMinutes int32  `json:"offset_minutes"`
}

type updateReminderRequest struct {
	Name          *string `json:"name"`
	Days          *int32  `json:"days"`
	TriggerType   *string `json:"trigger_type"`
	OffsetMinutes *int32  `json:"offset_minutes"`
	IsActive      *bool   `json:"is_active"`
}

type setActiveRequest struct {
	IsActive bool `json:"is_active"`
}

func (h Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Title == "" || req.DueAt.IsZero() {
		writeError(w, http.StatusBadRequest, "title and due_at are required")
		return
	}
	status := req.Status
	if status == "" {
		status = "pending"
	}
	if status != "pending" && status != "completed" {
		writeError(w, http.StatusBadRequest, "status must be pending or completed")
		return
	}

	task, err := h.q.CreateTask(r.Context(), db.CreateTaskParams{
		Title:       req.Title,
		Description: req.Description,
		DueAt:       req.DueAt.UTC(),
		Status:      status,
	})
	if err != nil {
		log.WithError(err).Error("create task")
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	h.audit(r, "task_created", pgtype.UUID{}, task.ID, map[string]any{
		"title": task.Title, "due_at": task.DueAt, "status": task.Status,
	})
	writeJSON(w, http.StatusCreated, task)
}

func (h Handler) GetTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dueFrom, err := parseTimeParam(q.Get("due_from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_from must be RFC3339")
		return
	}
	dueTo, err := parseTimeParam(q.Get("due_to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_to must be RFC3339")
		return
	}
	status := optionalText(q.Get("status"))
	if status.Valid && status.String != "pending" && status.String != "completed" {
		writeError(w, http.StatusBadRequest, "status must be pending or completed")
		return
	}

	tasks, err := h.q.GetTasks(r.Context(), db.GetTasksParams{
		Status:  status,
		DueFrom: dueFrom,
		DueTo:   dueTo,
	})
	if err != nil {
		log.WithError(err).Error("list tasks")
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := h.q.GetTask(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		log.WithError(err).Error("get task")
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Status != nil && *req.Status != "pending" && *req.Status != "completed" {
		writeError(w, http.StatusBadRequest, "status must be pending or completed")
		return
	}

	task, err := h.q.UpdateTask(r.Context(), db.UpdateTaskParams{
		ID:          id,
		Title:       textOrNull(req.Title),
		Description: textOrNull(req.Description),
		DueAt:       tsOrNull(req.DueAt),
		Status:      textOrNull(req.Status),
	})
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		log.WithError(err).Error("update task")
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}
	h.audit(r, "task_updated", pgtype.UUID{}, task.ID, map[string]any{
		"title": task.Title, "due_at": task.DueAt, "status": task.Status,
	})
	writeJSON(w, http.StatusOK, task)
}

func (h Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := h.q.GetTask(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	h.audit(r, "task_deleted", pgtype.UUID{}, task.ID, map[string]any{
		"title": task.Title,
	})
	if err := h.q.DeleteTask(r.Context(), id); err != nil {
		log.WithError(err).Error("delete task")
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	taskID, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.q.GetTask(r.Context(), taskID); isNoRows(err) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}

	var req reminderRuleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TriggerType == "" {
		req.TriggerType = "at_due"
	}
	if err := validateRule(req.TriggerType, req.OffsetMinutes, req.Days); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := h.q.CreateReminderRule(r.Context(), db.CreateReminderRuleParams{
		TaskID:        taskID,
		Name:          req.Name,
		Days:          req.Days,
		TriggerType:   req.TriggerType,
		OffsetMinutes: req.OffsetMinutes,
	})
	if err != nil {
		if isFKViolation(err) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		log.WithError(err).Error("create reminder rule")
		writeError(w, http.StatusInternalServerError, "failed to create reminder rule")
		return
	}
	h.audit(r, "rule_created", rule.ID, rule.TaskID, map[string]any{
		"name": rule.Name, "trigger_type": rule.TriggerType, "days": rule.Days,
	})
	writeJSON(w, http.StatusCreated, rule)
}

func (h Handler) GetReminders(w http.ResponseWriter, r *http.Request) {
	taskID, err := optionalUUID(r.URL.Query().Get("task_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	rules, err := h.q.ListReminderRules(r.Context(), taskID)
	if err != nil {
		log.WithError(err).Error("list reminder rules")
		writeError(w, http.StatusInternalServerError, "failed to list reminder rules")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h Handler) GetReminder(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	rule, err := h.q.GetReminderRule(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "reminder rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get reminder rule")
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h Handler) UpdateReminder(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateReminderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	existing, err := h.q.GetReminderRule(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "reminder rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get reminder rule")
		return
	}

	trigger := existing.TriggerType
	if req.TriggerType != nil {
		trigger = *req.TriggerType
	}
	offset := existing.OffsetMinutes
	if req.OffsetMinutes != nil {
		offset = *req.OffsetMinutes
	}
	days := existing.Days
	if req.Days != nil {
		days = *req.Days
	}
	if err := validateRule(trigger, offset, days); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := h.q.UpdateReminderRule(r.Context(), db.UpdateReminderRuleParams{
		ID:            id,
		Name:          textOrNull(req.Name),
		Days:          int4OrNull(req.Days),
		TriggerType:   textOrNull(req.TriggerType),
		OffsetMinutes: int4OrNull(req.OffsetMinutes),
		IsActive:      boolOrNull(req.IsActive),
	})
	if err != nil {
		log.WithError(err).Error("update reminder rule")
		writeError(w, http.StatusInternalServerError, "failed to update reminder rule")
		return
	}

	event := "rule_updated"
	if req.IsActive != nil && *req.IsActive != existing.IsActive {
		if *req.IsActive {
			event = "rule_activated"
		} else {
			event = "rule_deactivated"
		}
	}
	h.audit(r, event, rule.ID, rule.TaskID, map[string]any{
		"name": rule.Name, "is_active": rule.IsActive,
	})
	writeJSON(w, http.StatusOK, rule)
}

func (h Handler) SetReminderStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req setActiveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rule, err := h.q.SetReminderRuleActive(r.Context(), db.SetReminderRuleActiveParams{
		ID:       id,
		IsActive: req.IsActive,
	})
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "reminder rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update reminder rule")
		return
	}
	event := "rule_deactivated"
	if req.IsActive {
		event = "rule_activated"
	}
	h.audit(r, event, rule.ID, rule.TaskID, map[string]any{"is_active": rule.IsActive})
	writeJSON(w, http.StatusOK, rule)
}

func (h Handler) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	rule, err := h.q.GetReminderRule(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "reminder rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get reminder rule")
		return
	}
	h.audit(r, "rule_deleted", rule.ID, rule.TaskID, map[string]any{"name": rule.Name})
	if err := h.q.DeleteReminderRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete reminder rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) GetAllAuditLogs(w http.ResponseWriter, r *http.Request) {
	taskID, err := optionalUUID(r.URL.Query().Get("task_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	logs, err := h.q.ListAuditLogs(r.Context(), taskID)
	if err != nil {
		log.WithError(err).Error("list audit logs")
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (h Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathUUID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	entry, err := h.q.GetAuditLog(r.Context(), id)
	if isNoRows(err) {
		writeError(w, http.StatusNotFound, "audit log not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get audit log")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h Handler) audit(r *http.Request, event string, ruleID, taskID pgtype.UUID, details map[string]any) {
	var raw []byte
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	if err := h.q.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
		EventType: event,
		RuleID:    ruleID,
		TaskID:    taskID,
		Details:   raw,
	}); err != nil {
		log.WithError(err).WithField("event", event).Error("write audit log")
	}
}

func optionalText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func validateRule(triggerType string, offsetMinutes, days int32) error {
	switch triggerType {
	case "before_due", "at_due", "after_due":
	default:
		return errors.New("trigger_type must be before_due, at_due, or after_due")
	}
	if triggerType == "at_due" && offsetMinutes != 0 {
		return errors.New("at_due requires offset_minutes to be 0")
	}
	if offsetMinutes < 0 {
		return errors.New("offset_minutes must be >= 0")
	}
	if days < 0 || days > 127 {
		return errors.New("days must be between 0 and 127")
	}
	return nil
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
