# ==================================================================================== #
# VARIABLES
# ==================================================================================== #

APP_NAME        = goflow
MAIN_PATH       = ./cmd/server
BINARY_PATH     = ./bin/$(APP_NAME)
MIGRATIONS_DIR  = ./migrations
DB_DSN          = "postgres://goflow:goflow_secret@localhost:5432/goflow_db?sslmode=disable"

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## run: run the application
.PHONY: run
run:
	go run $(MAIN_PATH)/main.go

## build: build the application binary
.PHONY: build
build:
	@echo "Building..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_PATH) $(MAIN_PATH)/main.go
	@echo "Binary built at $(BINARY_PATH)"

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf bin/

# ==================================================================================== #
# DATABASE
# ==================================================================================== #

## migrate: run all migrations
.PHONY: migrate
migrate:
	@echo "Running migrations..."
	@for f in $(MIGRATIONS_DIR)/*.sql; do \
		echo "  Applying $$f"; \
		docker exec -i goflow_postgres psql -U goflow -d goflow_db < $$f; \
	done
	@echo "Migrations complete."

## migrate/reset: drop and recreate the database
.PHONY: migrate/reset
migrate/reset:
	@echo "Resetting database..."
	docker exec -i goflow_postgres psql -U goflow -c "DROP DATABASE IF EXISTS goflow_db;"
	docker exec -i goflow_postgres psql -U goflow -c "CREATE DATABASE goflow_db;"
	$(MAKE) migrate

## db/connect: open a psql session
.PHONY: db/connect
db/connect:
	docker exec -it goflow_postgres psql -U goflow -d goflow_db

# ==================================================================================== #
# DOCKER
# ==================================================================================== #

## docker/up: start all infrastructure containers
.PHONY: docker/up
docker/up:
	docker compose up -d

## docker/down: stop all containers
.PHONY: docker/down
docker/down:
	docker compose down

## docker/reset: stop containers and wipe volumes
.PHONY: docker/reset
docker/reset:
	docker compose down -v

## docker/build: build the Go app Docker image
.PHONY: docker/build
docker/build:
	docker build -t $(APP_NAME):latest .

## docker/logs: tail all container logs
.PHONY: docker/logs
docker/logs:
	docker compose logs -f

# ==================================================================================== #
# QUALITY
# ==================================================================================== #

## lint: run linter
.PHONY: lint
lint:
	golangci-lint run ./...

## test: run all tests
.PHONY: test
test:
	go test -v -race ./...

## test/cover: run tests with coverage report
.PHONY: test/cover
test/cover:
	go test -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## vet: run go vet
.PHONY: vet
vet:
	go vet ./...

# ==================================================================================== #
# SETUP
# ==================================================================================== #

## setup: install all dev tools
.PHONY: setup
setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

## tidy: tidy go modules
.PHONY: tidy
tidy:
	go mod tidy
	go mod verify