# Task Scheduler

A lightweight, database-backed **task and reminder scheduling system** built with **Go and PostgreSQL**, with a browser-based interface served directly by the Go application.

The project explores a practical scheduling problem:

> **How can a backend reliably detect when a reminder is due without depending on an in-memory timer for every scheduled event?**

The solution keeps task and reminder state in PostgreSQL and uses a periodic scheduler to evaluate active reminder rules against a time window.

**Live application:** https://tasks.amankhys.space

---

## Overview

The application allows users to create tasks and define reminder rules associated with those tasks.

A reminder rule can be configured to trigger:

* **Before** a task's due time
* **At** the task's due time
* **After** the task's due time

Reminder rules can also be either:

* **One-time**
* **Recurring on selected days of the week**

The application records important actions and scheduler events in an audit log, making system activity inspectable after the fact.

The backend exposes a REST API for tasks, reminder rules, and audit logs, while the same Go server also serves the web interface.

---

# Design Objective

The primary objective of this project is to implement the **core mechanics of persistent time-based scheduling** without introducing unnecessary distributed infrastructure.

The design intentionally focuses on:

* Persistent scheduling state
* Deterministic scheduling logic
* Recurring schedules
* Database-enforced constraints
* Auditability
* Simple concurrency
* Testable scheduling behavior
* A small operational footprint

The system is therefore designed as a **single Go application with a PostgreSQL database**, rather than as a distributed job queue or worker cluster.

This keeps the architecture understandable while still exposing the important engineering problems that arise in scheduling systems.

---

# Architecture

```text
                          Browser
                             │
                             │ HTTP
                             ▼
                 ┌────────────────────────┐
                 │       Go Server        │
                 │                        │
                 │   REST API             │
                 │   Web UI               │
                 │   Health Endpoint      │
                 │   Scheduler            │
                 └───────────┬────────────┘
                             │
                             │ SQL
                             ▼
                  ┌──────────────────────┐
                  │      PostgreSQL      │
                  │                      │
                  │  tasks               │
                  │  reminder_rules      │
                  │  audit_logs          │
                  └──────────────────────┘
```

The HTTP API and scheduler run inside the same Go process.

At application startup, the server:

1. Creates a PostgreSQL connection pool.
2. Verifies database connectivity.
3. Applies the database schema.
4. Seeds sample data when the database is empty.
5. Creates the HTTP router.
6. Starts the scheduler in a goroutine.
7. Starts the HTTP server.

The application also uses OS signal handling to terminate the shared context and stop the scheduler when the process is shutting down.

---

# Core Scheduling Principle

The scheduler does **not** create a separate in-memory timer for every reminder.

Instead, reminder rules are persisted in PostgreSQL and periodically evaluated.

The scheduler keeps track of two timestamps:

```text
last_tick
current_tick
```

A reminder is eligible to fire when its calculated execution time falls within the scheduler's evaluation window:

```text
last_tick < scheduled_time <= current_tick
```

This is important because a polling scheduler cannot rely on waking up at precisely the scheduled timestamp.

For example:

```text
Reminder scheduled for:
10:00:00

Scheduler checks:
09:59:55
10:00:10
```

The reminder can still be detected because `10:00:00` falls inside the interval being evaluated.

This makes the scheduler tolerant of normal polling delays while keeping the scheduling logic deterministic.

---

# Scheduler Loop

The scheduler runs every **15 seconds**.

The execution flow is approximately:

```text
                  Start
                    │
                    ▼
        Initialize previous tick
                    │
                    ▼
      Initial scheduler evaluation
                    │
                    ▼
             Wait 15 seconds
                    │
                    ▼
             Current tick
                    │
                    ▼
      Load active reminder rules
                    │
                    ▼
       Calculate scheduled time
                    │
                    ▼
    Is scheduled time in window?
                /       \
              No         Yes
              │           │
              │           ▼
              │     Check audit history
              │           │
              │      Already triggered?
              │         /      \
              │       Yes       No
              │       │          │
              │       │          ▼
              │       │    Record trigger
              │       │          │
              └───────┴──────────┘
                         │
                         ▼
                   Next tick
```

