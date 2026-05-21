# GoFlow — Architecture

## Overview

GoFlow is a distributed task queue and workflow engine. It separates task submission (HTTP API) from task processing (workers) using Redis as the message broker and PostgreSQL as the source of truth.

A request to enqueue a task goes through three layers before the worker picks it up:

```
Client → HTTP Handler → Service → Repository → PostgreSQL
                                ↘
                                  Asynq Client → Redis → Worker → Handler fn
```

---

## Component Map

```
┌─────────────────────────────────────────────────────────┐
│                       GoFlow Process                    │
│                                                         │
│  ┌─────────────┐         ┌──────────────────────┐      │
│  │  Fiber HTTP │         │    Asynq Worker       │      │
│  │   Server    │         │    (goroutine)        │      │
│  │             │         │                       │      │
│  │  /health    │         │  mux.HandleFunc(      │      │
│  │  /api/v1/   │         │    "email:send"       │      │
│  │    tasks    │         │    "report:generate"  │      │
│  │  /api/v1/   │         │    "task:fail"        │      │
│  │  tasks/dead │         │  )                    │      │
│  └──────┬──────┘         └──────────┬────────────┘      │
│         │                           │                    │
│  ┌──────▼──────┐           ┌────────▼────────┐          │
│  │   Handler   │           │  Handler fns    │          │
│  │  (api/)     │           │  (worker/)      │          │
│  └──────┬──────┘           └─────────────────┘          │
│         │                                                │
│  ┌──────▼──────┐                                        │
│  │   Service   │                                        │
│  │  (service/) │                                        │
│  └──────┬──────┘                                        │
│         │                                                │
│  ┌──────▼──────┐   ┌────────────────────────┐          │
│  │  Repository │   │      Queue Client       │          │
│  │ (repository)│   │  asynq.Client (enqueue) │          │
│  │             │   │  asynq.Inspector (DLQ)  │          │
│  └──────┬──────┘   └──────────┬─────────────┘          │
└─────────┼────────────────────┼──────────────────────────┘
          │                    │
    ┌─────▼──────┐   ┌─────────▼───┐
    │ PostgreSQL │   │    Redis     │
    │            │   │   (Asynq)   │
    └────────────┘   └─────────────┘
```

---

## Layers

### HTTP Layer — `internal/api/`

Handles incoming requests. Responsible for parsing the request body, calling the service, and writing the HTTP response. Has no business logic and no direct database access.

Files:
- `health.go` — `GET /health`
- `task.go` — all task and dead-letter endpoints

### Service Layer — `internal/service/`

Contains all business logic. Validates inputs, sets defaults, coordinates between the repository and the queue client. Does not know about HTTP or SQL.

Files:
- `task.go` — `CreateTask`, `GetAllTasks`, `GetTaskByID`, `ListDeadTasks`, `RetryDeadTask`, `DeleteDeadTask`

Key decisions made here:
- Default `max_retries` to 3 if not provided
- Default queue to `"default"` if not specified
- Calculate `run_at` from `delay_seconds`
- Save to PostgreSQL before enqueuing to Redis (write-then-enqueue order)

### Repository Layer — `internal/repository/`

Only talks to the database. No business logic. All SQL lives here.

Files:
- `task.go` — `Task` struct, `TaskRepository`, `Create`, `GetAll`, `GetByID`

### Worker — `internal/worker/`

Runs in a goroutine alongside the HTTP server. Polls Redis via Asynq and dispatches tasks to registered handler functions. On graceful shutdown, finishes any in-flight task before exiting.

Files:
- `server.go` — creates the Asynq server with priority queue config, registers handlers
- `handlers.go` — one function per task type; receives the task, unmarshals payload, does the work

### Shared Packages — `pkg/`

| Package | File | Purpose |
|---|---|---|
| `config` | `config.go` | Loads `.env` via Viper, exposes typed `Config` struct |
| `database` | `postgres.go` | Opens and validates a `*sqlx.DB` connection |
| `queue` | `client.go` | Wraps `asynq.Client` (enqueue) + `asynq.Inspector` (DLQ ops) |

---

## Data Flow: Enqueuing a Task

```
POST /api/v1/tasks
  │
  ▼
TaskHandler.CreateTask()           — parse body, call service
  │
  ▼
TaskService.CreateTask()           — validate, set defaults
  │
  ├──▶ TaskRepository.Create()    — INSERT into PostgreSQL, get back ID + timestamps
  │
  └──▶ queue.Client.EnqueueTask() — push to Redis via Asynq
              │
              ▼
          Asynq Worker (goroutine)
              │
              ▼
          Handler fn (e.g. HandleSendEmail)
              │
              ▼
          return nil → success
          return err → triggers retry
```

