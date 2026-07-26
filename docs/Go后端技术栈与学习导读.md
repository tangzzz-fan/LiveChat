# Go 后端技术栈与学习导读

> **读者**：Go 基础偏弱、想对照本仓已实现代码做复盘学习。  
> **本文解决什么**：本仓用了哪些技术、对应哪段代码、该先读哪份文档。  
> **本文不替代**：Specs（设计源）、工程问题库（为什么这么设计）、ADR（不可逆决策）。

## 0. 先回答：实现有没有文档佐证？

**有，但分层不齐。**

| 层级 | 有没有 | 代表文档 | 适合学什么 |
|------|--------|----------|------------|
| 产品/规格 | 有 | [`Specs/`](../Specs/) | 「系统该做什么」 |
| 架构总览 | 有 | [`架构设计总览.md`](./架构设计总览.md) | 拓扑、主链路、痛点索引 |
| 实现决策 | 有 | [`livechat-server/docs/technical-decisions.md`](../livechat-server/docs/technical-decisions.md) | Outbox、seq、Gateway 路由等**为什么** |
| Phase 分册 | 有（部分过时） | Phase1/Phase2 架构说明、阶段性实现介绍 | 分阶段模块边界；**状态描述可能落后于 INDEX** |
| 工程问题 | 有 | [`engineering-problems/`](./engineering-problems/) | IM 痛点 → 本仓方案 → 代码锚点 |
| ADR | 有少量 | [`adr/`](./adr/) | JWT、追踪、混沌等单一决策 |
| API | 有 Markdown | [`API参考.md`](./API参考.md) | 客户端怎么调；**无 OpenAPI/Swagger** |
| 操作 | 有 | [`build-and-test.md`](../livechat-server/docs/build-and-test.md)、`Makefile`、`scripts/` | 怎么起停、测、演练 |
| **验证方法论** | **有** | [`验证链路设计方法论.md`](./验证链路设计方法论.md) | 压测/混沌原则、场景设计、对外叙事 |
| **Go 语言/库用法导读** | **有** | 本文 | 标准库 + 依赖如何在本仓落地 |

结论：业务正确性与 IM 专题文档较全；**缺的是「弱 Go 读者」从语言/框架走进代码的梯子**——由本文补上。

---

## 1. 推荐阅读顺序（约 1–2 天可过一遍）

1. 本文 §2–§4（技术栈地图 + Go 语法在本仓的用法）  
2. [`架构设计总览.md`](./架构设计总览.md) §2（拓扑与模块表）  
3. [`technical-decisions.md`](../livechat-server/docs/technical-decisions.md) §1–§5（发送与投递）  
4. 工程问题选读：`01` Outbox → `02` seq → `06` 生命周期 → `03` 重连  
5. 对照代码：`cmd/message-service/main.go` → `internal/api/router.go` → `internal/messages/service.go`  
6. Gateway：`cmd/gateway/main.go` → `internal/gateway/manager.go`  
7. Consumer：`cmd/outbox-consumer/main.go` → `internal/outbox/consumer.go` → `internal/fanout/service.go`  
8. **包级源码导读（推荐穿插）**：[`code-walkthrough/01-messages.md`](./code-walkthrough/01-messages.md) → [`02-outbox.md`](./code-walkthrough/02-outbox.md)  
9. 动手：`make test`、跑一条 `scripts` smoke / `ws_probe`（见 build-and-test）

---

## 2. 运行时与依赖技术栈（对照 `go.mod`）

### 2.1 基础设施（进程外）

| 技术 | 本仓角色 | 入口 |
|------|----------|------|
| **PostgreSQL 16+** | 消息、Outbox、sync、群、附件等真相库；会话 SEQUENCE | `internal/infra/db.go`、`migrations/` |
| **Redis 7+** | 在线路由、OTP、热点群窗口、部分限流/缓存 | `internal/infra/redis.go`、`internal/cache/` |
| **本机对象目录** | 媒体分片/缩略图（非云对象存储） | `internal/media/` |

### 2.2 Go 模块与直接依赖

模块：`github.com/tangzzz-fan/LiveChat/livechat-server`（见 `livechat-server/go.mod`）。

