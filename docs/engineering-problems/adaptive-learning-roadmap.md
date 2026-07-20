# 适应性学习 Roadmap

本文件记录 LiveChat 项目当前工程问题库中已明确但尚未在代码中落地的高并发概念，按"何时会遇到 → 在学习什么 → 当前状态"组织。

## 1. 消息投递的实时路径 (gRPC Fanout → Gateway)

**何时会遇到**：当 Outbox Consumer 不止打印日志，而是真的通过 gRPC 推送到 Gateway。

**学习目标**：
- gRPC 的流式投递 vs 单次 RPC 投递
- Gateway 节点不可达时，Fanout 的 fallback 策略（当前是 Redis 路由 miss → 直接走 sync_events）

**当前状态**：P0 已实现 `logDeliverer` (日志占位)。完整 gRPC 投递是第一个要补齐的问题。

## 2. 背压 (Backpressure)

**何时会遇到**：Outbox 积压超过 10,000 条。

**学习目标**：
- Pending 积压时，是否应该限制消息发送接口 (HTTP 429)？
- Worker pool 动态扩容 vs 固定大小
- 消费者延迟监控（当前用 metrics lag_seconds）

**DDIA 相关**：Ch11 Stream Processing — 消费者处理速度落后于生产者的处理策略。

## 3. 存储分层与分片 (Sharding)

**何时会遇到**：当单 DB 的表超过千万级，或热点会话 (viral group) 写入 QPS 过万。

**学习目标**：
- messages 按 conversation_id 分片 → 跨分片查询 (SELECT * WHERE sender_user_id=?) 怎么优化
- sync_events 按 user_id 分片 → 一个群消息写给 500 个成员时产生 500 条 sync_events (写扩散)
- conversation_summaries 按 user_id 分片 → 跨分片排序 `ORDER BY last_message_at` 需要 scatter-gather

**DDIA 相关**：Ch6 Partitioning — 分区键选择, 二级索引问题, Rebalancing。

## 4. 热点群聊 (Hotspot Group Chat)

**何时会遇到**：单个群聊的并发写入达到每秒数千条。

**学习目标**：
- SEQUENCE 作为单写点在高并发下是瓶颈（`nextval()` 在单个 SEQUENCE 上有争抢）
- 写扩散 vs 读扩散：500 人群聊是否应为每个成员单独写一条 sync_event？
- 如何限制热点群的扇出延迟（超时、降级、部分投递）

**DDIA 相关**：Ch6 — Hotspot 处理, Ch9 — 顺序保证的代价。

## 5. 连接迁移 (WiFi ↔ 蜂窝)

**何时会遇到**：移动端频繁的网切，导致旧连接仍在、新连接已建立 → 路由混乱。

**学习目标**：
- 迁移期间的暂存消息（缓存 30 秒内未确认的消息）
- 新连接建立后的 conflict resolution（哪个连接"胜出"？）
- 旧连接如何优雅关闭

**DDIA 相关**：Ch8 — 不可靠网络下的部分失败。Spec 05 §9 已设计协议，P1 实现。

## 6. 写扩散 vs 读扩散 (Fan-out strategy)

**何时会遇到**：群聊规模超过 50 人，或活跃群数量超过数千。

**学习目标**：
- 小群 (< 100 人)→ 写扩散（发消息时直接写每个成员的收消息队列）
- 大群 (> 100 人)→ 读扩散（消息只写一次，成员拉取时查群消息）
- 切换点：多少成员时成本拐点？
- 混合策略：哪些成员是"活跃"的（需要写扩散），哪些是"潜水"的（读扩散就好）

**DDIA 相关**：Ch6 — Partitioning 策略, Ch9 — 冗余 vs 延迟的权衡。

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

## 9. Clock Skew 对消息排序的影响

**何时会遇到**：使用 `server_received_at` 而非 `conversation_seq` 排序。

**学习目标**：
- NTP 同步误差：两台服务器的时间戳可能差几百毫秒
- 分布式系统中唯一可靠的排序是逻辑时钟或序列号
- Spanner 的 TrueTime API (原子钟 + GPS) — 这是 P1 的概念学习

**DDIA 相关**：Ch8 — 不可靠的时钟。Ch9 — 全局快照隔离。

## 10. Idempotency Keys 与 Idempotency Windows

**何时会遇到**：大量用户同时超时重试 → 同一个 client_message_id 的服务端压力。

**学习目标**：
- 当前用 DB unique constraint → 被动的幂等保护
- 积极型保护 → 服务端缓存最近 5 分钟的 (user_id, client_message_id) → 在 DB 查询之前就拒绝重复
- 幂等窗口 vs 存储开销的权衡

**DDIA 相关**：Ch7 — 事务隔离级别, Ch9 — 全局唯一性保证。
