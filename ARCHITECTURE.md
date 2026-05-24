# GoFlow — Architecture

## Overview

GoFlow is a distributed task queue and workflow engine. It separates task submission (HTTP API) from task processing (workers) using Redis as the message broker and PostgreSQL as the source of truth. A workflow engine orchestrates multi-step processes with automatic compensation on failure. All components emit structured logs and Prometheus metrics.

---

## Component Map

```
┌──────────────────────────────────────────────────────────────────┐
│                          GoFlow Process                          │
│                                                                  │
│  ┌──────────────┐   ┌─────────────────┐   ┌─────────────────┐  │
│  │  Fiber HTTP  │   │  Asynq Worker   │   │ Workflow Engine  │  │
│  │  + Middleware│   │  (goroutine)    │   │  (goroutine)    │  │
│  │              │   │                 │   │                  │  │
│  │  /health     │   │  email:send     │   │  execute()      │  │
│  │  /metrics    │   │  report:gen     │   │  failWorkflow() │  │
│  │  /api/v1/    │   │  task:fail      │   │  StepHandlers   │  │
│  │    tasks     │   └────────┬────────┘   └────────┬────────┘  │
│  │  /api/v1/    │            │                      │            │
│  │    workflows │            └──────────┬───────────┘            │
│  └──────┬───────┘                       │                        │
│         │                               │                        │
│  ┌──────▼───────────────────────────────▼──────┐                │
│  │                 Service Layer                │                │
│  │       TaskService        WorkflowService     │                │
│  └──────┬──────────────────────────┬────────────┘                │
│         │                          │                              │
│  ┌──────▼───────┐   ┌─────────────▼──────┐                      │
│  │  Task Repo   │   │   Workflow Repo     │                      │
│  └──────┬───────┘   └─────────────┬──────┘                      │
│         │           ┌─────────────▼──────┐                      │
│         │           │   Queue Client      │                      │
│         │           │  asynq.Client       │                      │
│         │           │  asynq.Inspector    │                      │
│         │           └─────────────┬──────┘                      │
│                                   │                              │
│  ┌────────────────────────────────────────┐                     │
│  │           Observability                │                     │
│  │   pkg/logger (Zap)                     │                     │
│  │   pkg/metrics (Prometheus)             │                     │
│  │   GET /metrics endpoint                │                     │
│  └────────────────────────────────────────┘                     │
└──────────────────────────┬────────────────────────────────────── ┘
                           │
          ┌────────────────┼─────────────────┐
    ┌─────▼──────┐  ┌──────▼──────┐  ┌───────▼──────┐
    │ PostgreSQL │  │    Redis    │  │  Prometheus  │
    │            │  │  (Asynq)   │  │  + Grafana   │
    └────────────┘  └─────────────┘  └──────────────┘
```

---

## Layers

### HTTP Layer — `internal/api/`

Handles incoming requests. Parses body, calls service, writes response. No business logic or DB access.

- `health.go` — `GET /health`
- `task.go` — task CRUD + dead-letter endpoints
- `workflow.go` — workflow start, list, get
- `middleware.go` — logs every request (method, path, status, duration) and records HTTP histogram metric

### Service Layer — `internal/service/`

Business logic. Validates inputs, sets defaults, coordinates repositories and queue client. Emits metrics and structured logs on key operations.

### Repository Layer — `internal/repository/`

All SQL. No business logic.

- `task.go` — Task model, CRUD
- `workflow.go` — Workflow + WorkflowStep models, transactional create, status updates with explicit enum casting (`$1::step_status`)

### Workflow Engine — `internal/workflow/`

Orchestrates step execution in a goroutine per workflow. Non-blocking — HTTP returns immediately after workflow is created.

- `engine.go` — `StartWorkflow`, `execute`, `failWorkflow`
- `handlers.go` — `StepDefinition` type, built-in step handler functions

### Worker — `internal/worker/`

Asynq-based async task processor. Independent from the workflow engine.

### Observability — `pkg/logger` + `pkg/metrics`

- **logger**: Zap-based structured logger. Development mode: colored console. Production: JSON only.
- **metrics**: Prometheus counters, gauges, and histograms registered at startup. Exposed via `GET /metrics`.

---

## Data Flow: Task Queue

```
POST /api/v1/tasks
  │
  ▼
TaskHandler → TaskService
  │
  ├── TaskRepository.Create()        → INSERT into PostgreSQL
  ├── queue.Client.EnqueueTask()     → push to Redis
  ├── metrics.TasksEnqueued.Inc()    → increment counter
  └── logger.Info("task enqueued")   → structured log
            │
      Asynq Worker (goroutine)
            │
      handler fn
            │
      ├── metrics.TasksProcessed.Inc()
      ├── metrics.TaskDuration.Observe()
      └── return nil / err → retry / dead-letter
```

---

## Data Flow: Workflow Execution

```
POST /api/v1/workflows
  │
  ▼
WorkflowService.StartOrderWorkflow()
  │
  ├── engine.StartWorkflow()
  │     ├── INSERT workflow + steps → PostgreSQL (transaction)
  │     ├── metrics.WorkflowsStarted.Inc()
  │     └── go execute()  ← goroutine, non-blocking
  │
  └── return workflow record immediately

goroutine: execute()
  │
  for each step:
    ├── UPDATE step status = running
    ├── call StepHandler(input) → map[string]any, error
    ├── UPDATE step status = completed + store output
    └── logger.Info("step completed")
  │
  all done:
    ├── UPDATE workflow status = completed
    ├── metrics.WorkflowsCompleted{status="completed"}.Inc()
    ├── metrics.WorkflowDuration.Observe()
    └── logger.Info("workflow completed")
```

