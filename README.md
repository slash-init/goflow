# GoFlow

A distributed task queue and workflow engine built in Go. Tasks are enqueued via a REST API, persisted in PostgreSQL, queued through Redis, and processed by concurrent workers.

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

## Supported Task Types

| Type | Description |
|---|---|
| `email:send` | Sends an email (logs in dev) |
| `report:generate` | Generates a report for a user |

To add a new task type: create a handler function in `internal/worker/handlers.go` and register it in `internal/worker/server.go`.

---

## Queue Priorities

| Queue | Weight | Use for |
|---|---|---|
| `critical` | 6 | Time-sensitive tasks |
| `default` | 3 | Normal tasks |
| `low` | 1 | Background/non-urgent tasks |

---

## Project Structure

```
goflow/
├── cmd/server/main.go          # Entry point — wires everything together
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
│   └── queue/client.go         # Asynq enqueue client
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

- Graceful shutdown
- Dead-letter queue handling
- Workflow engine (sequential + parallel steps)
- Observability (Prometheus metrics, OpenTelemetry tracing)
- Admin dashboard
- Docker deployment for the Go app itself