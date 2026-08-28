CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    due_at TIMESTAMP NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'completed')),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE reminder_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,

    name TEXT NOT NULL,

    -- 0 = one-time
    -- 1 = Sunday
    -- 2 = Monday
    -- 4 = Tuesday
    -- 8 = Wednesday
    -- 16 = Thursday
    -- 32 = Friday
    -- 64 = Saturday
    -- combinations are added together
    days INT NOT NULL DEFAULT 0
        CHECK (days BETWEEN 0 AND 127),

    trigger_type TEXT NOT NULL DEFAULT 'at_due'
        CHECK (trigger_type IN (
            'before_due',
            'at_due',
            'after_due'
        )),

    offset_minutes INT NOT NULL DEFAULT 0
        CHECK (offset_minutes >= 0),

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- at_due must not have an offset
    CHECK (
        trigger_type <> 'at_due'
        OR offset_minutes = 0
    )
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    event_type TEXT NOT NULL
        CHECK (event_type IN (
            'task_created',
            'task_updated',
            'task_deleted',
            'rule_created',
            'rule_updated',
            'rule_deleted',
            'rule_activated',
            'rule_deactivated',
            'reminder_triggered'
        )),

    rule_id UUID REFERENCES reminder_rules(id) ON DELETE SET NULL,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,

    details JSONB,

    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reminder_rules_task_id
    ON reminder_rules(task_id);

CREATE INDEX idx_reminder_rules_active
    ON reminder_rules(is_active);

CREATE INDEX idx_audit_logs_rule_id
    ON audit_logs(rule_id);

CREATE INDEX idx_audit_logs_task_id
    ON audit_logs(task_id);

CREATE INDEX idx_audit_logs_created_at
    ON audit_logs(created_at);
