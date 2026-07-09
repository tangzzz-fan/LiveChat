# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LiveChat is a **learning-driven** WhatsApp-class IM system — iOS client (SwiftUI) + Go backend. The goal is not to ship a product but to **learn classic large-scale IM problems through hands-on implementation**: C100K connections, reliable messaging, offline sync, group fan-out, presence broadcast storms, E2EE.

## Repo Structure

```
LiveChat/
├── specs/                     # 15 architecture specs (SPEC-000 ~ SPEC-014)
├── proto/                     # .proto definitions (SPEC-001, shared Go/Swift)
├── server/                    # Go backend monorepo
│   ├── gateway/               #   WebSocket gateway (SPEC-002)
│   ├── msgsvc/                #   Message service (SPEC-003)
│   ├── api/                   #   REST API service
│   ├── pushworker/            #   APNs push worker (SPEC-008)
│   └── aisvc/                 #   AI streaming service (M4, SPEC-013)
├── ios/                       # SwiftUI App (SPEC-004)
├── loadtest/                  # Go loadtest tool (SPEC-005)
├── deploy/                    # docker-compose, Prometheus, Grafana
└── docs/
    ├── adr/                   # Architecture Decision Records
    ├── experiments/           # Runbook results for verification experiments
    └── statecharts/           # FSM state diagrams (SPEC-012)
```

## Milestone Philosophy

The project ships in 4 milestones. **M1 must hit a verifiable closed loop** before building further:

- **M1**: 1:1 chat, 50k connections, 1,000 msg/s, kill-9 zero-loss proof, iOS flight-mode experiment
- **M2**: Group chat, presence/typing/read receipts, APNs push
- **M3**: Media (MinIO), multi-device sync, E2EE (Signal Protocol)
- **M4**: Streaming rendering + FSM (012), AI bot + streaming pipeline (013), on-device AI (014)

Spec dependency graph (hard deps only):
```
001 → 002 → 003 → 004 → 005
              ├→ 006      009 ─┐
              ├→ 007          ├→ 011
              ├→ 008     010 ─┘

M4: 004 → 012 → 013, 014 (013/014 parallel after 012)
```

## Core Architectural Principles

These are the three foundations every spec builds on:

1. **Gateway is stateless** — connection state lives in Redis route table. Kill any gateway, client reconnects to another, no data loss.
2. **Connection ≠ business logic** — Gateway handles pipes (websocket, heartbeat, codec); Message Service handles semantics (ordering, dedup, persistence, fan-out).
3. **Client is local-first** — UI reads from local GRDB only. Network syncs the local DB toward server state. Messages render instantly; network unreliability is absorbed by the outbox + inbox model.

## Spec Conventions

- Every spec follows "Why → Challenges → Solutions → Scope → Acceptance Criteria → Test Plan"
- Acceptance criteria are **quantifiable and experimentable** (not "code is done")
- `docs/experiments/` records manual runbook results for each acceptance criterion
- Cross-spec consistency rules: when two specs define the same mechanism (e.g., inbox retention policy), one is the **canonical source** and the other **references it** to prevent drift

## Key Design Decisions (from specs)

- **Dual-sequence model**: `conv_seq` (per-conversation ordering) ≠ `inbox_seq` (per-user sync cursor). Confusing these is the #1 self-built IM design bug.
- **Push-pull hybrid**: Push is fast (best-effort), pull is complete (inbox sync). Push can drop — inbox is the source of truth.
- **Write-fanout for small groups, read-fanout documented for large**: WhatsApp/Discord hybrid strategy, threshold documented in SPEC-006.
- **Data classification spectrum**: From "never lose" (messages, via inbox) to "should lose" (typing indicators, via pure memory forward). SPEC-007 defines this; SPEC-013 reuses it for StreamChunk.
- **E2EE is last**: Encryption freezes your architecture — all content-dependent features (search, moderation, server dedup) die. Build everything in plaintext first, then lock it.
- **Bot sessions are explicitly unencrypted**: Server-side AI needs plaintext. On-device AI (014) is the only intelligence path for encrypted conversations.

## Development Commands

### Go (server/)

```bash
# Build all services
cd server && go build ./...

# Run tests
go test ./...                          # all tests
go test -run TestSnowflake ./msgsvc/  # single test

# Proto generation
cd proto && buf generate               # generate Go + Swift code
buf lint && buf breaking               # CI checks
```

### iOS

```bash
cd ios
xcodebuild -project LiveChat.xcodeproj -scheme LiveChat test   # run tests
```

### Loadtest

```bash
cd loadtest
make loadtest-smoke                                               # 1k connections, CI mode
go run ./cmd/loadtest --scenario scenarios/idle_50k.yaml --mode perf
go run ./cmd/loadtest --scenario scenarios/chat_1kmsg.yaml --mode correctness
```

### Docker Compose (full stack)

```bash
cd deploy
docker-compose up -d                    # 2×gateway + msgsvc + api + PG + Redis + prometheus + grafana
docker-compose logs -f gateway         # tail specific service
docker-compose down -v                 # tear down with volume cleanup
```

## Experiment Runbooks

Verification experiments from specs are manual runbooks. Results go to `docs/experiments/`. Key experiments:

- `idle_50k` — 50k connections stable for 30min
- `kill-9` — random gateway/msgsvc kills with zero-loss proof
- `flight-mode` — iOS offline message queue → online sync
- `reconnect-storm` — 5w simultaneous reconnect, prove rate limiter works
- `server-blind-test` — dump all DB/Redis/memory, grep for plaintext → zero hits

## Stack

| Layer | Tech |
|-------|------|
| iOS | SwiftUI, GRDB (SQLite), Swift Concurrency (`async/await`, actors), iOS 17+ |
| Backend | Go, goroutine-per-connection, gRPC, Protobuf (buf) |
| Storage | PostgreSQL (partitioned), Redis |
| Object Store | MinIO (S3-compatible, M3) |
| E2EE | libsignal (Signal Protocol, M3) |
| On-device AI | Apple Foundation Models (iOS 26+, M4), Core ML fallback |
| Observability | Prometheus + Grafana |
| Infra | docker-compose |
