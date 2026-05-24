# GoFlow

A distributed task queue and workflow engine built in Go. Production-grade patterns throughout — clean architecture, structured logging, Prometheus metrics, Saga compensation, graceful shutdown, and full Docker deployment.

---

## Requirements

- Go 1.21+
- Docker & Docker Compose

---

## Quick Start

```bash
git clone https://github.com/yourusername/goflow.git
cd goflow

# Start everything
docker compose up -d --build

# Run migrations
make migrate

# Done — all services running
```

| Service | URL |
|---|---|
| GoFlow API | http://localhost:8080 |
| Swagger UI | http://localhost:8080/swagger/index.html |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (admin/admin) |

---

## Local Development

```bash
# Start infrastructure only (postgres, redis, prometheus, grafana)
make docker/up

# Run migrations
make migrate

# Run server locally with hot reload
make run
```

---

## Environment Variables

Create a `.env` file for local development:

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

In Docker, config is passed via environment variables in `docker-compose.yml` — no `.env` file needed.

---

## Make Commands

```bash
make help           # list all commands
make run            # run server locally
make build          # compile static binary → bin/goflow
make migrate        # run all SQL migrations
make migrate/reset  # wipe and re-run migrations
make db/connect     # open psql session
make docker/up      # start infrastructure containers
make docker/down    # stop all containers
make docker/build   # build Docker image
make docker/logs    # tail all container logs
make lint           # run golangci-lint
make test           # run all tests
make vet            # run go vet
make tidy           # tidy go modules
make setup          # install dev tools
```

---

## API Reference

Full interactive docs at `http://localhost:8080/swagger/index.html`.

### Health

```
GET /health
```
```json
{ "status": "ok", "service": "goflow" }
```

### Metrics

```
GET /metrics
```
Prometheus-format metrics. Scraped by Prometheus every 5 seconds.

---

### Tasks

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/tasks` | Enqueue a task |
| `GET` | `/api/v1/tasks` | List all tasks |
| `GET` | `/api/v1/tasks/:id` | Get task by ID |
| `GET` | `/api/v1/tasks/dead?queue=default` | List dead tasks |
| `POST` | `/api/v1/tasks/dead/:id/retry` | Retry a dead task |
| `DELETE` | `/api/v1/tasks/dead/:id` | Delete a dead task |

**Create task body:**
```json
{
  "type": "email:send",
  "payload": { "to": "user@example.com", "subject": "Hello" },
  "priority": 1,
  "max_retries": 3,
  "queue": "default",
  "delay_seconds": 0
}
```

Queues: `critical` (weight 6), `default` (weight 3), `low` (weight 1).

---

### Workflows

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/workflows` | Start a workflow |
| `GET` | `/api/v1/workflows` | List all workflows |
| `GET` | `/api/v1/workflows/:id` | Get workflow + step details |

**Start workflow body:**
```json
{
  "name": "order:process",
  "input": {
    "order_id": "order_001",
    "amount": 99.99
  }
}
```

Set `amount > 1000` to trigger payment failure and test compensation.

Workflow statuses: `pending` `running` `completed` `failed` `compensating` `compensated`
Step statuses: `pending` `running` `completed` `failed` `compensating` `compensated` `skipped`

---

## Built-in Task Types

| Type | Description |
|---|---|
| `email:send` | Sends an email |
| `report:generate` | Generates a report |
| `task:fail` | Always fails (dead-letter testing) |

---

## Built-in Workflow: `order:process`

```
Step 1: inventory:reserve  → on failure: inventory:release
Step 2: payment:charge     → on failure: payment:refund
Step 3: notification:send  → no compensation
```

If step 2 fails, compensation runs in reverse: `payment:refund` → `inventory:release`.

---

## Observability

### Prometheus Metrics

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

### Grafana Dashboard

1. Open http://localhost:3000 → login `admin/admin`
2. Add data source → Prometheus → URL: `http://prometheus:9090`
3. Create panels with these queries:

```promql
# Workflow outcomes
goflow_workflows_completed_total

# HTTP p95 latency
histogram_quantile(0.95, rate(goflow_http_request_duration_seconds_bucket[1m]))

# Task failure rate
rate(goflow_tasks_processed_total{status="failure"}[5m])
```

### Structured Logging

All logs are structured JSON via Zap:

```json
{"level":"INFO","ts":"...","msg":"workflow completed","workflow_id":"...","name":"order:process","duration":"18ms"}
{"level":"ERROR","ts":"...","msg":"step failed","workflow_id":"...","step":"Charge Payment","error":"payment declined"}
```

`APP_ENV=development` → colored console output.
`APP_ENV=production` → JSON only.

---

## Project Structure

```
goflow/
├── cmd/server/main.go          # Entry point
├── docs/                       # Swagger generated docs
├── internal/
│   ├── api/                    # HTTP handlers + middleware
│   │   ├── health.go
│   │   ├── task.go
│   │   ├── workflow.go
│   │   └── middleware.go
│   ├── service/
│   │   ├── task.go
│   │   ├── task_test.go
│   │   └── workflow.go
│   ├── repository/
│   │   ├── task.go
│   │   └── workflow.go
│   ├── worker/
│   │   ├── server.go
│   │   └── handlers.go
│   └── workflow/
│       ├── engine.go
│       ├── engine_test.go
│       └── handlers.go
├── pkg/
│   ├── config/config.go
│   ├── database/postgres.go
│   ├── queue/client.go
│   ├── logger/logger.go
│   └── metrics/metrics.go
├── migrations/
│   ├── 001_create_tasks.sql
│   └── 002_create_workflows.sql
├── observability/
│   └── prometheus.yml
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── .golangci.yml
├── .dockerignore
├── .gitignore
├── .env
└── go.mod
```

---

## Docker

The app is built with a multi-stage Dockerfile:

1. **Builder stage** (`golang:1.25-alpine`) — downloads dependencies, compiles a static binary
2. **Final stage** (`alpine:3.19`) — copies binary only, ~15MB image

```bash
docker build -t goflow:latest .   # build image
docker compose up -d --build      # build + start everything
docker compose down               # stop
docker compose down -v            # stop + wipe volumes
```

Container startup order: `postgres` (healthcheck) → `redis` (healthcheck) → `app` → `prometheus` → `grafana`.