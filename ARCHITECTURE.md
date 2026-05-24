# GoFlow — Architecture

## Overview

GoFlow is a distributed task queue and workflow engine. It separates task submission (HTTP API) from task processing (workers) using Redis as the message broker and PostgreSQL as the source of truth. A workflow engine orchestrates multi-step processes with automatic Saga compensation on failure. All components emit structured logs and Prometheus metrics, and the entire stack runs in Docker.

---

## Component Map

```
┌──────────────────────────────────────────────────────────────────────┐
│                           GoFlow Process                             │
│                                                                      │
│  ┌────────────────┐   ┌──────────────────┐   ┌──────────────────┐  │
│  │   Fiber HTTP   │   │   Asynq Worker   │   │ Workflow Engine  │  │
│  │  + Middleware  │   │   (goroutine)    │   │  (goroutine per  │  │
│  │                │   │                  │   │   workflow)      │  │
│  │  /health       │   │  email:send      │   │                  │  │
│  │  /metrics      │   │  report:generate │   │  execute()       │  │
│  │  /swagger/*    │   │  task:fail       │   │  failWorkflow()  │  │
│  │  /api/v1/tasks │   └────────┬─────────┘   └────────┬─────────┘  │
│  │  /api/v1/      │            │                       │             │
│  │    workflows   │            └──────────┬────────────┘             │
│  └───────┬────────┘                       │                          │
│          │                                │                          │
│  ┌───────▼────────────────────────────────▼──────┐                  │
│  │                   Service Layer                │                  │
│  │        TaskService         WorkflowService     │                  │
│  └───────┬─────────────────────────┬─────────────┘                  │
│          │                         │                                 │
│  ┌───────▼────────┐  ┌─────────────▼──────┐                        │
│  │   Task Repo    │  │   Workflow Repo     │                        │
│  └───────┬────────┘  └─────────────┬──────┘                        │
│          │            ┌─────────────▼──────┐                        │
│          │            │   Queue Client      │                        │
│          │            │  asynq.Client       │                        │
│          │            │  asynq.Inspector    │                        │
│          │            └─────────────┬───────┘                       │
│                                     │                               │
│  ┌──────────────────────────────────────────┐                      │
│  │              Observability               │                      │
│  │  pkg/logger  — Zap structured logging   │                      │
│  │  pkg/metrics — Prometheus counters,     │                      │
│  │                gauges, histograms        │                      │
│  └──────────────────────────────────────────┘                      │
└──────────────────────┬───────────────────────────────────────────── ┘
                       │
        ┌──────────────┼──────────────────┐
  ┌─────▼──────┐ ┌─────▼──────┐ ┌────────▼──────┐
  │ PostgreSQL │ │   Redis    │ │  Prometheus   │
  │            │ │  (Asynq)  │ │  + Grafana    │
  └────────────┘ └────────────┘ └───────────────┘
```

---

## Layers

### HTTP Layer — `internal/api/`

Parses requests, calls service, writes responses. No business logic or DB access.

- `middleware.go` — logs every request + records HTTP duration histogram
- `health.go` — `GET /health`
- `task.go` — task CRUD + dead-letter queue endpoints
- `workflow.go` — workflow start, list, detail

All handlers have Swagger doc comments — interactive docs at `/swagger/index.html`.

### Service Layer — `internal/service/`

Business logic. Validates inputs, sets defaults, coordinates repo and queue client. Emits metrics and structured logs on key operations.

### Repository Layer — `internal/repository/`

All SQL. No business logic. Uses `sqlx` with raw queries. Explicit enum casting (`$1::step_status`) required for `lib/pq` compatibility.

### Workflow Engine — `internal/workflow/`

Spawns a goroutine per workflow. Non-blocking — HTTP returns immediately after workflow record is created.

- `engine.go` — orchestration: `StartWorkflow`, `execute`, `failWorkflow`
- `handlers.go` — `StepDefinition` type + built-in step handler functions

### Worker — `internal/worker/`

Asynq-based async task processor. Runs concurrently with HTTP server and workflow engine. Handles retries and dead-letter automatically.

### Observability — `pkg/logger` + `pkg/metrics`

- **Zap logger**: structured JSON. Dev mode adds color + caller info. All `log.Printf` eliminated.
- **Prometheus**: counters, histograms, and gauges registered at startup. Exposed at `GET /metrics`. Scraped by Prometheus every 5s. Visualised in Grafana.

---

## Data Flows

### Enqueue a Task

```
POST /api/v1/tasks
  → TaskHandler.CreateTask()
  → TaskService.CreateTask()
      → TaskRepository.Create()         INSERT into PostgreSQL
      → queue.Client.EnqueueTask()      push to Redis
      → metrics.TasksEnqueued.Inc()
      → logger.Info("task enqueued")
  → return 201

Asynq Worker (goroutine)
  → handler fn executes
  → metrics.TasksProcessed.Inc()
  → metrics.TaskDuration.Observe()
  → return nil → success
  → return err → exponential backoff retry
                → max retries hit → archived in Redis (dead-letter)
```

### Workflow Execution

```
POST /api/v1/workflows
  → WorkflowService.StartOrderWorkflow()
  → engine.StartWorkflow()
      → INSERT workflow + steps in transaction → PostgreSQL
      → metrics.WorkflowsStarted.Inc()
      → go execute()   ← goroutine, non-blocking
  → return 201 immediately

goroutine execute():
  for each step (sequential):
    UPDATE step status = running
    call StepHandler(input) → output, err
    if err:
      UPDATE step status = failed
      call failWorkflow() → compensation
      return
    UPDATE step status = completed, store output
  UPDATE workflow status = completed
  metrics.WorkflowsCompleted{status="completed"}.Inc()
  metrics.WorkflowDuration.Observe()
```

