---
id: "0002"
title: "项目脚手架 + DB 迁移 + Mock Auth"
status: in_progress
labels: ["in-progress"]
parent: "0001"
blocked_by: []
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

搭建 Go 服务端项目骨架：模块目录、依赖管理、数据库迁移、以及 mock 认证端点。

端到端行为：开发者 clone 仓库后执行 `make dev`（启动 PG/Redis）→ `make migrate-up`（建表）→ `curl POST /v1/auth/register` 拿到 JWT → 用该 JWT 访问其他 API。

具体交付：

- Go module 初始化（`livechat-server`），`cmd/` 下三个入口（message-service、gateway、outbox-consumer）各自能编译并打印启动日志。
- `internal/infra/` 提供 PostgreSQL 连接池和 Redis 客户端封装，从 YAML 配置文件读取连接参数。
- `migrations/` 目录包含 6 条 DDL（users、devices、conversations、conversation_members、messages、outbox_events、sync_events、sync_cursors、conversation_summaries），使用 `golang-migrate` 作为迁移工具。
- `make dev`（docker-compose up -d pg + redis）、`make migrate-up`、`make migrate-down`。
- `POST /v1/auth/register` 接受 `{phone_e164, verification_code, device_id, platform}`，mock 验证码（接受任意 6 位数字），返回 `{access_token, refresh_token, expires_in}`。
- `POST /v1/auth/login` 同上。
- JWT 使用 HS256，claims 含 `user_id`、`device_id`、`exp`，`internal/auth/` 提供 `Sign()` 和 `Verify()` 函数。Refresh token 为随机 opaque token 的 SHA-256 hash，存于 `devices.refresh_token_hash`。
- HTTP 框架使用标准库 `net/http` 或 `chi`，统一 JSON 响应格式 `{data, error}` 和结构化错误日志（`slog`）。

## Acceptance criteria

- [ ] `make dev` 成功启动 PostgreSQL 16 和 Redis 7 容器
- [ ] `make migrate-up` 在 PostgreSQL 中创建全部 9 张表（8 张数据表 + outbox_events），字段类型与 PRD 一致
- [ ] `go build ./cmd/message-service` 和 `go build ./cmd/gateway` 和 `go build ./cmd/outbox-consumer` 编译成功
- [ ] `POST /v1/auth/register` 使用新手机号返回 201 + 有效 JWT
- [ ] `POST /v1/auth/register` 使用已注册手机号返回 409 conflict
- [ ] `POST /v1/auth/login` 使用已注册手机号 + 任意 6 位验证码返回 200 + 有效 JWT
- [ ] 使用 login 返回的 JWT 访问任意受保护端点，鉴权中间件校验通过
- [ ] `GET /health` 返回 200 + PostgreSQL 和 Redis 连通状态

## Current implementation status

- 已实现：`livechat-server` 模块结构、三个服务入口、迁移工具、`POST /v1/auth/register`、`POST /v1/auth/login`、JWT 与 refresh token 基础逻辑、`GET /health`。
- 已验证：`go build ./cmd/message-service`、`go build ./cmd/gateway`、`go build ./cmd/outbox-consumer` 可通过；注册后可获得 JWT 并访问受保护端点；健康检查可返回 PostgreSQL/Redis 状态。
- 未完成：本票的验收标准仍未全部关闭，尤其是 `make dev` 依赖 Docker，而当前验证环境没有 `docker` 命令；重复注册返回 `409` 这一条也尚未作为本地验证结论记录。

## Blocked by

None - can start immediately.