---

## Data Flow: Retry & Dead-Letter

```
Handler returns error
  │
  ▼
Asynq: retries < max_retries?
  ├── yes → exponential backoff → re-queue
  └── no  → move to archived set in Redis (dead-letter queue)
                    │
                    ▼
          GET  /api/v1/tasks/dead        — list dead tasks via Inspector
          POST /api/v1/tasks/dead/:id/retry  — RunArchivedTask()
          DELETE /api/v1/tasks/dead/:id      — DeleteArchivedTask()
```

Dead tasks are stored in a Redis sorted set: `asynq:{queue}:archived`.
Inspected and managed via `asynq.Inspector` — no direct Redis access needed.

---

## Data Flow: Graceful Shutdown

```
SIGTERM or CTRL+C
  │
  ▼
os/signal → quit channel unblocks
  │
  ├──▶ app.ShutdownWithContext(30s) — Fiber stops accepting, drains in-flight HTTP
  │
  ├──▶ workerSrv.Shutdown()         — Asynq waits for running tasks to finish
  │
  └──▶ defer db.Close()             — PostgreSQL connection closed
       defer queueClient.Close()    — Redis connection closed
```

The 30-second timeout is a hard ceiling — if tasks don't finish in time, the process exits anyway.

---

## Database Schema

### `tasks` table

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key, auto-generated |
| `type` | VARCHAR(255) | Task type identifier e.g. `email:send` |
| `payload` | JSONB | Arbitrary task data |
| `status` | ENUM | `pending`, `processing`, `completed`, `failed`, `dead` |
| `priority` | INTEGER | Higher = more important |
| `max_retries` | INTEGER | Max retry attempts |
| `retries` | INTEGER | Current retry count |
| `last_error` | TEXT | Nullable — last failure message |
| `run_at` | TIMESTAMPTZ | When to process (supports delayed tasks) |
| `created_at` | TIMESTAMPTZ | Auto-set |
| `updated_at` | TIMESTAMPTZ | Auto-set |

Indexes: `idx_tasks_status`, `idx_tasks_run_at`

Note: Task status in PostgreSQL is not automatically synced by Asynq. The DB record reflects the state at enqueue time. Live status lives in Redis. Status sync is a planned feature.

---

## Queue Configuration

Asynq uses named priority queues with weighted processing:

| Queue | Weight | Relative throughput |
|---|---|---|
| `critical` | 6 | ~60% of worker capacity |
| `default` | 3 | ~30% of worker capacity |
| `low` | 1 | ~10% of worker capacity |

Worker concurrency: 10 simultaneous tasks.
Retry backoff: exponential, managed by Asynq.
Dead-letter storage: Redis sorted set per queue (`asynq:{queue}:archived`).

---

## Infrastructure

| Service | Image | Port | Purpose |
|---|---|---|---|
| PostgreSQL | `postgres:16-alpine` | 5432 | Task persistence, workflow state |
| Redis | `redis:7-alpine` | 6379 | Asynq queue broker + dead-letter store |

Both run via Docker Compose. PostgreSQL data persisted in named volume (`postgres_data`). Redis data is ephemeral by default.

---

## Key Design Decisions

**Write-then-enqueue**: Tasks are saved to PostgreSQL before being pushed to Redis. If Redis is temporarily unavailable, the task record still exists and can be re-enqueued. If PostgreSQL fails, nothing is enqueued.

**Worker runs in the same process as the HTTP server**: Kept simple using a goroutine. In production this would be a separate binary (`cmd/worker/main.go`) to scale independently.

**Graceful shutdown with bounded timeout**: The process gives tasks 30 seconds to finish. This is long enough for most tasks but prevents indefinite hangs on shutdown.

**Dead-letter queue via Inspector, not direct Redis**: Asynq's `Inspector` API abstracts Redis internals. This means the DLQ implementation won't break if Asynq changes its internal storage format.

**Statically typed payloads in workers**: Each handler defines its own payload struct and unmarshals from raw bytes. Handlers are self-contained and easy to unit test.

**No ORM**: Uses `sqlx` with raw SQL. Full control over queries, no magic, easy to reason about performance.

---

## What's Not Built Yet

- Task status sync back to PostgreSQL after worker processes
- Workflow engine (multi-step, sequential/parallel, compensation)
- Observability (Prometheus, OpenTelemetry, Grafana)
- Admin dashboard
- Auth (JWT)
- Separate worker binary for independent scaling
- Docker image for the Go app