# GoFlow — Architecture

## Overview

GoFlow is a distributed task queue and workflow engine. It separates task submission (HTTP API) from task processing (workers) using Redis as the message broker and PostgreSQL as the source of truth. On top of the task queue sits a workflow engine that orchestrates multi-step processes with automatic compensation on failure.

---

## Component Map

```
┌──────────────────────────────────────────────────────────────┐
│                        GoFlow Process                        │
│                                                              │
│  ┌─────────────┐         ┌──────────────────────┐           │
│  │  Fiber HTTP │         │    Asynq Worker       │           │
│  │   Server    │         │    (goroutine)        │           │
│  │             │         │                       │           │
│  │  /health    │         │  email:send           │           │
│  │  /api/v1/   │         │  report:generate      │           │
│  │    tasks    │         │  task:fail            │           │
│  │  /api/v1/   │         └──────────────────────-┘           │
│  │  workflows  │                                             │
│  └──────┬──────┘         ┌──────────────────────┐           │
│         │                │   Workflow Engine     │           │
│         │                │   (goroutine)        │           │
│         │                │                      │           │
│         │                │  execute()           │           │
│         │                │  failWorkflow()      │           │
│         │                │  StepHandlers        │           │
│         │                └──────────┬───────────┘           │
│         │                           │                        │
│  ┌──────▼───────────────────────────▼──────┐                │
│  │              Service Layer              │                │
│  │   TaskService        WorkflowService    │                │
│  └──────┬───────────────────────┬──────────┘                │
│         │                       │                            │
│  ┌──────▼──────┐   ┌────────────▼────────┐                 │
│  │  Task Repo  │   │  Workflow Repo       │                 │
│  └──────┬──────┘   └────────────┬─────────┘                 │
│         │          ┌────────────▼─────────┐                 │
│         │          │  Queue Client        │                 │
│         │          │  asynq.Client        │                 │
│         │          │  asynq.Inspector     │                 │
│         │          └────────────┬─────────┘                 │
└─────────┼───────────────────────┼───────────────────────────┘
          │                       │
    ┌─────▼──────┐        ┌───────▼─────┐
    │ PostgreSQL │        │    Redis    │
    └────────────┘        └─────────────┘
```

---

## Layers

### HTTP Layer — `internal/api/`

Handles incoming requests. Parses body, calls service, writes response. No business logic, no DB access.

- `health.go` — `GET /health`
- `task.go` — task CRUD + dead-letter endpoints
- `workflow.go` — `POST /api/v1/workflows`, `GET /api/v1/workflows`, `GET /api/v1/workflows/:id`

### Service Layer — `internal/service/`

Business logic. Validates inputs, sets defaults, coordinates repositories and queue client.

- `task.go` — task creation, listing, dead-letter operations
- `workflow.go` — builds step definitions, delegates to engine

### Repository Layer — `internal/repository/`

All SQL. No business logic.

- `task.go` — `Task` model, CRUD
- `workflow.go` — `Workflow` + `WorkflowStep` models, transactional create, status updates

### Workflow Engine — `internal/workflow/`

Orchestrates step execution. Runs in a goroutine per workflow — non-blocking.

- `engine.go` — `StartWorkflow`, `execute`, `failWorkflow`
- `handlers.go` — `StepDefinition` type, built-in step handler functions

### Worker — `internal/worker/`

Asynq-based task processor. Separate from the workflow engine — handles one-off async tasks.

- `server.go` — Asynq server config + handler registration
- `handlers.go` — `HandleSendEmail`, `HandleGenerateReport`, `HandleAlwaysFail`

### Shared Packages — `pkg/`

| Package | Purpose |
|---|---|
| `config` | Loads `.env` via Viper |
| `database` | PostgreSQL connection via sqlx |
| `queue` | Asynq client (enqueue) + Inspector (dead-letter ops) |

---

## Data Flow: Task Queue

```
POST /api/v1/tasks
  │
  ▼
TaskHandler → TaskService → TaskRepository → INSERT into PostgreSQL
                         ↘
                           queue.Client.EnqueueTask() → Redis
                                                          │
                                                    Asynq Worker
                                                          │
                                               handler fn → return nil/err
                                                          │
                                              err: retry with backoff
                                              retries exhausted: → archived set
```

---

## Data Flow: Workflow Execution

```
POST /api/v1/workflows
  │
  ▼
WorkflowHandler → WorkflowService → engine.StartWorkflow()
                                          │
                                   INSERT workflow + steps → PostgreSQL
                                          │
                                   go execute()  ← goroutine, non-blocking
                                   │
                                   API returns immediately with workflow record
                                          │
                              ┌───────────▼────────────┐
                              │   for each step:        │
                              │   UPDATE status=running │
                              │   call StepHandler()    │
                              │   UPDATE status=completed│
                              └───────────┬─────────────┘
                                          │
                              all steps done → UPDATE workflow status=completed
```

---

## Data Flow: Compensation (Saga Pattern)

When a step fails, already-completed steps are rolled back in reverse order:

