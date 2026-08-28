-- name: CreateTask :one
INSERT INTO tasks (
    title,
    description,
    due_at,
    status
) VALUES (
    sqlc.arg('title'),
    sqlc.arg('description'),
    sqlc.arg('due_at'),
    sqlc.arg('status')
)
RETURNING *;

-- name: UpdateTask :one
UPDATE tasks
SET
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    due_at = COALESCE(sqlc.narg('due_at'), due_at),
    status = COALESCE(sqlc.narg('status'), status),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = sqlc.arg('id');

-- name: GetTask :one
SELECT * FROM tasks WHERE id = sqlc.arg('id');

-- name: CountTasks :one
SELECT COUNT(*) FROM tasks;

-- name: GetTasks :many
SELECT *
FROM tasks
WHERE
    (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
    AND (sqlc.narg('due_from')::timestamp IS NULL OR due_at >= sqlc.narg('due_from'))
    AND (sqlc.narg('due_to')::timestamp IS NULL OR due_at <= sqlc.narg('due_to'))
ORDER BY due_at ASC;

-- name: CreateReminderRule :one
INSERT INTO reminder_rules (
    task_id,
    name,
    days,
    trigger_type,
    offset_minutes
) VALUES (
    sqlc.arg('task_id'),
    sqlc.arg('name'),
    sqlc.arg('days'),
    sqlc.arg('trigger_type'),
    sqlc.arg('offset_minutes')
)
RETURNING *;

-- name: UpdateReminderRule :one
UPDATE reminder_rules
SET
    name = COALESCE(sqlc.narg('name'), name),
    days = COALESCE(sqlc.narg('days'), days),
    trigger_type = COALESCE(sqlc.narg('trigger_type'), trigger_type),
    offset_minutes = COALESCE(sqlc.narg('offset_minutes'), offset_minutes),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetReminderRuleActive :one
UPDATE reminder_rules
SET
    is_active = sqlc.arg('is_active'),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteReminderRule :exec
DELETE FROM reminder_rules WHERE id = sqlc.arg('id');

-- name: GetReminderRule :one
SELECT * FROM reminder_rules WHERE id = sqlc.arg('id');

-- name: ListReminderRules :many
SELECT *
FROM reminder_rules
WHERE (sqlc.narg('task_id')::uuid IS NULL OR task_id = sqlc.narg('task_id'))
ORDER BY created_at ASC;

-- name: GetActiveReminderRules :many
SELECT
    r.id,
    r.task_id,
    r.name,
    r.days,
    r.trigger_type,
    r.offset_minutes,
    r.is_active,
    r.created_at,
    r.updated_at,
    t.title AS task_title,
    t.description AS task_description,
    t.due_at,
    t.status AS task_status
FROM reminder_rules r
INNER JOIN tasks t ON t.id = r.task_id
WHERE r.is_active = TRUE
  AND t.status = 'pending';

-- name: CreateAuditLog :exec
INSERT INTO audit_logs (
    event_type,
    rule_id,
    task_id,
    details
) VALUES (
    sqlc.arg('event_type'),
    sqlc.narg('rule_id'),
    sqlc.narg('task_id'),
    sqlc.narg('details')
);

-- name: ListAuditLogs :many
SELECT *
FROM audit_logs
WHERE (sqlc.narg('task_id')::uuid IS NULL OR task_id = sqlc.narg('task_id'))
ORDER BY created_at DESC;

-- name: GetAuditLog :one
SELECT * FROM audit_logs WHERE id = sqlc.arg('id');

-- name: HasReminderTriggeredSince :one
SELECT EXISTS (
    SELECT 1 FROM audit_logs
    WHERE rule_id = sqlc.arg('rule_id')
      AND event_type = 'reminder_triggered'
      AND created_at >= sqlc.arg('since')
);