---

## Data Flow: Compensation (Saga Pattern)

```
StepHandler returns error
  │
  ▼
execute() calls failWorkflow()
  │
  ├── UPDATE workflow status = compensating
  ├── logger.Warn("workflow compensating")
  │
  iterate completed steps in REVERSE order:
    if step.CompensationTaskType != nil:
      ├── UPDATE step status = compensating
      ├── call compensation StepHandler()
      ├── UPDATE step status = compensated
      └── logger.Info("step compensated")
  │
  ├── UPDATE workflow status = compensated
  └── metrics.WorkflowsCompleted{status="compensated"}.Inc()
```

---

## Data Flow: Graceful Shutdown

```
SIGTERM or CTRL+C
  │
  ▼
signal.Notify → quit channel unblocks
  │
  ├── app.ShutdownWithContext(30s)  — Fiber drains HTTP
  ├── workerSrv.Shutdown()          — Asynq waits for tasks
  ├── defer db.Close()
  └── defer queueClient.Close()
```

---

## Observability Stack

```
GoFlow (:8080/metrics)
    │  scrape every 5s
    ▼
Prometheus (:9090)
    │  data source
    ▼
Grafana (:3000)
    │
    └── Dashboard panels:
        - Tasks enqueued total
        - Tasks processed (success vs failure)
        - Workflows started
        - Workflow outcomes (completed vs compensated)
        - HTTP p95 latency by route
```

### Metrics reference

| Metric | Type | Labels |
|---|---|---|
| `goflow_tasks_enqueued_total` | Counter | `type`, `queue` |
| `goflow_tasks_processed_total` | Counter | `type`, `status` |
| `goflow_task_duration_seconds` | Histogram | `type` |
| `goflow_dead_tasks_total` | Gauge | `queue` |
| `goflow_workflows_started_total` | Counter | `name` |
| `goflow_workflows_completed_total` | Counter | `name`, `status` |
| `goflow_workflow_duration_seconds` | Histogram | `name` |
| `goflow_http_request_duration_seconds` | Histogram | `method`, `route`, `status_code` |

---

## Database Schema

### `tasks`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK |
| `type` | VARCHAR | Handler identifier |
| `payload` | JSONB | Task input |
| `status` | ENUM | `pending` `processing` `completed` `failed` `dead` |
| `priority` | INTEGER | |
| `max_retries` | INTEGER | |
| `retries` | INTEGER | |
| `last_error` | TEXT | Nullable |
| `run_at` | TIMESTAMPTZ | Supports delayed tasks |
| `created_at` | TIMESTAMPTZ | |
| `updated_at` | TIMESTAMPTZ | |

### `workflows`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK |
| `name` | VARCHAR | e.g. `order:process` |
| `status` | ENUM | `pending` `running` `completed` `failed` `compensating` `compensated` |
| `input` | JSONB | Workflow-level input |
| `current_step` | INTEGER | Index of active step |
| `last_error` | TEXT | Nullable |
| `created_at` | TIMESTAMPTZ | |
| `updated_at` | TIMESTAMPTZ | |

### `workflow_steps`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK |
| `workflow_id` | UUID | FK → workflows |
| `step_index` | INTEGER | 0-based execution order |
| `name` | VARCHAR | Human-readable |
| `task_type` | VARCHAR | Maps to StepHandler |
| `status` | ENUM | `pending` `running` `completed` `failed` `compensating` `compensated` `skipped` |
| `input` | JSONB | Step input |
| `output` | JSONB | Nullable, set on success |
| `last_error` | TEXT | Nullable |
| `compensation_task_type` | VARCHAR | Nullable |
| `started_at` | TIMESTAMPTZ | Set on status→running |
| `completed_at` | TIMESTAMPTZ | Set on terminal status |
| `created_at` | TIMESTAMPTZ | |

---

## Infrastructure

| Service | Image | Port | Purpose |
|---|---|---|---|
| PostgreSQL | `postgres:16-alpine` | 5432 | Task + workflow persistence |
| Redis | `redis:7-alpine` | 6379 | Asynq queue + dead-letter |
| Prometheus | `prom/prometheus:latest` | 9090 | Metrics collection |
| Grafana | `grafana/grafana:latest` | 3000 | Metrics visualisation |

---

## Key Design Decisions

**Write-then-enqueue**: PostgreSQL write happens before Redis enqueue. Prevents silent loss if Redis is briefly unavailable.

**Workflow in goroutine, not separate process**: Simple for now. Tradeoff: crash mid-workflow means the workflow won't resume. Crash recovery is a planned feature.

**Saga compensation is reverse-order and best-effort**: Mirrors transaction rollback semantics. If a compensation handler itself fails, it logs and continues — the workflow still ends in `compensated`.

**Explicit enum casting**: `$1::step_status` in SQL updates. Required because `lib/pq` doesn't automatically coerce strings to PostgreSQL enums in all contexts.

**SELinux-aware volume mount**: Prometheus config mounted with `:z` flag so SELinux relabels the file context for Docker access.

**Structured logging everywhere**: All `log.Printf` replaced with `logger.Info/Error/Warn` from Zap. Every log line includes structured fields — no string formatting, fully machine-parseable.

---

## What's Not Built Yet

- Crash recovery for in-flight workflows
- Step output chaining (pass step N output as input to step N+1)
- Parallel step execution
- Conditional workflow branching
- Task status sync to PostgreSQL post-processing
- Auth (JWT)
- Makefile + Dockerfile for the Go app
- Full containerised deployment