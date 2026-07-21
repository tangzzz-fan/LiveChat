# 适应性学习 Roadmap

本文件记录 LiveChat 项目当前工程问题库中已明确但尚未在代码中落地的高并发概念，按"何时会遇到 → 在学习什么 → 当前状态"组织。

## 1. 消息投递的实时路径 (gRPC Fanout → Gateway)

**何时会遇到**：Outbox Consumer 需要把事件推到 Gateway 在线会话。

**学习目标**：
- gRPC 的流式投递 vs 单次 RPC 投递
- Gateway 节点不可达时，Fanout 的 fallback（Redis 路由 miss → sync_events）

**当前状态（2026-07-21）**：**已基本落地**。`internal/fanout` + outbox-consumer 注册 `message_created` handler；在线走 Deliverer，离线走 sync；热点群返回 `ErrGroupBusy`。后续可加深：跨节点 gRPC 流式与投递 ACK 闭环压测。

### 已有实现参考 (2026-07-20)

- Outbox Consumer 事件消费循环: `livechat-server/internal/outbox/consumer.go` — `Run()` worker pool
- Fanout 服务投递编排: `livechat-server/internal/fanout/service.go` — `Fanout()` 查成员 → 在线设备 → 投递或 sync
- Sync 事件 AppendEvent: `livechat-server/internal/sync/service.go` — `AppendEvent()`
- LogDeliverer 占位实现: `livechat-server/cmd/outbox-consumer/main.go` — 历史占位；当前已接真实 Fanout handler
- 连接限流: `livechat-server/internal/gateway/ratelimit.go` — 每 IP / 每 user token bucket

## 2. 背压 (Backpressure)

**何时会遇到**：Outbox 积压超过 10,000 条。

**学习目标**：
- Pending 积压时，是否应该限制消息发送接口 (HTTP 429)？
- Worker pool 动态扩容 vs 固定大小
- 消费者延迟监控（当前用 metrics lag_seconds）

**当前状态（2026-07-21）**：Consumer 固定 worker + pending/lag 可观测；**发送侧按 pending 反压尚未实现**。演练见 `docs/chaos/02-outbox-backpressure.md`。

### 已有实现参考 (2026-07-20)

- Outbox Consumer 配置: `livechat-server/internal/outbox/consumer.go` — `Config` 结构体 (BatchSize, MaxRetries 等)
- Worker pool 与事件通道: `livechat-server/internal/outbox/consumer.go` — `Run()` 中的 channel + goroutine
- Consumer metrics: `livechat-server/internal/outbox/consumer.go` — `Metrics()` 含 pending_count 和 lag_seconds
- 退避重试算法: `livechat-server/internal/outbox/consumer.go` — `backoffDuration()`

## 3. 存储分层与分片 (Sharding)

**何时会遇到**：当单 DB 的表超过千万级，或热点会话 (viral group) 写入 QPS 过万。

**学习目标**：
- messages 按 conversation_id 分片 → 跨分片查询 (SELECT * WHERE sender_user_id=?) 怎么优化
- sync_events 按 user_id 分片 → 一个群消息写给 500 个成员时产生 500 条 sync_events (写扩散)
- conversation_summaries 按 user_id 分片 → 跨分片排序 `ORDER BY last_message_at` 需要 scatter-gather

**DDIA 相关**：Ch6 Partitioning — 分区键选择, 二级索引问题, Rebalancing。

### 已有实现参考 (2026-07-20)

- sync_events DDL 已按 user_id 分片: `livechat-server/migrations/006_sync_events.up.sql` — `PRIMARY KEY (user_id, event_seq)`
- messages 按 conversation_id 自然分片: `livechat-server/migrations/004_messages.up.sql` — `INDEX ... (conversation_id, conversation_seq)`
- conversation_summaries 按 user_id 分片: `livechat-server/migrations/008_conversation_summaries.up.sql` — `PRIMARY KEY (user_id, conversation_id)`

## 4. 热点群聊 (Hotspot Group Chat)

**何时会遇到**：单个群聊的并发写入达到每秒数十到数百条。

**学习目标**：
- SEQUENCE 作为单写点在高并发下是瓶颈
- 写扩散 vs 读扩散与热点隔离
- 如何限制热点群的扇出延迟（超时、降级、部分投递）

**当前状态（2026-07-21）**：**已落地保护**：Redis 60s 窗口 `ZCARD > 50` → `ErrGroupBusy`，Consumer 不重试。压测/演练：`load_test` `group_fanout` + `docs/chaos/06-hot-group-flood.md`。仍需关注：busy 时用户可见性语义是否与产品预期一致。

### 已有实现参考 (2026-07-20)

- 单会话 SEQ 写入: `livechat-server/internal/messages/service.go` — `ensureSeq()` + `nextval()`
- Fanout 群成员遍历: `livechat-server/internal/fanout/service.go` — `Fanout()` 查 conversation_members → 逐个处理
- 群事件类型预留: `livechat-server/proto/ws_frame.proto` — opcode 0x3000-0x3FFF (群组事件)

## 5. 连接迁移 (WiFi ↔ 蜂窝)

**何时会遇到**：移动端频繁的网切，导致旧连接仍在、新连接已建立 → 路由混乱。

**学习目标**：
- 迁移期间的暂存消息（缓存 30 秒内未确认的消息）
- 新连接建立后的 conflict resolution（哪个连接"胜出"？）
- 旧连接如何优雅关闭