The scheduler only evaluates active reminder rules associated with tasks that are still eligible for reminder processing.

---

# 15-Second Polling Interval

The scheduler uses a **15-second ticker**.

This interval is a deliberate trade-off.

A shorter interval would reduce the maximum scheduling delay but increase polling frequency and database activity.

A longer interval would reduce polling overhead but increase the potential delay between a reminder becoming due and being detected.

The current design therefore prioritizes a simple, predictable polling model rather than exact real-time execution.

---

# Two-Minute Startup Window

When the scheduler starts, it initializes its previous tick to approximately **two minutes before the current time**:

```text
last_tick = now - 2 minutes
```

The first scheduler evaluation therefore covers only approximately:

```text
(now - 2 minutes) → now
```

### Why?

The scheduler needs some look-back period at startup.

Consider:

```text
Reminder due:       10:00:00
Application starts: 10:00:05
```

Without a startup look-back, the scheduler could miss the reminder because the reminder's scheduled time occurred just before the scheduler began.

With the two-minute window, the scheduler can detect recently due reminders around the startup boundary.

### The limitation

The two-minute window is intentionally bounded.

For example:

```text
Reminder due:       10:00
Application starts: 10:05
```

The initial evaluation covers approximately:

```text
10:03 → 10:05
```

The `10:00` reminder is outside that window and therefore will not be recovered by the startup check.

This means the current implementation does **not** attempt to replay an unlimited amount of historical scheduling state after downtime.

### Why this design?

The two-minute value is a practical recovery buffer rather than a full missed-job recovery mechanism.

Fully recovering arbitrary missed reminders would require additional persistent execution state and an explicit policy for overdue work.

For example:

```text
Application starts
        │
        ▼
Find overdue reminders
        │
        ├── execute
        ├── skip
        ├── execute latest occurrence
        └── mark expired
```

That is a separate design problem from normal scheduling and is intentionally outside the current scope.

---

# Reminder Model

A reminder rule contains:

* Task association
* Rule name
* Trigger type
* Offset in minutes
* Recurring-day configuration
* Active/inactive state

The supported trigger types are:

```text
before_due
at_due
after_due
```

The offset is expressed in minutes.

Examples:

```text
30 minutes before
10 minutes before
At the due time
15 minutes after
```

For `at_due`, the offset must be zero. The database enforces this constraint directly.

---

# Recurring Reminders

Recurring reminders use a seven-bit representation of the days of the week.

```text
Sunday     = 1
Monday     = 2
Tuesday    = 4
Wednesday  = 8
Thursday   = 16
Friday     = 32
Saturday   = 64
```

Multiple days are represented by combining the corresponding bits.

For example:

```text
Monday + Wednesday + Friday

2 + 8 + 32 = 42
```

The scheduler checks the current weekday using a bitwise operation.

A value of:

```text
days = 0
```

represents a one-time reminder.

The database constrains the valid mask to the range `0–127`.

### Why a bitmask?

The domain has exactly seven possible weekdays.

A bitmask provides:

* Compact storage
* Simple membership checks
* No additional table required
* Straightforward combination of multiple weekdays

It is therefore a reasonable representation for the current scheduling model.

---

# Database Design

The core data model is:

```text
tasks
  │
  └── reminder_rules
          │
          └── audit_logs
```

## Tasks

A task represents the item being scheduled.

It contains information such as:

* Title
* Description
* Due time
* Status
* Creation/update timestamps

Task status is currently constrained to:

```text
pending
completed
```

## Reminder Rules

A reminder rule defines when a task should generate a reminder.

Each rule belongs to a task and can be:

* Created
* Updated
* Activated
* Deactivated
* Deleted

Deleting a task also removes its associated reminder rules through the database foreign-key relationship.

## Audit Logs

The audit log records significant application and scheduler events, including:

```text
task_created
task_updated
task_deleted

rule_created
rule_updated
rule_deleted
rule_activated
rule_deactivated

reminder_triggered
```

