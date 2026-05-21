# GoFlow

A distributed task queue and workflow engine built in Go. Tasks are enqueued via a REST API, persisted in PostgreSQL, queued through Redis, and processed by concurrent workers with automatic retries, priority queues, and dead-letter queue management.

---

## Requirements

- Go 1.21+
- Docker & Docker Compose

---

## Quick Start

```bash
# Clone the repo
git clone https://github.com/slash-init/goflow.git
cd goflow

# Start infrastructure
docker compose up -d

# Run database migrations
docker exec -i goflow_postgres psql -U goflow -d goflow_db < migrations/001_create_tasks.sql

# Start the server (HTTP + worker runs together)
go run cmd/server/main.go
```

Server starts on `http://localhost:8080`.

---

## Environment Variables

Create a `.env` file in the project root:

```env
APP_PORT=8080

DB_HOST=localhost
DB_PORT=5432
DB_USER=goflow
DB_PASSWORD=goflow_secret
DB_NAME=goflow_db

REDIS_ADDR=localhost:6379
```

---

## API Reference

### Health check

```
GET /health
```

Response:
```json
{ "status": "ok", "service": "goflow" }
```

---

### Create a task

```
POST /api/v1/tasks
```

Body:
```json
{
  "type": "email:send",
  "payload": {
    "to": "user@example.com",
    "subject": "Welcome"
  },
  "priority": 1,
  "max_retries": 3,
  "queue": "default",
  "delay_seconds": 0
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | yes | Task handler identifier |
| `payload` | object | yes | Arbitrary data passed to the handler |
| `priority` | int | no | Higher = more important (default 0) |
| `max_retries` | int | no | How many times to retry on failure (default 3) |
| `queue` | string | no | `critical`, `default`, or `low` (default: `default`) |
| `delay_seconds` | int | no | Delay before processing (default 0) |

Response `201 Created`:
```json
{
  "id": "uuid",
  "type": "email:send",
  "status": "pending",
  "priority": 1,
  "max_retries": 3,
  "retries": 0,
  "run_at": "...",
  "created_at": "..."
}
```

---

### List all tasks

```
GET /api/v1/tasks
```

Response:
```json
{
  "count": 2,
  "tasks": [ ... ]
}
```

---

### Get a task by ID

```
GET /api/v1/tasks/:id
```

Response: task object or `404` if not found.

---

### List dead tasks

Tasks that have exhausted all retries land in the dead-letter queue.

```
GET /api/v1/tasks/dead?queue=default
```

| Query param | Default | Description |
|---|---|---|
| `queue` | `default` | Which queue to inspect (`critical`, `default`, `low`) |

Response:
```json
{
  "queue": "default",
  "count": 2,
  "tasks": [
    {
      "id": "uuid",
      "type": "task:fail",
      "payload": "{}",
      "queue": "default",
      "last_error": "intentional failure for testing",
      "last_failed_at": "2026-05-21T13:49:53+05:30"
    }
  ]
}
```

---

### Retry a dead task

Re-queues the task for immediate processing.

```
POST /api/v1/tasks/dead/:id/retry?queue=default
```

Response:
```json
{ "id": "uuid", "message": "task re-queued successfully" }
```

---

### Delete a dead task

Permanently removes the task from the dead-letter queue.

```
DELETE /api/v1/tasks/dead/:id?queue=default
```

Response:
```json
{ "id": "uuid", "message": "task deleted" }
```

---

## Supported Task Types

| Type | Description |
|---|---|
| `email:send` | Sends an email (logs in dev) |
| `report:generate` | Generates a report for a user |
| `task:fail` | Always fails — used for testing dead-letter flow |

To add a new task type: create a handler function in `internal/worker/handlers.go` and register it in `internal/worker/server.go`.

---

## Queue Priorities

| Queue | Weight | Use for |
|---|---|---|
| `critical` | 6 | Time-sensitive tasks |
| `default` | 3 | Normal tasks |
| `low` | 1 | Background/non-urgent tasks |

Worker concurrency: 10 simultaneous tasks.

---

## Retry & Dead-Letter Behaviour

- Failed tasks are automatically retried with exponential backoff
- After `max_retries` exhausted, tasks move to the dead-letter queue (archived in Redis)
- Dead tasks are inspectable, retryable, and deletable via API
- Retry backoff is managed by Asynq — no manual scheduling needed

---

## Graceful Shutdown

The server handles `CTRL+C` and `SIGTERM` cleanly:

1. Stops accepting new HTTP requests
2. Waits up to 30 seconds for in-flight HTTP requests to finish
3. Waits for any currently-running worker tasks to complete
4. Closes DB and Redis connections

No tasks are dropped or corrupted on shutdown.

---

## Project Structure

```
goflow/
├── cmd/server/main.go          # Entry point — wires everything, handles shutdown
├── internal/
│   ├── api/                    # HTTP handlers
│   │   ├── health.go
│   │   └── task.go
│   ├── service/
│   │   └── task.go             # Business logic
│   ├── repository/
│   │   └── task.go             # DB queries + Task model
│   └── worker/
│       ├── server.go           # Asynq worker server + handler registration
│       └── handlers.go         # Task processing functions
├── pkg/
│   ├── config/config.go        # Env config via Viper
│   ├── database/postgres.go    # PostgreSQL connection
│   └── queue/client.go         # Asynq client + Inspector (dead-letter ops)
├── migrations/
│   └── 001_create_tasks.sql    # Tasks table
├── docker-compose.yml
├── .env                        # Not committed
└── go.mod
```

---

## Running Infrastructure Only

```bash
docker compose up -d            # Start postgres + redis
docker compose ps               # Check status
docker compose down             # Stop
docker compose down -v          # Stop + delete volumes (wipes DB)
```

---

## What's Coming

- Workflow engine (sequential + parallel + compensating steps)
- Observability (Prometheus metrics, OpenTelemetry tracing, Grafana)
- Admin dashboard (monitor queues, tasks, workflows)
- Auth (JWT)
- Docker image for the Go app + full containerised deployment