**DDIA 相关**：Ch8 — 不可靠网络下的部分失败。Spec 05 §9 已设计协议，P1 实现。

### 已有实现参考 (2026-07-20)

- 连接重连退避算法: `livechat-server/internal/gateway/manager.go` — `checkStale()` + 90s 超时
- Redis 路由 TTL 自动过期: `livechat-server/internal/gateway/manager.go` — `registerRoute()` 5min TTL
- 会话迁移冲突处理: `livechat-server/internal/gateway/manager.go` — 旧 session 被新连接踢出 (ErrorFrame should_reconnect=true)

## 6. 写扩散 vs 读扩散 (Fan-out strategy)

**何时会遇到**：群聊规模超过 50 人，或活跃群数量超过数千。

**学习目标**：
- 小群 (< 100 人)→ 写扩散（发消息时直接写每个成员的收消息队列）
- 大群 (> 100 人)→ 读扩散（消息只写一次，成员拉取时查群消息）
- 切换点：多少成员时成本拐点？
- 混合策略：哪些成员是"活跃"的（需要写扩散），哪些是"潜水"的（读扩散就好）

**DDIA 相关**：Ch6 — Partitioning 策略, Ch9 — 冗余 vs 延迟的权衡。

### 已有实现参考 (2026-07-20)

- 当前 P0 Fanout 实现了写扩散: `livechat-server/internal/fanout/service.go` — `Fanout()` 遍历所有成员
- Fanout 已区分 在线投递 vs 离线 sync: 在线 → `deliverer.DeliverMessage()`，离线 → `sync.AppendEvent()`
- sync_events 表支持批量写入: `livechat-server/internal/sync/service.go` — `AppendEvent()` 逐条写入，P1 可优化为批量

## 7. Copy-on-Write 在消息系统中的应用

**何时会遇到**：消息编辑、撤回功能。如果直接修改 messages 表的记录，sync_events 需要和修改时间对齐。

**学习目标**：
- 消息编辑 → 新建一行（新消息 ID），旧行不可见。不修改原行。
- 消息撤回 → 追加一条"撤回"事件（tombstone），原消息保留但客户端不显示。

## 8. 结构化日志与分布式追踪的关系

**何时会遇到**：单个用户报告消息未收到，需要追踪这条消息在整个系统中的路径。

**学习目标**：
- trace_id 贯穿：客户端 → Gateway → Message Service → Outbox → Fanout → Sync/Push
- slog 结构化日志的查询方式（`grep trace_id=xxx` 或 `jq`）
- 与 Jaeger/Tempo 分布式追踪的差异

**DDIA 相关**：Ch12 — 审计与数据流追踪。

### 已有实现参考 (2026-07-20)

- trace_id 已注入 WsFrame: `livechat-server/proto/ws_frame.proto` — `WsFrame.trace_id`
- slog JSON 日志: `livechat-server/cmd/message-service/main.go` — `slog.NewJSONHandler`
- `/metrics` 端点 (expvar): `livechat-server/internal/metrics/metrics.go` — `Handler()`
- HTTP 请求日志含 method、path、duration_ms: `livechat-server/internal/api/router.go` — `withLogging()`
- trace_id 由客户端传入或服务端生成: `WsFrame` proto字段已定义，网关层尚未自动注入

## 9. Clock Skew 对消息排序的影响

**何时会遇到**：使用 `server_received_at` 而非 `conversation_seq` 排序。

**学习目标**：
- NTP 同步误差：两台服务器的时间戳可能差几百毫秒
- 分布式系统中唯一可靠的排序是逻辑时钟或序列号
- Spanner 的 TrueTime API (原子钟 + GPS) — 这是 P1 的概念学习

**DDIA 相关**：Ch8 — 不可靠的时钟。Ch9 — 全局快照隔离。

### 已有实现参考 (2026-07-20)

- 消息排序强制用 conversation_seq: `livechat-server/internal/messages/service.go` — `Send()` 分配 seq → 不按时间戳排序
- 客户端渲染规则: `livechat-server/internal/sync/service.go` — `GetMessages()` `ORDER BY conversation_seq ASC`
- HandshakeResponse 含 server_time_ms: `livechat-server/internal/gateway/manager.go` — 握手时下发服务端时间戳

## 10. Idempotency Keys 与 Idempotency Windows

**何时会遇到**：大量用户同时超时重试 → 同一个 client_message_id 的服务端压力。

**学习目标**：
- 当前用 DB unique constraint → 被动的幂等保护
- 积极型保护 → 服务端缓存最近 5 分钟的 (user_id, client_message_id) → 在 DB 查询之前就拒绝重复
- 幂等窗口 vs 存储开销的权衡

**DDIA 相关**：Ch7 — 事务隔离级别, Ch9 — 全局唯一性保证。

### 已有实现参考 (2026-07-20)

- DB unique constraint 幂等: `livechat-server/internal/messages/service.go` — `ON CONFLICT (sender_user_id, client_message_id) DO NOTHING`
- xmax=0 判断是否新插入: `livechat-server/internal/messages/service.go` — `RETURNING (xmax = 0) AS is_new`
- 幂等命中返回已有记录: `livechat-server/internal/messages/service.go` — 查询已有消息返回 `is_duplicate: true`
- 服务端主动去重: 尚未实现（当前依赖 DB unique constraint 作为被动保护，P1 可增加内存窗口去重）
