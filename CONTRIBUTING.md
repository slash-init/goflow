# Contributing

## Setup

1. Install Go 1.21+
2. Install Docker & Docker Compose
3. Run `make setup` to install dev tools
4. Run `make docker/up` to start infrastructure
5. Run `make migrate` to set up the database
6. Run `make run` to start the server

## Before submitting

- `make lint` — must pass clean
- `make test` — must pass
- `make vet` — must pass
EOF
