# ==================================================================================== #
# BUILD STAGE
# ==================================================================================== #
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy mod files first — separate layer so it caches when only code changes
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/goflow \
    ./cmd/server/main.go

# ==================================================================================== #
# FINAL STAGE
# ==================================================================================== #
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bin/goflow .

RUN addgroup -S goflow && adduser -S goflow -G goflow
USER goflow

EXPOSE 8080

ENTRYPOINT ["./goflow"]