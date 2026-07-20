.PHONY: dev dev-down migrate-up migrate-down proto build test

# ── Docker ───────────────────────────────────────────
dev:
	docker compose up -d
	@echo "PostgreSQL: localhost:5432, Redis: localhost:6379"

dev-down:
	docker compose down

# ── Migrations ───────────────────────────────────────
migrate-up:
	go run cmd/migrate/main.go up

migrate-down:
	go run cmd/migrate/main.go down

# ── Proto ────────────────────────────────────────────
proto:
	buf generate proto

# ── Build ────────────────────────────────────────────
build:
	go build ./cmd/message-service
	go build ./cmd/gateway
	go build ./cmd/outbox-consumer

# ── Test ─────────────────────────────────────────────
test:
	go test ./... -count=1 -timeout=60s

test-integration:
	go test ./... -count=1 -timeout=120s -tags=integration

# ── Run ──────────────────────────────────────────────
run-message-service:
	go run ./cmd/message-service

run-gateway:
	go run ./cmd/gateway

run-outbox-consumer:
	go run ./cmd/outbox-consumer
