# 关键技术决策与实现细节

本文档记录 livechat-server 实现过程中的关键架构决策、数据流约定和实现细节，作为 Specs 与代码之间的桥梁。

## 1. 消息发送链路：幂等写入与 Outbox 事务

### 决策

`POST /v1/messages/send` 的写入路径中，`messages` 表和 `outbox_events` 表的 INSERT 必须在**同一个数据库事务**中完成。事务提交成功则两者同时可见，失败则都不写入。

### 为什么

这是整个系统正确性的根。如果消息写了但 Outbox 事件没有写，消息变成了"幽灵消息"——数据在库里但永远不会被投递。如果 Outbox 事件写了但消息没有写，消费者会拿到一个无意义的事件。

### 实现

```go
// internal/messages/service.go — Send()
tx, _ := db.BeginTx(ctx, nil)
// 1. INSERT INTO messages ... ON CONFLICT DO NOTHING
// 2. INSERT INTO outbox_events ...
tx.Commit()
```

`ON CONFLICT (sender_user_id, client_message_id) DO NOTHING` 保证了幂等性：客户端因超时重发时，服务端不会写入重复消息，也不会生成重复投递事件。第二次请求通过查询已有消息的 `server_message_id` 和 `conversation_seq` 返回相同结果，但 `is_duplicate=true`。

### ConversationSummary 更新为什么不在同一事务

向每个 conversation member 写入 summary 会增加事务锁范围和延迟。这里的选择是：**消息已持久化即返回 200，summary 异步更新（eventual consistency）**。最差情况下，接收方的会话列表可能在数百毫秒内看不到新消息（直到 summary 更新完成），但消息本身已经安全。

---

## 2. conversation_seq 分配方式

### 决策

每个会话使用独立的 PostgreSQL `SEQUENCE`（`conversation_seq_{sanitized_conv_id}`），通过 `nextval()` 获取严格递增序号。

### 为什么

会话内消息顺序是聊天系统最基础的正确性保证。PostgreSQL SEQUENCE 提供：
- 单写点：所有并发写入同一会话的请求都通过同一个 SEQUENCE 串行化。
- 不回滚：即使事务失败，序号也不会复用，因此客户端不会看到"同一位置出现不同消息"。
- 高性能：`nextval()` 是内存操作，不受表锁影响。

### 序号间隙是正常的

由于 `nextval()` 不回滚（即使事务 rollback），会话内的 conversation_seq 可能存在间隙（例如 1, 2, 4, 5）。客户端通过 `GET /v1/conversations/{cid}/messages` 按 seq 拉取时不应假设 seq 是连续的，而应按 `ORDER BY conversation_seq ASC` 渲染。

---

## 3. Outbox Consumer：轮询 vs CDC

### 决策

P0 阶段使用数据库轮询（`SELECT ... FOR UPDATE SKIP LOCKED`），不引入 Debezium/Kafka/CDC。

### 为什么

- 学习目标优先：轮询模式的消费者逻辑简单，可以直接阅读和理解。
- 零外部依赖：不需要 Kafka、ZooKeeper、Debezium 等重型基础设施。
- P0 的 QPS 规模（数百~数千条/秒）下，轮询延迟（100ms 间隔 + 单次 100 条批处理）完全可以接受。

### 如何处理消费者实例并发

`FOR UPDATE SKIP LOCKED` 确保多个 consumer 实例不会同时处理同一行。每个实例用 worker pool（默认 4 workers）并行处理事件。

### Lease 机制

如果有 consumer 实例在处理中崩溃，事件会卡在 `status='processing'`。`reapStale()` 函数每 30 秒扫描一次，将超过 60 秒的 `processing` 事件重置为 `retry`。

---

## 4. Gateway 会话路由模型

### 决策

Gateway 是无状态的，连接信息存进程内存，路由信息存 Redis。Gateway 节点之间不直接通信，投递通过 Fanout 服务查 Redis 路由后 gRPC 到目标节点。

### 为什么

- Gateway 节点的核心职责是"找到目标连接并写数据"，不应该承载业务逻辑。
- 如果 Gateway 之间相互通信，会引入 gossip 协议复杂度。
- Redis 作为路由表天然支持 TTL（心跳续期）和高效的 `KEYS` 查询。
- P0 阶段单 Gateway 实例足以验证协议正确性；多实例部署留到 P1。

### Redis Key 设计

```
gateway:user:{user_id}:{device_id}   → "{node_id}:{session_id}"   TTL=300s
gateway:node:{node_id}:connections    → SET of "user_id:device_id"  TTL=300s
```

心跳每 30s 刷新一次 TTL。90s 无心跳 → Redis TTL 自动过期 → 路由失效，后续投递自动走 sync_events。

---

## 5. 消息投递的两条路径

### 实时投递

```
Send → Outbox → Consumer → Fanout → 查 Redis 路由
                                    ├─ 在线: gRPC → Gateway → WebSocket 推送
                                    └─ 离线: sync_events 写入
```