| 依赖 | 用途 | 本仓主要位置 |
|------|------|----------------|
| **标准库 `net/http`** | HTTP API（Go 1.22+ 增强路由） | `internal/api/router.go` |
| **标准库 `database/sql`** | SQL 访问（驱动见下） | 各 `internal/*/service.go` |
| **`github.com/lib/pq`** | PostgreSQL 驱动 | `infra/db.go` |
| **`github.com/redis/go-redis/v9`** | Redis 客户端 | `infra/redis.go`、gateway 路由、auth OTP |
| **`github.com/gorilla/websocket`** | WebSocket 升级与读写 | `internal/gateway/` |
| **`github.com/golang-jwt/jwt/v5`** | Access/Refresh JWT | `internal/auth/` |
| **`google.golang.org/protobuf`** | WS 帧 / 部分 RPC 消息编解码 | `proto/`、`gateway/frame.go` |
| **`google.golang.org/grpc`** | 进程间：Fanout→Gateway 投递、ACK 等 | `gateway/delivery.go`、`receipts/` |
| **`golang.org/x/image`** | 缩略图生成 | `internal/media/` |

说明：本仓**没有**引入 Gin/Echo/Fiber、GORM/Ent、Kafka、OpenTelemetry SDK（追踪偏手写，见 ADR 0003）。刻意保持「标准库 + 少量库」，便于学习。

### 2.3 三个长驻进程

| 进程 | 端口（默认） | 技术要点 |
|------|--------------|----------|
| `message-service` | HTTP `:8080`，另有 gRPC | `http.ServeMux` + JWT 中间件 + 业务 service |
| `gateway` | WS `:8081` | gorilla websocket + Redis 路由 + gRPC 收投递 |
| `outbox-consumer` | metrics `:8082` | DB 轮询 Outbox + worker pool + Fanout/Push |

启动：`livechat-server` 下 `make run-*`，或仓库根 `scripts/setup.sh --start` / `scripts/stop.sh`。细节见 [`build-and-test.md`](../livechat-server/docs/build-and-test.md)。

---

## 3. 本仓用到的 Go 语言点（对照学习）

不必先通读《Go 语言圣经》；按下面「在本仓出现的形态」学更快。

### 3.1 包与可见性

- `cmd/...`：可执行入口，`package main` + `func main()`  
- `internal/...`：**仅本模块可导入**（Go 编译器强制），放业务与基础设施  
- 导出：首字母大写（如 `NewRouter`）；小写仅包内（如 `handleSendMessage`）

### 3.2 `context.Context`

几乎所有 DB/Redis/RPC 调用第一个参数是 `ctx`。用于超时、取消、请求级值（如 user id）。  
读：`api` 中间件把 JWT claims 塞进 context，handler 再取出。

### 3.3 错误处理

惯用 `if err != nil { return ..., err }`，较少 panic。  
业务错误有时变成 HTTP 状态码（400/401/429/500）。学习时跟一条 send 路径看 err 如何冒泡到 `writeJSON`。

### 3.4 接口（interface）与依赖倒置

例如 Push 的 `APNsClient`、媒体的 `ObjectStore`、Fanout 的 `PushNotifier`：  
**用小接口描述依赖**，便于 mock / 混沌注入（如 `PUSH_INJECT_DELAY_MS`）。  
这是 Go 常见风格，不是 Java 式大接口。

### 3.5 并发

| 模式 | 出现位置 | 在学什么 |
|------|----------|----------|
| `go func()` | thumbnail worker、cleanup、consumer workers | 启动后台任务 |
| `chan` / worker pool | `outbox/consumer.go` | 有界并发消费 |
| 内存 map + mutex | Gateway session 表 | 连接状态 |
| **不用**共享内存传业务消息 | Outbox + DB | 「进程间靠持久化事件」 |

### 3.6 数据库访问习惯

- `database/sql`：`DB.BeginTx` → `Exec`/`Query` → `Commit`/`Rollback`  
- 幂等：`ON CONFLICT DO NOTHING`（见 messages）  
- 消费者：`FOR UPDATE SKIP LOCKED`（见 outbox）  
- 迁移：自研 `cmd/migrate` + `migrations/*.sql`（非 golang-migrate 库亦可对照 SQL）

### 3.7 HTTP 路由（Go 1.22+）

```go
mux.Handle("POST /v1/messages/send", authMw.Wrap(...))
mux.Handle("GET /v1/groups/{gid}/members", ...)
```

方法 + 路径模式注册在 `ServeMux` 上，**无第三方 Web 框架**。中间件是「包一层 `http.Handler`」。

### 3.8 WebSocket 与 Protobuf

1. HTTP Upgrade → `*websocket.Conn`  
2. 自定义帧：`gateway.UnmarshalFrame` + protobuf payload  
3. 心跳刷新 Redis TTL  

学习顺序：先读 `docs/API参考.md` §6 opcode 表，再读 `gateway/frame.go` / `manager.go`。

### 3.9 测试

- `*_test.go`：`testing` 包 + `go test`  
- 集成测试会打真 Postgres/Redis（见 `Makefile` 的 `-p 1` 串行，避免共享状态打架）  
- 复盘时挑一个 `router_integration_test.go` 里的用例当「可执行文档」

---

## 4. `internal/` 包地图（实现佐证索引）

