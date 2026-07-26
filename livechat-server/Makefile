.PHONY: dev dev-down migrate-up migrate-down proto loadtest-proto build test cleanup-sync-events

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

# 压测客户端的 Python 绑定；协议变更后必须重跑，否则压测会静默失效
loadtest-proto:
	cd ../load_test && ./gen_proto.sh

# ── Build ────────────────────────────────────────────
build:
	go build ./cmd/message-service
	go build ./cmd/gateway
	go build ./cmd/outbox-consumer
	go build ./cmd/sync-retention

# ── Test ─────────────────────────────────────────────
test:
	go test ./... -count=1 -timeout=60s -p 1

test-integration:
	go test ./... -count=1 -timeout=120s -tags=integration

# ── Run ──────────────────────────────────────────────
run-message-service:
	go run ./cmd/message-service

run-gateway:
	go run ./cmd/gateway

run-outbox-consumer:
	go run ./cmd/outbox-consumer

cleanup-sync-events:
	go run ./cmd/sync-retention
