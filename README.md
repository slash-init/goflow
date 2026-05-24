# GoFlow

A distributed task queue and workflow engine built in Go. Tasks are enqueued via a REST API, persisted in PostgreSQL, queued through Redis, and processed by concurrent workers with automatic retries, priority queues, dead-letter queue management, multi-step workflow orchestration with compensation, and full observability via Prometheus and Grafana.

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

# Start the server
go run cmd/server/main.go
```

| Service | URL |
|---|---|
| GoFlow API | http://localhost:8080 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (admin/admin) |

---

## Environment Variables

Create a `.env` file in the project root:

```env
APP_PORT=8080
APP_ENV=development

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

### Metrics

```
GET /metrics
```

Prometheus-format metrics. Scrape this endpoint from Prometheus.

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

#### Get a task by ID

```
GET /api/v1/tasks/:id
```

#### List dead tasks

```
GET /api/v1/tasks/dead?queue=default
```

#### Retry a dead task

```
POST /api/v1/tasks/dead/:id/retry?queue=default
```

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

The workflow starts immediately and runs in the background. The API returns as soon as the workflow record is created.

To trigger compensation (for testing), set `amount > 1000` — the payment handler will decline it and compensation will run in reverse.

Response `201 Created`: workflow object.

#### List all workflows

```
GET /api/v1/workflows
```

#### Get workflow + step details

```
GET /api/v1/workflows/:id
```

Possible workflow statuses: `pending` `running` `completed` `failed` `compensating` `compensated`
Possible step statuses: `pending` `running` `completed` `failed` `compensating` `compensated` `skipped`

---

## Supported Task Types

| Type | Description |
|---|---|
| `email:send` | Sends an email (logs in dev) |
| `report:generate` | Generates a report for a user |
| `task:fail` | Always fails — used for testing dead-letter flow |

---

## Built-in Workflow Step Handlers

| Task Type | Description | Compensation |
|---|---|---|
| `inventory:reserve` | Reserves inventory | `inventory:release` |
| `inventory:release` | Releases inventory (compensation) | — |
| `payment:charge` | Charges payment | `payment:refund` |
| `payment:refund` | Refunds charge (compensation) | — |
| `notification:send` | Sends confirmation | — |

---

## Queue Priorities

| Queue | Weight | Use for |
|---|---|---|
| `critical` | 6 | Time-sensitive tasks |
| `default` | 3 | Normal tasks |
| `low` | 1 | Background tasks |

Worker concurrency: 10 simultaneous tasks.

---

## Observability

### Metrics (Prometheus)

All metrics are exposed at `GET /metrics` in Prometheus format.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `goflow_tasks_enqueued_total` | Counter | `type`, `queue` | Tasks enqueued |
| `goflow_tasks_processed_total` | Counter | `type`, `status` | Tasks processed (status: success/failure) |
| `goflow_task_duration_seconds` | Histogram | `type` | Task processing duration |
| `goflow_dead_tasks_total` | Gauge | `queue` | Tasks in dead-letter queue |
| `goflow_workflows_started_total` | Counter | `name` | Workflows started |
| `goflow_workflows_completed_total` | Counter | `name`, `status` | Workflows completed (status: completed/compensated/failed) |
| `goflow_workflow_duration_seconds` | Histogram | `name` | Workflow execution duration |
| `goflow_http_request_duration_seconds` | Histogram | `method`, `route`, `status_code` | HTTP request duration |

### Useful PromQL queries

```promql
# Workflow success vs compensation rate
rate(goflow_workflows_completed_total[1m])

# HTTP p95 latency by route
histogram_quantile(0.95, rate(goflow_http_request_duration_seconds_bucket[1m]))

# Task failure rate
rate(goflow_tasks_processed_total{status="failure"}[5m])
```

### Logging (Zap)

Structured JSON logs with level, timestamp, caller, and contextual fields:

```json
{"level":"INFO","ts":"2026-05-24T12:00:01.695+0530","msg":"step completed","workflow_id":"...","step":"Reserve Inventory"}
{"level":"ERROR","ts":"...","msg":"step failed","workflow_id":"...","step":"Charge Payment","error":"payment declined"}
```

Set `APP_ENV=production` for JSON-only logs. `APP_ENV=development` gives colored console output.

### Grafana dashboard

1. Open http://localhost:3000 (admin/admin)
2. Add Prometheus data source: URL = `http://prometheus:9090`
3. Create panels with the PromQL queries above

---

## Graceful Shutdown

Handles `CTRL+C` and `SIGTERM`:
1. Stops accepting new HTTP requests
2. Waits up to 30s for in-flight requests to finish
3. Waits for running worker tasks to complete
4. Closes DB and Redis connections

---

## Project Structure

```
goflow/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── api/                    # HTTP handlers + middleware
│   │   ├── health.go
│   │   ├── task.go
│   │   ├── workflow.go
│   │   └── middleware.go       # Request logger + metrics
│   ├── service/
│   │   ├── task.go
│   │   └── workflow.go
│   ├── repository/
│   │   ├── task.go
│   │   └── workflow.go
│   ├── worker/
│   │   ├── server.go
│   │   └── handlers.go
│   └── workflow/
│       ├── engine.go
│       └── handlers.go
├── pkg/
│   ├── config/config.go
│   ├── database/postgres.go
│   ├── queue/client.go
│   ├── logger/logger.go        # Zap structured logger
│   └── metrics/metrics.go      # Prometheus metric definitions
├── migrations/
│   ├── 001_create_tasks.sql
│   └── 002_create_workflows.sql
├── observability/
│   └── prometheus.yml          # Prometheus scrape config
├── docker-compose.yml
├── .env
└── go.mod
```

---

## Running Infrastructure

```bash
docker compose up -d            # Start all services
docker compose ps               # Check status
docker compose down             # Stop
docker compose down -v          # Stop + wipe volumes
```

---

## What's Coming

- Makefile for common dev tasks
- Dockerfile for the Go app
- Full containerised deployment
- Auth (JWT)
- Parallel workflow steps

---

## Building & Deployment

### Local development (Go runs on host machine)

```bash
# Start infrastructure only
make docker/up

# Run migrations
make migrate

# Run the server
make run
```

### Full containerised deployment (everything in Docker)

```bash
# Build the static binary first
make build

# Build Docker image + start all services
docker compose up -d --build

# Run migrations
make migrate

# Check all containers
docker compose ps
```

### Make commands

```bash
make help          # list all commands
make run           # run server locally
make build         # compile static binary to bin/goflow
make migrate       # run all SQL migrations
make migrate/reset # wipe and re-run migrations
make db/connect    # open psql session
make docker/up     # start all Docker services
make docker/down   # stop all Docker services
make docker/build  # build Docker image
make docker/logs   # tail all container logs
make lint          # run golangci-lint
make test          # run all tests
make tidy          # tidy go modules
```

### Docker image

The image is built in two steps:
1. `make build` — compiles a static Linux binary (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`)
2. `docker build` — copies the binary into a minimal Alpine image (~15MB)

The app reads config from environment variables when running in Docker — no `.env` file needed.