当前实现已经按 Spec 05 收敛为强契约内部接口：

- Gateway 暴露 `GatewayDeliveryService.DeliverMessage` gRPC 服务。
- Outbox Consumer / Fanout 侧通过 `GatewayDeliveryServiceClient` 将 `WsMessageDelivery` protobuf 投递到目标 Gateway 节点。
- 若目标 `device_id` 在本节点不存在，Gateway 返回 gRPC `NotFound`，并清理过期路由，后续由 `sync_events` 补拉兜底。
- `sync_events` 仍是事实来源，实时投递只是在线设备的加速路径。

这替代了早期的节点内临时通道实现，后续开发不再以临时传输层作为基线。

### 离线补拉

```
设备重连 → WebSocket 握手（携带 last_event_seq）
→ Gateway 返回 latest_event_seq
→ 客户端发现 gap → GET /v1/sync/events?cursor=N
→ 同步完成后 → GET /v1/conversations/{cid}/messages?from_seq=N 补齐具体消息
```

---

## 6. 多端已读状态收敛规则

### 决策

已读位置使用 `last_read_seq = MAX(本地值, 同步事件中的值)` 收敛。

### 为什么

这是分布式系统中最简单的确定性收敛规则。MAX 操作满足结合律和交换律，无论事件到达顺序如何，所有设备最终收敛到同一个值。

### 实现位置

- 服务端：`conversation_summaries.unread_count` 在 `MarkRead()` 时重置为 0。
- 客户端：收到 `sync_event`（`event_type='message_read'` 或 `conversation_updated`）后，本地 `last_read_seq = max(local, event.last_read_seq)`。
- `sync_cursors` 的 `last_event_seq` 也使用 `GREATEST` 更新，防止旧游标回退。

### ACK 上送链路

`ACK(read)` 不在 Gateway 本地落业务库，而是走独立的内部服务链路：

```
Client → Gateway(WebSocket ACK)
       → gRPC MessageAckService.ProcessAck
       → outbox_events(read_receipt)
       → Outbox Consumer
       → conversation_updated / message_read sync_events
```

原因是 Gateway 只负责连接与协议，已读推进属于业务语义和多端一致性范畴，必须进入 Message Service 的单一可信源。

---

## 7. Protobuf 帧设计

### 为什么用 Protobuf Binary 而不是 JSON Text

- 节省 40-60% 带宽（对于移动端长连接至关重要）。
- Schema 演进：添加字段不破坏旧客户端。
- 类型安全：编译期检查，消除手写 JSON 解析错误。

### 帧结构

```
[4 bytes length prefix (big-endian)] [Protobuf WsFrame bytes]
```

- Length prefix 上限 1 MiB。
- `WsFrame.opcode` 决定 `payload` 的反序列化目标类型。
- `WsFrame.seq_id` 和 `ack_seq_id` 为协议层提供的序列号和捎带 ACK，P0 尚未使用，为 P1 的可靠投递确认预留。

---

## 8. 认证方案（P0 Mock）

### 决策

P0 使用 mock 短信验证：任意 6 位数字即通过。JWT 使用 HS256（共享密钥），未引入非对称密钥体系。

### 为什么

- 真实短信网关（Twilio/阿里云）需要企业资质和成本。
- PKI/非对称密钥管理属于 Spec 10 的安全体系范畴，不应在消息正确性阶段引入。
- JWT 的 claims 设计（`user_id + device_id + exp`）与生产一致，后续只需更换签名算法和验证方式。

### JWT Claims

```json
{
  "user_id": 1,
  "device_id": "ios-dev-001",
  "exp": 1752681600,
  "iat": 1752678000
}
```

- 有效期：1 小时
- Gateway 握手时验证 JWT，提取 `user_id` 和 `device_id`，不查 DB。
- Refresh token 为随机 32 字节的 SHA-256 hash，存于 `devices.refresh_token_hash`。

---

## 9. 错误处理策略

### HTTP API

| 场景 | HTTP Status | 是否需要重试 |
|------|------------|-------------|
| 缺少必填字段 | 400 | 否 |
| JWT 无效/过期 | 401 | 否（刷新 token） |
| 非会话成员 | 403 | 否 |
| 会话不存在 | 403（故意不区分） | 否 |
| DB 写入失败 | 500 | 是（退避重试） |
| 幂等命中 | 200 + `is_duplicate:true` | 否 |

### Outbox Consumer

| 重试次数 | 行为 |
|----------|------|
| 0-9 | exponential backoff（1s, 2s, 4s, 8s, 16s, 30s），重试 |
| 10+ | 标记 `failed`（死信），不自动重试 |

### Gateway

- JWT 验证失败 → `ErrorFrame`（`should_reconnect=false`）
- 同设备新连接 → 旧连接收到 `ErrorFrame`（`should_reconnect=true`）后被踢出
- 超时断开 → `ErrorFrame`（`should_reconnect=true`）
