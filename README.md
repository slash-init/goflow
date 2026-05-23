# GoFlow

A distributed task queue and workflow engine built in Go. Tasks are enqueued via a REST API, persisted in PostgreSQL, queued through Redis, and processed by concurrent workers with automatic retries, priority queues, dead-letter queue management, and multi-step workflow orchestration with compensation.

---

## Requirements

- Go 1.21+
- Docker & Docker Compose

---

## Quick Start

```bash
# Clone the repo
git clone https://github.com/yourusername/goflow.git
cd goflow

# Start infrastructure
docker compose up -d

# Run database migrations
docker exec -i goflow_postgres psql -U goflow -d goflow_db < migrations/001_create_tasks.sql
docker exec -i goflow_postgres psql -U goflow -d goflow_db < migrations/002_create_workflows.sql

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

### Tasks

#### Create a task

```
POST /api/v1/tasks
```

Body:
```json
{
  "type": "email:send",
  "payload": { "to": "user@example.com", "subject": "Welcome" },
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
| `max_retries` | int | no | Retry attempts on failure (default 3) |
| `queue` | string | no | `critical`, `default`, or `low` (default: `default`) |
| `delay_seconds` | int | no | Delay before processing (default 0) |

Response `201 Created`: task object.

---

#### List all tasks

```
GET /api/v1/tasks
```

---

#### Get a task by ID

```
GET /api/v1/tasks/:id
```

---

#### List dead tasks

```
GET /api/v1/tasks/dead?queue=default
```

Response:
```json
{
  "queue": "default",
  "count": 1,
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

#### Retry a dead task

```
POST /api/v1/tasks/dead/:id/retry?queue=default
```

---

#### Delete a dead task

```
DELETE /api/v1/tasks/dead/:id?queue=default
```

---

### Workflows

#### Start a workflow

```
POST /api/v1/workflows
```

Body:
```json
{
  "name": "order:process",
  "input": {
    "order_id": "order_001",
    "amount": 99.99
  }
}
```

The workflow starts immediately and runs in the background. The API returns as soon as the workflow is created — you don't wait for it to finish.

Response `201 Created`:
```json
{
  "ID": "uuid",
  "Name": "order:process",
  "Status": "running",
  "CurrentStep": 0,
  "CreatedAt": "..."
}
```

---

#### List all workflows

```
GET /api/v1/workflows
```

---

#### Get a workflow with step details

```
GET /api/v1/workflows/:id
```

Response:
```json
{
  "workflow": {
    "ID": "uuid",
    "Name": "order:process",
    "Status": "completed",
    "CurrentStep": 3,
    "LastError": null
  },
  "steps": [
    {
      "StepIndex": 0,
      "Name": "Reserve Inventory",
      "TaskType": "inventory:reserve",
      "Status": "completed",
      "Output": "...",
      "StartedAt": "...",
      "CompletedAt": "..."
    }
  ]
}
```

Possible workflow statuses: `pending`, `running`, `completed`, `failed`, `compensating`, `compensated`.
Possible step statuses: `pending`, `running`, `completed`, `failed`, `compensating`, `compensated`, `skipped`.

---

## Supported Task Types

| Type | Description |
|---|---|
| `email:send` | Sends an email (logs in dev) |
| `report:generate` | Generates a report for a user |
| `task:fail` | Always fails — used for testing dead-letter flow |

---

## Built-in Workflow Step Handlers

| Task Type | Description | Has Compensation |
|---|---|---|
| `inventory:reserve` | Reserves inventory for an order | yes → `inventory:release` |
| `inventory:release` | Releases reserved inventory (compensation) | no |
| `payment:charge` | Charges payment for an order | yes → `payment:refund` |
| `payment:refund` | Refunds a charge (compensation) | no |
| `notification:send` | Sends order confirmation | no |

To add a new step handler: implement a `StepHandler` function and call `engine.RegisterHandler(taskType, fn)`.

---

## Workflow: Order Processing

The built-in `order:process` workflow runs three steps in sequence:

```
Step 1: inventory:reserve   → compensation: inventory:release
Step 2: payment:charge      → compensation: payment:refund
Step 3: notification:send   → no compensation
```

If any step fails, compensation runs in reverse order for all previously completed steps. For example, if `payment:charge` fails, `inventory:release` runs to undo step 1.

To trigger a failure for testing, set `amount > 1000` — the payment handler will decline it.

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
- After `max_retries` exhausted, tasks move to the dead-letter queue
- Dead tasks are inspectable, retryable, and deletable via API

---

## Graceful Shutdown

The server handles `CTRL+C` and `SIGTERM` cleanly:

1. Stops accepting new HTTP requests
2. Waits up to 30 seconds for in-flight HTTP requests to finish
3. Waits for any currently-running worker tasks to complete
4. Closes DB and Redis connections

---

## Project Structure

```
goflow/
├── cmd/server/main.go          # Entry point — wires everything, handles shutdown
├── internal/
│   ├── api/                    # HTTP handlers
│   │   ├── health.go
│   │   ├── task.go
│   │   └── workflow.go
│   ├── service/
│   │   ├── task.go             # Task business logic
│   │   └── workflow.go         # Workflow business logic
│   ├── repository/
│   │   ├── task.go             # Task DB queries + model
│   │   └── workflow.go         # Workflow + step DB queries + models
│   ├── worker/
│   │   ├── server.go           # Asynq worker server
│   │   └── handlers.go         # Task processing functions
│   └── workflow/
│       ├── engine.go           # Workflow orchestration engine
│       └── handlers.go         # Step handler functions + StepDefinition
├── pkg/
│   ├── config/config.go        # Env config via Viper
│   ├── database/postgres.go    # PostgreSQL connection
│   └── queue/client.go         # Asynq client + Inspector
├── migrations/
│   ├── 001_create_tasks.sql
│   └── 002_create_workflows.sql
├── docker-compose.yml
├── .env
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

- Observability (Prometheus metrics, OpenTelemetry tracing, Grafana dashboard)
- Admin UI (monitor queues, tasks, workflows in browser)
- Auth (JWT)
- Docker image for the Go app + full containerised deployment
- Parallel workflow steps