| 包 | 职责一句话 | 建议先读的文件 | 相关文档 |
|----|------------|----------------|----------|
| `domain` | 共享类型/常量 | `types.go` | Spec 02 |
| `api` | HTTP 路由与中间件 | `router.go` | API参考 |
| `auth` | OTP、JWT、`session_version` | `auth.go` | 工程问题 08、09；ADR 0002 |
| `messages` | 幂等发送 + Outbox 同事务 | `service.go` | technical-decisions §1；工程问题 01 |
| `outbox` | 轮询、lease、重试、死信 | `consumer.go` | technical-decisions §3 |
| `fanout` | 成员解析、分级扇出、热点 | `service.go` | 工程问题 10 |
| `gateway` | WS、路由、限流、投递、ACK | `manager.go`, `ratelimit.go` | Spec 05；工程问题 03 |
| `sync` | sync_events、游标 | `service.go` | Spec 06；工程问题 05 |
| `receipts` | 送达/已读 | `service.go` | 工程问题 06 |
| `conversations` | 会话列表投影 | `summary.go` | Spec 06 |
| `group` | 群与成员 | 群相关 service | Spec 07 |
| `media` | 上传/缩略图/HMAC 下载 | `service.go` | 工程问题 13 |
| `push` | 离线推送编排 + mock APNs | `orchestrator.go` | 工程问题 11；chaos 04 |
| `metrics` | Prometheus 文本指标 | `metrics.go` | Spec 12 / 0018 |
| `cache` | 通用缓存封装（热路径引用仍有限） | `redis.go` | 工程问题 12 |
| `infra` | DB/Redis 连接 | `db.go`, `redis.go` | build-and-test |
| `traceutil` | 手写 trace id | `trace.go` | ADR 0003；工程问题 14 |

---

## 5. 已有文档怎么选（按学习目标）

### 想搞懂「消息为什么不丢 / 顺序 / 多端」

→ `engineering-problems/01,02,04,05,06` + `technical-decisions.md`

### 想搞懂「长连接与高负载」

→ [`验证链路设计方法论.md`](./验证链路设计方法论.md)（先懂「为什么这样测」）  
→ `架构设计总览` 痛点表 + `engineering-problems/03,10,15` + `load-practice/` + `docs/chaos/`  
→ 证据：`load_test/baselines/local-measured-baseline.md`

### 想调 HTTP / WS

→ `API参考.md`；WS 真帧用 `livechat-server/scripts/ws_probe.go`

### 想看「当初为什么选 A 不选 B」

→ `docs/adr/` + `technical-decisions.md`

### 想知道「现在做到哪一阶段」

→ **以 [`issues/INDEX.md`](../issues/INDEX.md) 为准**。  
注意：`阶段性实现介绍.md` 仍可能写着「Phase 2 未开始」——那是历史交接点文稿，**状态以 INDEX 为准**（Phase 1–3 学习闭环已完成，后续为 iOS / 高负载验证票）。

---

## 6. 仍可补强的文档缺口（未在本轮写满）

| 缺口 | 说明 | 建议后续 |
|------|------|----------|
| OpenAPI / Swagger | 仅有 Markdown API | 可视化试调票 |
| `阶段性实现介绍` 状态刷新 | 与 INDEX 不一致 | 改开头「以 INDEX 为准」或重写阶段结论 |
| 逐包「源码导读」 | 已开始：[`code-walkthrough/`](./code-walkthrough/)（messages、outbox） | 可续写 gateway / fanout / api |
| gRPC 服务清单专页 | 散落在 Phase 文档与代码 | 可从 `proto/` 生成一页表 |

---

## 7. 最小动手实验（巩固）

```bash
# 1) 依赖与迁移（本机 PG + Redis 已起）
cd livechat-server && make migrate-up

# 2) 三个终端
make run-message-service
make run-gateway
make run-outbox-consumer

# 3) 健康与指标
curl -s http://localhost:8080/health
curl -s http://localhost:8082/metrics | head

# 4) 读测试当文档
make test
```

然后打开 `internal/messages/service.go` 的 `Send`，对照 `technical-decisions.md` §1，用自己的话写出：事务里写了哪两张表、幂等键是什么、返回的 `is_duplicate` 何时为 true。

---

## 8. 导航回链

- 总览：[`架构设计总览.md`](./架构设计总览.md)  
- 源码导读：[`code-walkthrough/`](./code-walkthrough/)  
- 服务端操作：[`livechat-server/README.md`](../livechat-server/README.md)  
- 问题库：[`engineering-problems/INDEX.md`](./engineering-problems/INDEX.md)  
- 高负载计划：[`高负载IM验证计划.md`](./高负载IM验证计划.md)  