```
Step 1: completed  ──────────────────────────────┐
Step 2: completed  ────────────────────┐         │
Step 3: FAILED                         │         │
  │                                    │         │
  ▼                                    ▼         ▼
failWorkflow()              compensate step 2  compensate step 1
  │                         payment:refund     inventory:release
  │
  UPDATE workflow status=compensating
  │
  iterate completed steps in reverse
  │
  for each step with CompensationTaskType:
    call compensation handler
    UPDATE step status=compensated
  │
  UPDATE workflow status=compensated
```

Compensation is best-effort — if a compensation handler fails, it logs and continues to the next step. The workflow ends in `compensated` regardless.

---

## Data Flow: Graceful Shutdown

```
SIGTERM or CTRL+C
  │
  ▼
signal.Notify → quit channel unblocks in main()
  │
  ├──▶ app.ShutdownWithContext(30s)  — Fiber drains in-flight HTTP requests
  ├──▶ workerSrv.Shutdown()          — Asynq waits for running tasks to finish
  └──▶ defer db.Close()              — PostgreSQL connection closed
       defer queueClient.Close()     — Redis connection closed
```

---

## Database Schema

### `tasks`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK, auto-generated |
| `type` | VARCHAR(255) | Handler identifier |
| `payload` | JSONB | Task input data |
| `status` | ENUM | `pending` `processing` `completed` `failed` `dead` |
| `priority` | INTEGER | Higher = more urgent |
| `max_retries` | INTEGER | Max retry attempts |
| `retries` | INTEGER | Current retry count |
| `last_error` | TEXT | Nullable |
| `run_at` | TIMESTAMPTZ | Supports delayed tasks |
| `created_at` | TIMESTAMPTZ | Auto-set |
| `updated_at` | TIMESTAMPTZ | Auto-set |

### `workflows`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK, auto-generated |
| `name` | VARCHAR(255) | Workflow identifier e.g. `order:process` |
| `status` | ENUM | `pending` `running` `completed` `failed` `compensating` `compensated` |
| `input` | JSONB | Workflow-level input |
| `current_step` | INTEGER | Index of the step currently executing |
| `last_error` | TEXT | Nullable — set on failure |
| `created_at` | TIMESTAMPTZ | Auto-set |
| `updated_at` | TIMESTAMPTZ | Auto-set |

### `workflow_steps`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK, auto-generated |
| `workflow_id` | UUID | FK → workflows.id |
| `step_index` | INTEGER | Order of execution (0-based) |
| `name` | VARCHAR(255) | Human-readable step name |
| `task_type` | VARCHAR(255) | Maps to a registered StepHandler |
| `status` | ENUM | `pending` `running` `completed` `failed` `compensating` `compensated` `skipped` |
| `input` | JSONB | Step input data |
| `output` | JSONB | Nullable — populated on success |
| `last_error` | TEXT | Nullable — populated on failure |
| `compensation_task_type` | VARCHAR(255) | Nullable — handler to run on rollback |
| `started_at` | TIMESTAMPTZ | Set when status → running |
| `completed_at` | TIMESTAMPTZ | Set when status → completed/failed/compensated |
| `created_at` | TIMESTAMPTZ | Auto-set |

Indexes: `idx_workflow_steps_workflow_id`, `idx_workflows_status`

---

## Queue Configuration

| Queue | Weight | Use for |
|---|---|---|
| `critical` | 6 | ~60% of capacity |
| `default` | 3 | ~30% of capacity |
| `low` | 1 | ~10% of capacity |

Worker concurrency: 10. Retry backoff: exponential (managed by Asynq).

---

## Infrastructure

| Service | Image | Port | Purpose |
|---|---|---|---|
| PostgreSQL | `postgres:16-alpine` | 5432 | Task + workflow persistence |
| Redis | `redis:7-alpine` | 6379 | Asynq queue broker + dead-letter store |

---

## Key Design Decisions

**Workflow runs in a goroutine, not a separate task**: Each workflow execution is a goroutine spawned by `StartWorkflow`. This keeps things simple — no separate scheduler, no polling. The tradeoff is that if the process crashes mid-workflow, the workflow won't resume automatically. Crash recovery is a planned feature.

**Saga compensation is reverse-order**: Completed steps are compensated newest-first. This mirrors how database transactions roll back — last operation undone first.

**Write-then-enqueue for tasks**: Tasks are saved to PostgreSQL before Redis. If Redis is down, the record exists and can be re-enqueued manually.

**Step output stored as JSONB**: Each step's output is persisted so later steps could reference earlier results. The current implementation passes the original workflow input to every step — chaining step outputs as inputs to subsequent steps is a planned enhancement.

**No ORM**: Raw SQL via `sqlx`. Full control, no magic, easy to audit.

**Explicit enum casting in PostgreSQL**: Status updates use `$1::step_status` and `$1::workflow_status` to avoid `lib/pq` silently failing on enum comparisons.

---

## What's Not Built Yet

- Crash recovery for in-flight workflows (process restart resumes from current step)
- Step output chaining (pass step N output as input to step N+1)
- Parallel step execution
- Conditional branching in workflows
- Task status sync to PostgreSQL after Asynq processes
- Observability (Prometheus, OpenTelemetry, Grafana)
- Admin dashboard
- Auth (JWT)
- Separate worker binary for independent scaling
- Docker image for the Go app