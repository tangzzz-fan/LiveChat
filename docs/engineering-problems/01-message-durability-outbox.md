# 消息不丢：写入与投递之间的一致性裂缝

标签: `durability`, `idempotency`

## 问题是什么

消息写入 DB 成功，但通知投递系统的消息丢失了——"数据在库里但永不会被投递"（幽灵消息）。

## 典型场景

```
INSERT INTO messages (...) VALUES (...);  -- ✅ 成功
publish_to_kafka(message_event);          -- ❌ 失败（网络抖动 / broker 宕机）
```

- 数据库写入和消息队列投递不是原子操作。
- 不限于 Kafka——任何写 DB + 通知下游的两步操作都有这个风险。
- 聊天系统中每条消息都必须投递，丢失不是可接受状态。

## 通用分析思路

1. 问自己：**"哪个是真理源？"** → 通常是 DB（因为 DB 有事务保证）。
2. 问自己：**"下游怎样从真理源重新获取？"** → 需要一张"待处理"表。
3. 问自己：**"原子性的边界在哪？"** → 真理源写入和"待处理"事件写入必须在同一事务。

核心原则：**不要在事务中做 I/O**。事务内只写数据，事务外才做网络调用。

## 当前项目方案

### Outbox 模式（Spec 04）

```
BEGIN
  INSERT INTO messages (...);              -- 真理写入
  INSERT INTO outbox_events (...);         -- 事务性事件
COMMIT
                                          -- 事务提交后：
→ 独立 Consumer 轮询 outbox_events         -- 异步，可重试
→ Fanout、Sync、Push                       -- 下游操作
```

代码：`livechat-server/internal/messages/service.go` — `Send()` 在同一事务中写入 messages + outbox_events。

### 为什么放在同一事务就能解决

- PostgreSQL 保证事务提交后两条 INSERT 同时可见。
- Consumer 轮询 outbox_events，只有 `status='pending'` 的记录。
- 消费者崩溃 → lease 回收 → 重试，不受事务影响。
- `FOR UPDATE SKIP LOCKED` 防止多个 consumer 争抢同一条。

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| **Outbox + 轮询**（当前） | 简单、零额外依赖、事务保证强 | 轮询延迟（100ms），不适合需要 <10ms 投递的场景 |
| **CDC (Debezium + Kafka)** | 实时性好、解耦 | 运维复杂、需要 Kafka 集群、排障困难 |
| **两阶段提交 (XA)** | 严格原子性 | 性能极差、大多数 MQ 不支持、已过时 |
| **直接在事务中做网络调用** | 代码简单 | 事务内阻塞、连接池耗尽风险、永远不要这样做 |

## 踩坑记录

- PostgreSQL `ON CONFLICT DO NOTHING` 返回 `(xmax=0) AS is_new` 来判断是否真的插入了新行。`xmax=0` 表示该行已插入但未被任何事务标记为删除（即新行）；如果 `xmax != 0` 且 `DO NOTHING` 生效，说明发生了冲突——行已存在且没有被删除。这是判断幂等命中的可靠方式。
- 幂等重发时不能只返回给客户端就结束——需要查已有消息的 `server_message_id` 和 `conversation_seq` 返回，让客户端知道"这条消息就是之前那条"。