Audit entries can also contain structured JSON details about the event.

---

# Auditability

The audit log provides a persistent record of activity inside the system.

When a reminder is triggered, the scheduler records details including:

* Rule name
* Task title
* Task due time
* Trigger type
* Offset
* Recurring-day configuration
* Calculated scheduled time

This provides visibility into scheduler behavior and gives the application a persistent execution history rather than relying only on application logs.

---

# Why PostgreSQL?

PostgreSQL is used as the persistent source of truth for the scheduler.

This allows the system to retain:

* Tasks
* Reminder rules
* Active/inactive state
* Audit history

across application restarts.

The database is also responsible for enforcing several domain rules through:

* Foreign keys
* Check constraints
* Indexes
* JSONB audit details

This prevents invalid state from being dependent entirely on application-level validation.

---

# Why `sqlc`?

The project uses [`sqlc`](https://sqlc.dev/) to generate the Go database access layer from SQL queries.

The approach is:

```text
SQL queries
     │
     ▼
    sqlc
     │
     ▼
Generated Go code
     │
     ▼
Application
```

The goal is to retain explicit SQL while getting strongly typed Go database methods and models.

This avoids hiding database behavior behind a large ORM abstraction while reducing repetitive database-access code.

---

# Why Polling Instead of Individual Timers?

A different implementation could create an in-memory timer for every reminder:

```text
Reminder A → timer
Reminder B → timer
Reminder C → timer
Reminder D → timer
```

That approach introduces additional complexity around:

* Application restarts
* Reconstructing timers
* Updating reminders
* Cancelling reminders
* Recurring schedules
* Missed reminders
* Large numbers of timers

The current design instead treats PostgreSQL as the source of truth:

```text
                 PostgreSQL
                     │
                     │ persisted state
                     ▼
                 Scheduler
                     │
                     │ periodic evaluation
                     ▼
               Due reminder
                     │
                     ▼
               Audit record
```

For the scale and purpose of this project, this is a deliberate choice to favor **simplicity, persistence, and clear scheduling semantics** over a more complex timer-management system.

---

# API

The Go server exposes a REST API for tasks, reminder rules, and audit logs.

## Tasks

```http
POST   /tasks
GET    /tasks
GET    /tasks/{id}
PUT    /tasks/{id}
DELETE /tasks/{id}
```

Task listing also supports query parameters for filtering by status and due-time ranges.

## Reminder Rules

```http
POST   /tasks/{id}/reminder-rules
GET    /reminder-rules
GET    /reminder-rules/{id}
PUT    /reminder-rules/{id}
PATCH  /reminder-rules/{id}/status
DELETE /reminder-rules/{id}
```

## Audit Logs

```http
GET /audit-logs
GET /audit-logs/{id}
```

Audit logs can also be filtered by task.

## Health Check

```http
GET /health
```

The health endpoint currently returns a simple successful status response.

---

# HTTP Design

The API uses standard HTTP methods and status codes and performs validation at the handler layer before interacting with the database.

Examples include:

* `400 Bad Request` for malformed input
* `404 Not Found` when a requested resource does not exist
* `201 Created` for newly created resources
* `204 No Content` for successful deletions

The API also enables CORS and handles preflight `OPTIONS` requests.

---

# Project Structure

```text
.
├── db/
│   ├── schema.sql
│   └── queries.sql
│
├── docs/
│
├── internal/
│   └── db/
│       ├── db.go
│       ├── models.go
│       └── queries.sql.go
│
├── web/
│   ├── index.html
│   ├── app.js
│   ├── styles.css
│   └── static/
│
├── handlers.go
├── helpers.go
├── http.go
├── main.go
├── scheduler.go
├── scheduler_test.go
├── seed.go
├── sqlc.yaml
├── docker-compose.yml
├── go.mod
└── go.sum
```

The repository keeps the scheduler, HTTP handling, SQL definitions, generated database layer, tests, and web assets separate while remaining within a single application.

---

# Startup and Configuration

The application reads its PostgreSQL connection string from:

```text
DATABASE_URL
```

If it is not provided, the application uses:

```text
postgres://postgres:postgres@localhost:5433/task_scheduler?sslmode=disable
```

The HTTP server defaults to:

```text
:8081
```

and can be changed using:

```text
PORT
```

The scheduler interval is currently configured in the application as:

```text
15 seconds
```

---

# Running Locally

## Requirements

* Go
* Docker
* Docker Compose
* PostgreSQL, if Docker is not used

## Start PostgreSQL

```bash
docker compose up -d
```

The provided Compose configuration uses PostgreSQL 16 and exposes the database on:

```text
localhost:5433
```

A PostgreSQL health check is also configured.

## Start the application

```bash
go run .
```

The default application URL is:

```text
http://localhost:8081
```

At startup, the application connects to PostgreSQL, applies the schema, seeds an empty database, starts the scheduler, and begins serving HTTP requests.

## Run tests

```bash
go test ./...
```

---

# Testing

The scheduler has focused tests around the core scheduling decision logic.

The current test coverage includes scenarios such as:

* One-time reminders
* Before-due reminders
* Recurring weekday reminders
* Non-matching weekdays
* Time-window evaluation

The important behavior being tested is that a reminder can be detected when its scheduled timestamp falls **between two scheduler ticks**, rather than only when the scheduler executes at exactly that timestamp.

---

# Strengths

## Persistent state

Tasks and reminder rules are stored in PostgreSQL rather than being dependent solely on process memory.

## Simple scheduling model

The scheduler is small and deterministic:

```text
previous tick
      ↓
current tick
      ↓
evaluate reminders
```

This makes the core behavior relatively easy to understand and test.

## Explicit scheduling semantics

The use of a time window means the scheduler does not depend on exact timer alignment.

## Recurring rules

The system supports recurring weekday schedules while keeping their representation compact.

## Database constraints

The database itself enforces important invariants, reducing invalid state.

## Auditability

Scheduler and application events are persisted in an audit log.

## Clear separation of responsibilities

The HTTP layer handles requests and validation while the scheduler independently handles time-based evaluation.

## Small operational footprint

The project can run with a Go application and PostgreSQL without requiring a message broker, cache, or distributed coordination service.

---

# Limitations and Trade-offs

The current architecture intentionally favors simplicity and is **not a distributed scheduling system**.

## Single scheduler

The application currently starts one scheduler within the Go process.

The trigger flow performs:

```text
check audit history
        ↓
record reminder trigger
```

as separate database operations.

If multiple scheduler instances were introduced, two instances could potentially observe the same reminder before either records the trigger.

A horizontally scaled implementation would therefore need an atomic claiming/idempotency mechanism.

Possible approaches include:

* PostgreSQL row-level locking
* `SELECT ... FOR UPDATE SKIP LOCKED`
* Atomic state transitions
* Unique execution keys
* Leases
* A dedicated job queue

---

## Two-minute startup recovery limit

The initial scheduler evaluation looks back approximately **two minutes**.

Therefore, reminders that became due more than two minutes before application startup are not automatically recovered.

This is a deliberate boundary of the current implementation.

The system does not attempt to replay arbitrary historical reminders.

---

## Long downtime

If the application remains offline for an extended period, some reminders may be missed.

A more durable implementation would persist explicit execution state such as:

```text
next_run_at
execution_status
```

and define what should happen when overdue work is discovered after recovery.

---

## Polling scalability

The scheduler periodically loads active reminder rules and evaluates them.

This is straightforward for a small system, but at sufficiently large scale it would be more efficient to query only reminders that are approaching or have passed their `next_run_at`.

---

## Exactly-once execution

The system does not claim a distributed exactly-once execution guarantee.

In a production job-processing system, execution semantics would need to be explicitly defined and designed around the side effects being performed.

Common models include:

```text
at-most-once
at-least-once
idempotent / effectively-once
```

The appropriate model depends on the actual work performed by the scheduler.

---

## Time-zone handling

Time-based scheduling becomes significantly more complex when schedules are defined in user-local time.

A production implementation would need explicit timezone semantics and careful handling of:

* UTC
* Local time
* Recurring schedules
* Daylight-saving transitions

The current implementation keeps the scheduling model simpler.

---

# Why These Trade-offs Are Intentional

The project is designed around an important engineering principle:

> **Do not introduce distributed complexity before the requirements justify it.**

A scheduler can eventually become a large system involving:

```text
Scheduler
    ↓
Job queue
    ↓
Workers
    ↓
Retries
    ↓
Idempotency
    ↓
Distributed coordination
    ↓
Dead-letter handling
```

That architecture may be appropriate at large scale.

It is not necessary for the current project.

The current implementation therefore focuses on getting the fundamental model right first:

```text
Persistent state
        +
Scheduling rules
        +
Time-window evaluation
        +
Execution history
```

---

# Potential Evolution

A natural progression from the current design would be:

```text
Current
────────────────────────────

Go API
  +
Scheduler
  +
PostgreSQL


More robust scheduler
────────────────────────────

Go API
  +
Scheduler
  +
PostgreSQL
  +
Atomic job claiming


Worker-based architecture
────────────────────────────

Go API
  +
Scheduler
  ↓
Durable Job Queue
  ↓
Workers
  +
PostgreSQL


Distributed architecture
────────────────────────────

API cluster
     +
Scheduler cluster
     +
Durable queue
     +
Worker cluster
     +
Persistent execution state
     +
Observability
```

Potential future improvements include:

* Persisting `next_run_at`
* Atomic job claiming
* Retry policies
* Idempotency keys
* Missed-job recovery
* Worker processes
* Dead-letter handling
* Time-zone-aware scheduling
* Distributed scheduler coordination
* Metrics and tracing

---

# Engineering Decisions

| Decision                      | Reason                                                 |
| ----------------------------- | ------------------------------------------------------ |
| PostgreSQL as source of truth | Persist scheduling state                               |
| 15-second polling             | Balance responsiveness and polling overhead            |
| Time-window evaluation        | Avoid dependence on exact scheduler timing             |
| 2-minute startup look-back    | Provide a small startup recovery buffer                |
| Weekday bitmask               | Compact representation of seven fixed weekdays         |
| `sqlc`                        | Explicit SQL with typed Go access                      |
| Audit log                     | Persist scheduler and application events               |
| Single Go process             | Keep the architecture appropriate to the current scope |
| Docker Compose                | Reproducible PostgreSQL environment                    |
| Database constraints          | Protect important invariants at the data layer         |

---

# Project Philosophy

The project follows a few core principles:

### Keep the source of truth persistent

Scheduling state belongs in the database rather than only in application memory.

### Prefer simple mechanisms that are easy to reason about

Polling and time-window evaluation are straightforward to understand and test.

### Make trade-offs explicit

The system intentionally does not claim to solve distributed scheduling, long-outage recovery, or exactly-once execution.

### Let complexity follow requirements

Queues, workers, distributed locking, and other infrastructure should be introduced when the workload and reliability requirements make them necessary.

### Build a foundation that can evolve

The current architecture provides a clear path toward atomic job claiming, worker-based execution, retries, and distributed scheduling without changing the fundamental concept of persistent scheduling state.

---

# Conclusion

Task Scheduler is a compact implementation of a **persistent, database-backed reminder scheduling system**.

Its purpose is to demonstrate how a backend can combine:

```text
Tasks
  +
Reminder rules
  +
Relative scheduling
  +
Recurring schedules
  +
Periodic evaluation
  +
Audit history
  +
REST APIs
  +
Persistent storage
```

The current system intentionally favors **clarity and simplicity within a single-scheduler environment**.

Its limitations around multiple scheduler instances, prolonged downtime, polling scalability, timezone-aware scheduling, and exactly-once execution are explicit boundaries of the current design.

That makes the project a practical foundation for exploring how a simple scheduler can evolve into a more robust distributed job-processing system as requirements grow.
