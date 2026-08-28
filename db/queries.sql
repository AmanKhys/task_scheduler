-- name: CreateTask :exec
insert into tasks (
	title,
	description,
	due_at,
	status
) values (
	$1, $2, $3, $4
);

-- name: UpdateTask :exec
UPDATE tasks
SET
    title = COALESCE($2, title),
    description = COALESCE($3, description),
    due_at = COALESCE($4, due_at),
    status = COALESCE($5, status),
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: GetTasks :many
SELECT *
FROM tasks
WHERE
($1::text IS NULL OR status = $1)
    AND ($2::timestamp IS NULL OR due_at >= $2)
    AND ($3::timestamp IS NULL OR due_at <= $3)
ORDER BY due_at ASC;


-- name: CreateReminderRule :exec
insert into reminder_rules (
	task_id,
	name,
	days,
	trigger_type,
	offset_minutes
) values (
	$1, $2, $3, $4, $5
);

-- name: UpdateReminderRule :exec
UPDATE reminder_rules
SET
    name = COALESCE($2, name),
    days = COALESCE($3, days),
    trigger_type = COALESCE($4, trigger_type),
    offset_minutes = COALESCE($5, offset_minutes),
    is_active = COALESCE($6, is_active),
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteReminderRule :exec
DELETE FROM reminder_rules WHERE id = $1;

-- name: GetReminderRules :many
SELECT * FROM reminder_rules WHERE task_id = $1;


-- name: CreateAuditLog :exec
insert into audit_logs (
	event_type,
	rule_id,
	task_id,
	details
) values (
	$1, $2, $3, $4
);

-- name: GetAuditLogs :many
SELECT * FROM audit_logs WHERE task_id = $1;

-- name: GetAuditLog :one
SELECT * FROM audit_logs WHERE id = $1;

