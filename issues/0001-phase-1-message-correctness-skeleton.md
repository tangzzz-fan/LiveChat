---
id: "0001"
title: "阶段一：消息正确性骨架 — Message Service + Gateway 落地实现"
status: complete
labels: ["done"]
created_at: 2026-07-20
---

# PRD: 阶段一 — 消息正确性骨架（Message Service + Gateway 落地实现）

## Problem Statement

当前 LiveChat 项目已完成全部 14 份规格文档（`Specs/00–13`），领域模型、消息生命周期、Outbox 模式、长连接网关协议、离线同步策略等核心设计均已在 spec 层面收敛。

**问题在于：这些设计还没有可运行的代码。** 对于每个参与此项目的人来说，规格文档只能说明"应该怎么做"，但无法验证：
- Outbox 模式在真实 DB 事务中是否能保证不丢消息？
- 客户端发送→服务端持久化→投递→已读回执这条链路跑起来后，状态转换是否与 Spec 02 的状态机一致？
- 网关在弱网和重连风暴下能否按 Spec 05 的退避策略正确行为？
- 多端已读状态是否真的收敛到 `MAX(last_read_seq)`？

没有可运行的消息正确性骨架，后续群聊、媒体、推送等 P0 扩展就没有可信的基线，每次改动都得靠人眼对照 spec，不可持续。

## Current implementation status

- 已验证：`make build` 可通过，`make test` 可执行，`./scripts/phase1-smoke.sh` 可验证 direct conversation 下的注册、消息发送、幂等重发、会话摘要、同步事件和消息补拉。
- 已新增验证：Gateway 已有 `MESSAGE_DELIVERY` 自动化测试，证明连接成功的 Device 可以接收实时投递帧。
- 已新增验证：`ACK(read)` 已有转发与业务投影测试，证明 Gateway 能把 ACK 转发到 Message Service，且 `read_receipt` 会把已读状态投影为 `message_read` / `conversation_updated` sync events。
- 已新增验证：`./scripts/phase1-realtime-delivery.sh` 已固定验证 `curl send -> Outbox -> Fanout -> gRPC Gateway -> WebSocket MESSAGE_DELIVERY`；`./scripts/phase1-read-receipt.sh` 已固定验证 `WebSocket ACK(read) -> gRPC Message Service -> Outbox Consumer -> sync_events`、B 的 `unread_count=0`、A 的 `message_read`、B 其他设备的 `conversation_updated` 和 `MAX(last_read_seq)` 收敛示例。
- 已新增验证：`TestProcessEventRetryThenRecoveryMarksDoneWithoutLoss` 已固定证明“下游短暂不可用 -> Outbox 重试 -> 恢复后成功处理 -> 不进入 failed 状态”，满足里程碑标准 5。
- 已新增验证：`ReconnectBackoffWindowGrowthAndCap`、`ReconnectBackoffDelayStaysInsideWindow`、`FastReconnectEligible` 已固定证明 `Spec 05 §6.1` 的标准退避窗口与 30s 封顶；`TestGatewayWatchdogClosesStaleSessionWithReconnectHint` 与旧连接替换测试共同证明服务端会发出 `should_reconnect=true` 信号，满足里程碑标准 6。
- 已实现：`Message Service`、`Gateway`、`Outbox Consumer`、`Sync Service` 的基础代码骨架已经存在，并可在本机 PostgreSQL/Redis 环境下运行。
- 结论：6 条里程碑验证标准均已形成固定 runbook 或自动化测试证据，父票可以关闭。

## Solution

**实现阶段一的全部服务端核心服务**，交付一条可独立验证的消息发送→投递→已读闭环链路：

1. **Message Service**（Go）：消息接收 HTTP API、幂等写入、Outbox 事件生成、ConversationSummary 维护
2. **Outbox Consumer**（Go，与 Message Service 同进程或独立部署）：从 Outbox 表拉取事件，驱动投递、同步和推送编排
3. **Gateway**（Go）：WebSocket 长连接接入、握手鉴权、心跳管理、用户路由注册、协议帧转发
4. **Sync Service**（作为 Message Service 的子模块）：sync_events 写入、增量同步 API、游标管理

不实现在这一阶段的：
- Gateway 不实现连接迁移（WiFi ↔ 蜂窝），作为 P1 学习扩展
- 不做端到端加密（消息以明文或业务层加密传输）