### Saga Compensation

```
failWorkflow() called on step failure:
  UPDATE workflow status = compensating

  iterate completed steps in REVERSE:
    if step.CompensationTaskType != nil:
      UPDATE step status = compensating
      call compensation handler
      UPDATE step status = compensated

  UPDATE workflow status = compensated
  metrics.WorkflowsCompleted{status="compensated"}.Inc()
```

Compensation is best-effort. If a compensation handler fails, it logs the error and continues to the next step. The workflow still ends in `compensated`.

### Dead-Letter Queue

```
task fails max_retries times
  → Asynq moves to Redis sorted set: asynq:{queue}:archived

GET  /api/v1/tasks/dead        → Inspector.ListArchivedTasks(queue, PageSize(100))
POST /api/v1/tasks/dead/:id/retry → Inspector.RunArchivedTask(queue, id)
DELETE /api/v1/tasks/dead/:id  → Inspector.DeleteArchivedTask(queue, id)
```

### Graceful Shutdown

```
SIGTERM / CTRL+C
  → signal.Notify fires → quit channel unblocks
  → app.ShutdownWithContext(30s)   Fiber drains in-flight HTTP
  → workerSrv.Shutdown()           Asynq waits for running tasks
  → defer db.Close()
  → defer queueClient.Close()
  → logger.Info("shutdown complete")
  → process exits 0
```

---

## Database Schema

### `tasks`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK, auto-generated |
| `type` | VARCHAR(255) | Handler identifier |
| `payload` | JSONB | Task input |
| `status` | ENUM | `pending` `processing` `completed` `failed` `dead` |
| `priority` | INTEGER | Higher = more urgent |
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
| `name` | VARCHAR(255) | e.g. `order:process` |
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
| `step_index` | INTEGER | 0-based |
| `name` | VARCHAR(255) | Human-readable |
| `task_type` | VARCHAR(255) | Maps to StepHandler |
| `status` | ENUM | `pending` `running` `completed` `failed` `compensating` `compensated` `skipped` |
| `input` | JSONB | Step input |
| `output` | JSONB | Nullable, set on success |
| `last_error` | TEXT | Nullable |
| `compensation_task_type` | VARCHAR(255) | Nullable |
| `started_at` | TIMESTAMPTZ | Set on status→running |
| `completed_at` | TIMESTAMPTZ | Set on terminal status |
| `created_at` | TIMESTAMPTZ | |

---

## Infrastructure

| Service | Image | Port | Purpose |
|---|---|---|---|
| PostgreSQL | `postgres:16-alpine` | 5432 | Task + workflow persistence |
| Redis | `redis:7-alpine` | 6379 | Asynq queue + dead-letter store |
| GoFlow App | `goflow:latest` | 8080 | API + worker + workflow engine |
| Prometheus | `prom/prometheus:latest` | 9090 | Metrics collection |
| Grafana | `grafana/grafana:latest` | 3000 | Metrics visualisation |

Startup order enforced via Docker Compose healthchecks:
`postgres` (healthy) + `redis` (healthy) → `app` starts.

---

## Build

```
golang:1.25-alpine (builder)
  COPY go.mod go.sum → go mod download (cached layer)
  COPY . .
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"
  → /app/bin/goflow (static binary)

alpine:3.19 (final)
  COPY --from=builder /app/bin/goflow
  USER goflow (non-root)
  EXPOSE 8080
  → ~15MB image
```

`CGO_ENABLED=0` produces a fully static binary — no glibc dependency, runs on Alpine without modification.

---

## Key Design Decisions

**Write-then-enqueue**: PostgreSQL write before Redis enqueue. Task record always exists even if Redis is temporarily unavailable.

**Workflow per goroutine, not per process**: Simple and sufficient for current scale. Tradeoff: process crash loses in-flight workflow state. Crash recovery (resume from `current_step`) is a planned enhancement.

**Saga compensation is reverse-order and best-effort**: Mirrors transaction rollback. Compensation failure is logged and skipped — the workflow still ends in `compensated` rather than hanging indefinitely.

**Explicit PostgreSQL enum casting**: `$1::step_status` required because `lib/pq` doesn't automatically coerce strings to enums in all query contexts. Silent failure without the cast.

**SELinux-aware volume mount**: Prometheus config uses `:z` volume flag to relabel the SELinux context so Docker can read files from the user home directory.

**Static binary in minimal image**: `CGO_ENABLED=0` + Alpine base = ~15MB image with zero Go toolchain overhead. Production-safe.

**Structured logging everywhere**: Zero `log.Printf` in the codebase. Every log line is machine-parseable JSON with consistent fields. Lint enforced via `golangci-lint`.

---

## What Could Be Added Next

- Crash recovery: on startup, resume workflows stuck in `running` state from `current_step`
- Step output chaining: pass step N output as input to step N+1
- Parallel step execution: fan-out steps that don't depend on each other
- Conditional branching: skip steps based on previous output
- Task status sync: update PostgreSQL task record after Asynq processes it
- JWT authentication on API endpoints
- Rate limiting per client
- Separate worker binary (`cmd/worker/`) for independent horizontal scaling
- GitHub Actions CI pipeline (lint + test on every PR)