### Seams（测试接缝）

按从高到低的优先级列出可测试界面：

| 接缝 | 类型 | 验证目标 |
|------|------|----------|
| HTTP `POST /v1/messages/send` | 集成测试 | 幂等写入、Outbox 事件在同一事务中持久化 |
| WebSocket 帧协议（Protobuf opcode） | 集成测试 | 握手 → 心跳 → 消息投递 → ACK 全流程 |
| Outbox Consumer → Fanout/Sync/Push | 单元+集成测试 | 事件消费幂等、重试退避、死信转移 |
| `GET /v1/sync/events?cursor=N` | 集成测试 | 增量同步正确性、游标前进、has_more 分页 |
| `GET /v1/conversations/{cid}/messages?from_seq=N` | 集成测试 | 会话消息补拉、缺口补齐 |

这些接缝都是 Spec 04–06 中已定义的 API 合约，不需要发明新接缝。每个接缝都有明确的 request/response schema，可直接转化为测试 fixture。

## User Stories

### 消息发送核心链路

1. As a User, I want to send a text message to another User in a 1:1 conversation, so that I can communicate with my contacts.
2. As a User, I want to see my message status transition from "sending" → "accepted" → "delivered" → "read", so that I know the message reached the recipient.
3. As a User, I want my message to not be lost even if the network drops during sending, so that I don't have to re-type and re-send.
4. As a User, I want duplicate messages to be automatically detected and suppressed, so that retrying a send doesn't create duplicate messages visible to the recipient.
5. As a User, I want to be notified immediately when my message fails permanently (e.g., I'm not a member of the conversation), so that I can take corrective action.

### 实时投递与连接

6. As a User, I want to receive new messages in real time without manually refreshing, so that the conversation feels instant.
7. As a User, I want my connection to stay alive through periodic heartbeats, so that I don't get disconnected during idle periods.
8. As a User, I want to be able to reconnect quickly after a temporary network interruption, so that I don't miss messages.
9. As a User, I want reconnection to use exponential backoff so that I don't overwhelm the server during a widespread outage.

### 离线同步

10. As a User, I want to see all messages I missed while offline as soon as I reconnect, so that nothing is lost.
11. As a User, I want the sync to be incremental (only fetch what I missed), so that it's fast and data-efficient.
12. As a User, I want messages within a conversation to appear in the correct order, so that I can follow the conversation thread correctly.
13. As a User, I want to detect when my local message store has gaps and automatically fill them, so that my conversation view is always complete.

### 多端一致性

14. As a User with multiple devices, I want marking a conversation as read on one device to sync to my other devices, so that I don't see the same unread badge everywhere.
15. As a User with multiple devices, I want the unread count to be consistent across all my devices, so that I can trust what I see on any screen.
16. As a User, I want my conversation list (sorted by most recent message) to stay consistent across devices, so that I don't get confused about which conversation is active.

### 会话管理

17. As a User, I want to see my conversation list with the last message preview and unread count, so that I can quickly decide which conversation to open.
18. As a User, I want my conversation list to load fast (under 50ms), so that the app feels responsive.

### 运维与可观测性

19. As an Operator, I want to see message send latency (P50/P95/P99), so that I can detect performance regressions.
20. As an Operator, I want to be alerted when the Outbox consumer falls behind (pending events > threshold), so that I can intervene before users notice delays.
21. As an Operator, I want to be alerted when a Gateway node is overloaded (CPU > 85%), so that I can scale out before connections drop.
22. As an Operator, I want dead-letter events to trigger an alert, so that message loss is never silent.

## Implementation Decisions

### 技术栈选型

- **语言**：Go（一致于 Spec 05 §8 的推荐）。选择 Go 而非 Rust/Java 的原因是：goroutine 长连接模型自然匹配 Gateway 的 C10K→C1000K 需求；标准库 `net/http` 和 `database/sql` 成熟稳定；团队学习曲线合理。
- **数据库**：PostgreSQL 16+。Outbox 依赖同一事务内写入 messages + outbox_events，需要 ACID 保证。
- **缓存/路由存储**：Redis 7+。Gateway 使用 Redis 存储用户路由表（`gateway:user:{user_id}:{device_id}`），依赖 TTL 和心跳续期。
- **协议编码**：Protobuf（proto3），WebSocket 层使用 binary frames。
- **内部通信**：gRPC（Gateway ↔ Message Service）。
- **WebSocket 库**：`gorilla/websocket` 或 `nhooyr.io/websocket`。

### 模块划分

```
livechat-server/
├── cmd/
│   ├── message-service/    # Message Service 入口
│   ├── gateway/            # Gateway 入口
│   └── outbox-consumer/   # Outbox Consumer 入口
├── internal/
│   ├── domain/             # 共享领域类型（User, Message, Conversation, etc.）
│   ├── messages/           # 消息接收、幂等写入、seq 分配
│   ├── outbox/             # Outbox 表操作 + 消费者逻辑
│   ├── sync/               # sync_events 写入、增量同步 API、游标管理
│   ├── gateway/            # WebSocket acceptor、session manager、心跳、路由
│   ├── fanout/             # 投递编排（查路由 → gRPC 发给 Gateway → 写 sync_events）
│   ├── auth/               # JWT 签发与验证（mock 模式，不接真实短信）
│   └── infra/              # DB 连接池、Redis 客户端、gRPC server/client
├── proto/                  # Protobuf 定义
│   ├── ws_frame.proto      # WebSocket 帧协议
│   ├── message.proto       # Message Service gRPC
│   └── sync.proto          # Sync Service gRPC
├── migrations/             # PostgreSQL DDL
│   ├── 001_users.up.sql
│   ├── 002_messages.up.sql
│   ├── 003_outbox_events.up.sql
│   ├── 004_sync_events.up.sql
│   ├── 005_sync_cursors.up.sql
│   └── 006_conversation_summaries.up.sql
└── configs/                # 配置模板
    ├── message-service.yaml
    └── gateway.yaml
```

### 数据库 Schema

迁移脚本应直接实现 Spec 04 §4.2 的 `outbox_events` DDL、Spec 06 §3.2 的 `sync_cursors` DDL、Spec 06 §3.3 的 `sync_events` DDL，以及以下核心表：

- **`users`**：`id BIGSERIAL PK`, `phone_e164 VARCHAR(20) UNIQUE`, `display_name VARCHAR(100)`, `created_at TIMESTAMPTZ`
- **`devices`**：`id VARCHAR(64) PK`, `user_id BIGINT FK`, `platform VARCHAR(20)`, `push_token VARCHAR(256)`, `last_seen_at TIMESTAMPTZ`
- **`conversations`**：`id VARCHAR(64) PK`, `type VARCHAR(10)` ('direct'|'group'), `created_at TIMESTAMPTZ`
- **`conversation_members`**：`conversation_id FK`, `user_id FK`, `joined_at TIMESTAMPTZ`, PK(`conversation_id`, `user_id`)
- **`messages`**：`server_message_id VARCHAR(64) PK`, `conversation_id FK`, `conversation_seq BIGINT`, `sender_user_id FK`, `sender_device_id FK`, `client_message_id VARCHAR(128)`, `message_type VARCHAR(20)`, `content JSONB`, `server_received_at TIMESTAMPTZ`。UNIQUE 约束 `(sender_user_id, client_message_id)` 用于幂等。
- **`conversation_summaries`**：`user_id FK`, `conversation_id FK`, `last_message_preview TEXT`, `last_message_at TIMESTAMPTZ`, `unread_count INT DEFAULT 0`, `is_pinned BOOLEAN DEFAULT FALSE`, `updated_at TIMESTAMPTZ`，PK(`user_id`, `conversation_id`)

### API 合约

#### HTTP API（Message Service 对外）

- `POST /v1/messages/send` — 发送消息。Request/Response 见 Spec 04 §5.1。
- `GET /v1/sync/events?cursor={seq}&limit={n}` — 增量同步。Response 见 Spec 06 §3.4。
- `GET /v1/conversations` — 获取会话列表。返回 `conversation_summaries` 中当前用户的记录，按 `last_message_at DESC` 排序，分页。
- `GET /v1/conversations/{cid}/messages?from_seq={seq}&limit={n}` — 会话消息补拉。返回指定会话 `conversation_seq >= from_seq` 的消息，按 seq ASC。
- `POST /v1/auth/register` — 注册（mock：手机号 + 验证码，直接返回 token）。
- `POST /v1/auth/login` — 登录（mock：手机号 + 验证码，返回 JWT access_token + refresh_token）。

#### WebSocket 协议（Gateway 对外）

Opcode 枚举和帧格式见 Spec 05 §3.1–3.2。第一阶段只实现：

- `0x0001–0x0008`：握手、心跳、ACK、ERROR、DISCONNECT、RECONNECT
- `0x1001`：MESSAGE_DELIVERY（服务端→客户端）
- `0x2001–0x2002`：SYNC_EVENT、CONVERSATION_UPDATE

#### gRPC（内部服务间）

- `FanoutService.DeliverMessage(device_id, frame)` — Message Service → Gateway，投递消息帧到指定设备。
- `SyncService.AppendEvent(user_id, event)` — Outbox Consumer → Sync Service，追加同步事件。

### Outbox 消费者行为

消费者逻辑按 Spec 04 §4.3 实现，并补充以下运行时决策：

- 轮询间隔：100ms（pending 事件为空时升至 500ms）。
- 每次拉取上限：100 条（可配置）。
- 重试上限：10 次（可配置），超出进入 dead 状态。
- Lease 机制：`processing` 状态的超时设为 60 秒，超时后由其他消费者实例接管（`UPDATE ... WHERE status='processing' AND processed_at < NOW() - INTERVAL '60s'`）。
- 并发消费：单实例内使用 worker pool（默认 4 workers，可配置）。

### 连接管理决策

- Gateway 是无状态的——连接状态存于进程内存，路由信息存于 Redis。
- Gateway 节点之间不互相通信——投递路由通过 Redis 查表后直接 gRPC 到目标节点。
- 用户路由 TTL = 300s，心跳每 30s 续期一次。
- 重连风暴防御按 Spec 05 §6 实现：IP 限流、user 限流、重连优先队列、退避算法。

### 认证模式

P0 阶段使用 mock 认证：
- 注册/登录时不发真实短信，接受任意 6 位验证码。
- JWT 使用 HS256 签名，claims 包含 `user_id`、`device_id`、`exp`，有效期 1 小时。
- Refresh token 为随机 opaque token，存于 `devices` 表的 `refresh_token_hash` 列，有效期 30 天。
- Gateway 握手时验证 JWT，提取 `user_id` 和 `device_id` 建立 session。

### 可观测性

- 所有 HTTP/gRPC 端点输出 Prometheus metrics（请求计数、延迟直方图、错误率）。
- Outbox 消费者暴露：`pending_count`、`processing_count`、`failed_count`、`consumer_lag_seconds`。
- Gateway 暴露：`active_connections`、`connections_per_second`、`heartbeat_timeouts`。
- 使用结构化日志（`slog`），每条日志带 `trace_id`。
- 不引入分布式追踪系统（Jaeger/Tempo），但 logging 和 metrics 必须可用。

## Testing Decisions

### 什么构成一个好测试

- 只测试外部可观察行为（API response、状态变更、事件产出），不测试内部实现细节。
- 每个测试不应依赖其他测试的运行顺序。
- 集成测试使用真实的 PostgreSQL（testcontainers 或本地实例），不 mock DB。
- WebSocket 测试使用真实的 TCP 连接，不 mock 网络层。
- 单元测试用于纯逻辑（如退避算法、序号缺口检测、已读收敛规则），mock 外部依赖。

### 测试层级

| 层级 | 覆盖目标 | 工具 |
|------|----------|------|
| 单元测试 | 退避算法、幂等去重逻辑、心跳超时计算、序号缺口检测、已读状态 MAX 收敛 | Go `testing` + table-driven tests |
| 集成测试 | HTTP API 端到端（发送→持久化→Outbox 事件生成）、WebSocket 握手→心跳→断开、同步分页 | Go `testing` + `testcontainers-go`（PostgreSQL, Redis） |
| 契约测试 | Protobuf schema 向后兼容、gRPC 接口签名稳定 | `buf breaking` |
| 负载测试 | Gateway 连接容量、消息发送吞吐 | `k6` 或 `vegeta`（独立脚本，不阻塞 CI） |

### 优先级（先写什么）

1. 消息发送→幂等写入→Outbox 生成的集成测试（这是整条链路的根）。
2. WebSocket 握手 + 心跳的集成测试（连接管理是实时投递的基础）。
3. Outbox 消费者投递 + 同步事件写入的集成测试。
4. 增量同步 API 的集成测试。
5. 单元测试（补足边界条件）。

### 测试先例

当前仓库没有已有测试代码。测试风格参考 Go 社区标准：
- Table-driven tests（`github.com/golang/go/wiki/TableDrivenTests`）
- `testcontainers-go` 用于集成测试的 DB/Redis 生命周期管理
- `net/http/httptest` 用于 HTTP handler 测试

## Out of Scope

- **客户端代码（iOS/Android/Desktop）**：阶段一只实现服务端。客户端可用 `websocat`、`curl` 或简单的 Go 命令行工具进行验证。
- **群聊扇出**：只实现 1:1 私聊的消息发送与投递。群聊的写扩散/读扩散策略留到阶段二（Spec 07）。
- **媒体消息上传/下载**：只实现 `message_type = "text"`。图片/文件的上传、缩略图生成、对象存储集成留到阶段二（Spec 08）。
- **推送通知**：离线用户不会收到 APNs/FCM 推送。离线消息只能通过重连后的增量同步补拉。推送编排留给阶段二（Spec 09）。
- **E2EE**：消息以明文 JSON 传输，不做端到端加密。E2EE 密钥管理与每设备加密留给阶段三（Spec 10，P1 学习扩展）。
- **联系人发现与通讯录匹配**：用户之间通过 `user_id` 直接发起会话，不做手机号通讯录匹配（Spec 03 P1 部分）。
- **连接迁移（WiFi ↔ 蜂窝）**：连接断开后走标准重连退避，不做无线网络切换时的无缝迁移（Spec 05 §9，P1 学习扩展）。
- **分布式部署**：所有服务作为独立二进制运行，但不做多实例水平扩展、负载均衡和一致性哈希路由。单实例模式下可正常工作即可。
- **生产级 CI/CD**：提供 `Makefile` 和 `docker-compose.yml` 用于本地开发与测试，不做 GitHub Actions 流水线。
- **管理后台 / Dashboard**：不做 Web UI。通过 `curl`、Prometheus metrics 端点和日志进行运维。

## Further Notes

### 里程碑验证标准

阶段一完成的硬性标准：

1. **消息发送闭环可演示**：用 `curl` 发送一条文本消息 → 服务端持久化 → Outbox 消费者产出投递事件 → 目标设备通过 WebSocket 收到 MESSAGE_DELIVERY 帧。
2. **幂等正确**：用相同 `client_message_id` 发送两次 → 第二次返回 `is_duplicate: true`，数据库中只有一条消息记录，投递只发生一次。
3. **离线补拉正确**：设备断开 WebSocket → 发送方发送 3 条消息 → 设备重连 → `GET /v1/sync/events?cursor=N` 返回这 3 条消息。
4. **多端已读收敛**：设备 A 标记已读到 `seq=100` → 设备 B 通过同步事件收到 `last_read_seq=100` → 设备 B 本地已读位置更新为 `MAX(本地, 100)`。
5. **Outbox 重试不丢消息**：模拟下游短暂不可用 → Outbox 消费者重试 → 恢复后事件被成功处理，不进入 dead 状态。
6. **重连退避**：客户端模拟连续重连 → 延迟逐次递增，不超过 30s 封顶。

### 与后续阶段的接口预留

- `messages.content` 使用 JSONB 而非 text——为 Spec 08 的 `message_type = "image"` 时携带 `{thumbnail_url, media_id, dimensions}` 等结构化 payload 预留。
- `sync_events.event_type` 使用 VARCHAR 枚举——为 Spec 07 的 `group_member_added/removed` 等事件类型预留。
- `conversations.type` 支持 `'direct'` 和 `'group'`——group 类型在阶段一不触发写扩散，但 schema 不拒绝。
- Gateway opcode 范围 `0x3000–0x3FFF` 已预留但阶段一不实现——群组事件帧不会被解析，但不会被拒绝（静默忽略）。

### 开发环境

- 依赖：Go 1.22+、PostgreSQL 16+、Redis 7+、Protocol Buffers compiler (`protoc`)
- 启动方式：`make dev`（通过 `docker-compose up -d` 启动 PG 和 Redis，然后 `go run` 启动各服务）
- 迁移：`make migrate-up`（使用 `golang-migrate` 或 `goose`）
- Proto 生成：`make proto`（`buf generate`）
