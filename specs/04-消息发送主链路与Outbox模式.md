# Spec 04 — 消息发送主链路与 Outbox 模式

## 1. 目标

定义消息从客户端点击发送到接收方设备展示的完整主链路。核心挑战不是"能发出去"，而是：**消息不丢、不重、不乱序——在弱网、重试、服务重启、网络分区等所有异常场景下。** 本 spec 是理解大规模实时聊天系统正确性的核心。

## 2. 核心挑战

### 2.1 "发送成功"的语义陷阱

这是聊天系统最常见的 bug 来源。客户端收到 HTTP 200 不代表对端收到了消息：

```
客户端理解:                              服务端真实状态:
────────────────────────────────────────────────────────
"发送成功，对端应该看到了"    →    "消息已持久化，投递进行中"
"发送失败，需要重试"          →    "消息已持久化，HTTP 响应丢失"
"超时了，我重发一次"          →    重复消息（如果没幂等）
```

**正确分层：**

| 阶段 | 语义 | 确认方式 | 客户端状态 |
|------|------|----------|-----------|
| Accepted | 服务端已持久化 | HTTP 200 | accepted |
| Delivered | 消息已到达对方设备 | WebSocket ACK 事件 | delivered |
| Read | 对方已查看 | Read Receipt 事件 | read |

### 2.2 写入数据库与消息投递的一致性裂缝

```
// 危险模式：非事务性
INSERT INTO messages (...);      // ← 成功
publish_to_kafka(message_event); // ← 失败 → 消息写了但没人投递! 幽灵消息!
```

### 2.3 弱网下的重试风暴

客户端超时后重试 → 服务端压力增大 → 更多超时 → 更多重试 → **正反馈循环**。

## 3. 工业界方案对比

### 3.1 WhatsApp 的方案

WhatsApp 后端(Erlang/OTP + FreeBSD +自定义 XMPP):
- 消息写入 Mnesia(分布式 DB)后,通过 Erlang 消息传递触发扇出
- 进程模型:每个用户/会话有独立的 Erlang Process,天然隔离
- ACK 是 XMPP 协议层的一部分,不是 HTTP

### 3.2 我们的方案（简化版，学习工业化思想）

对于非 Erlang 体系（Go/Rust/Java + PostgreSQL + Kafka）:
- **Outbox 模式**:消息写入 + Outbox 事件写入在同一条 DB 事务中
- **Debezium/CDC** 或独立消费者从 Outbox 表读取事件
- **At-Least-Once + 幂等**:投递至少一次，接收端根据 `server_message_id` 去重

## 4. Outbox 模式详解

### 4.1 为什么需要 Outbox

```
没有 Outbox:
  BEGIN
    INSERT INTO messages  -- 成功
  COMMIT
  → 现在需要通知 Kafka/扇出消费者 → 网络调用
  → 如果网络调用失败? 消息孤岛!

有 Outbox:
  BEGIN
    INSERT INTO messages;
    INSERT INTO outbox_events (aggregate_id, event_type, payload);
  COMMIT  ← 两条要么都成,要么都不成
  → 异步消费者从 outbox_events 读取(轮询或 CDC)
  → 读取失败? 重试! 幂等保证安全
```

### 4.2 Outbox 表结构

```sql
CREATE TABLE outbox_events (
    id              BIGSERIAL PRIMARY KEY,
    aggregate_type  VARCHAR(50) NOT NULL,   -- 'message', 'receipt', 'group_event'
    aggregate_id    VARCHAR(64) NOT NULL,   -- server_message_id
    event_type      VARCHAR(100) NOT NULL,  -- 'message_created', 'delivery_acked', 'read_receipt'
    payload         JSONB NOT NULL,         -- 事件体
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending/processing/done/failed
    processed_at    TIMESTAMPTZ,
    retry_count     INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    
    INDEX idx_outbox_status_created (status, created_at)
);

-- 消费者查询:
-- SELECT * FROM outbox_events
-- WHERE status = 'pending'
-- ORDER BY created_at
-- LIMIT 100;
```

### 4.3 Outbox 消费者

```
Outbox Consumer (独立进程/服务):

while true:
    events = fetch_pending_events(limit=100)
    
    for event in events:
        update_status(event.id, 'processing')
        
        switch event.event_type:
            case 'message_created':
                → FanoutService.fanout(event)    // 投递到在线设备
                → SyncService.append(event)       // 追加到同步事件流
                → PushService.schedule(event)     // 离线推送
            case 'delivery_acked':
                → 更新投递状态, 通知发送者
            case 'read_receipt':
                → 更新已读状态, 广播给其他设备
        
        update_status(event.id, 'done')
        
        if processing_failed:
            retry_count++
            if retry_count < max_retries:
                update_status(event.id, 'pending')  // 稍后重试
            else:
                update_status(event.id, 'failed')   // 进入死信队列
                → alert! 需要人工介入
```

#### 消费者幂等保证

- 每个 Outbox 事件以 `aggregate_id + event_type` 为业务幂等键。
- 投递下游（Fanout / Sync / Push）必须能处理重复事件：Fanout 根据 `server_message_id` 去重，Sync 根据 `(user_id, event_seq)` 去重，Push 根据 `(user_id, conversation_id, server_message_id)` 去重。
- 消费者处理完成后才更新 `status='done'`，崩溃重启后会重新消费 `processing` 状态的事件（需配合处理超时或 lease 机制）。

#### 死信告警规则

| 条件 | 告警级别 | 响应 |
|------|----------|------|
| `failed` 事件数 > 10 / 分钟 | P1 | 人工介入，检查下游服务 |
| 单条事件 `retry_count` >= max_retries | P2 | 自动入 DLQ，触发通知 |
| 消费者消费延迟 > 30 秒 | P1 | 扩容消费者或检查 DB 性能 |
| `pending` 事件积压 > 10,000 | P0 | 立即触发限流与扩容 |

## 5. 消息接收主链路（服务端）

### 5.1 详细步骤

```
POST /v1/messages/send
Authorization: Bearer <access_token>

Request:
{
  "client_message_id": "dev_a_20260709_001",   // 幂等键
  "conversation_id":    "conv_abc123",
  "message_type":       "text",
  "content":            "<message_payload>",  // P0: 明文或业务层加密; P1: E2EE 密文
  "plaintext_preview":  "",                    // 可选, 用于通知预览
  "reply_to_message_id": null,
  "client_sent_at_ms":  1752681600000
}

服务端处理:
  1. 鉴权: 校验 access_token → 提取 user_id, device_id
  2. 校验: sender 属于该 conversation, 未被禁言
  3. 分配 seq:
     SELECT nextval('conversation_seq_' || conversation_id) AS seq
  4. 幂等写入 (同一事务):
     BEGIN
       INSERT INTO messages (...)
       VALUES ($..., client_message_id, ...)
       ON CONFLICT (sender_user_id, client_message_id) DO NOTHING
       RETURNING server_message_id, conversation_seq,
                 (xmax = 0) AS is_new;
       
       IF is_new THEN
         INSERT INTO outbox_events (
           aggregate_type, aggregate_id, event_type, payload
         ) VALUES (
           'message', server_message_id, 'message_created',
           jsonb_build_object(
             'server_message_id', server_message_id,
             'conversation_id', conversation_id,
             'conversation_seq', conversation_seq,
             'sender_user_id', sender_user_id,
             'sender_device_id', device_id,
             'message_type', message_type,
             'created_at', NOW()
           )
         );
       END IF;
     COMMIT;
  5. 更新 ConversationSummary (异步或同步):
     UPDATE conversation_summaries
     SET last_message_id = server_message_id,
         last_message_preview = plaintext_preview,
         last_message_at = NOW(),
         updated_at = NOW()
     WHERE conversation_id = $1;
     -- 对每个成员 (除发送者):
     UPDATE conversation_summaries
     SET unread_count = unread_count + 1,
         updated_at = NOW()
     WHERE conversation_id = $1 AND user_id != $sender;
     
Response 200:
{
  "server_message_id": "msg_abc123_00042",
  "conversation_seq":   42,
  "is_duplicate":       false,   // 幂等命中时为 true
  "server_received_at_ms": 1752681600123
}
```

### 5.2 时序图

```
Client A          API Gateway       Message Service      PostgreSQL        Outbox Consumer
   │                   │                   │                  │                    │
   │ POST /send        │                   │                  │                    │
   │──────────────────►│                   │                  │                    │
   │                   │ Validate JWT      │                  │                    │
   │                   │──────────────────►│                  │                    │
   │                   │                   │ BEGIN TX         │                    │
   │                   │                   │─────────────────►│                    │
   │                   │                   │ INSERT message   │                    │
   │                   │                   │─────────────────►│                    │
   │                   │                   │ INSERT outbox    │                    │
   │                   │                   │─────────────────►│                    │
   │                   │                   │ COMMIT           │                    │
   │                   │                   │─────────────────►│                    │
   │                   │                   │                  │                    │
   │                   │  200 {accepted}   │                  │                    │
   │◄──────────────────│◄──────────────────│                  │                    │
   │                   │                   │                  │                    │
   │                   │                   │                  │ poll pending       │
   │                   │                   │                  │◄───────────────────│
   │                   │                   │                  │ events             │
   │                   │                   │                  │───────────────────►│
   │                   │                   │                  │                    │
   │                   │                   │                  │                    │ Fanout
   │                   │                   │                  │                    │ ──┐
   │                   │                   │                  │                    │   │
   │                   │                   │                  │                    │◄──┘
   │                   │                   │                  │                    │
   │  ◄── WebSocket: message_delivery ────│                  │                    │
   │  （异步，通常比 HTTP 响应晚 100-500ms）                  │                    │
```

## 6. 失败补偿策略

### 6.1 各类失败的处理

| 失败类型 | 场景 | 处理 |
|----------|------|------|
| 参数校验失败 | 无权限/会话不存在 | 返回 4xx，客户端标记 failed，不重试 |
| DB 写入失败 | 连接池耗尽/死锁 | 返回 5xx，客户端退避重试 |
| HTTP 响应丢失 | 服务端成功但响应未到达 | 客户端重试 → 命中幂等 → 返回原结果 |
| Outbox 消费失败 | 消费者崩溃/网络抖动 | retry_count + 退避，最多 10 次 |
| 投递时目标不在线 | 对方离线 | 写入离线同步流，等对方上线补拉 |
| 投递时网关不可达 | 网关 Pod 挂了 | 重试, 网关集群有冗余 |

### 6.2 客户端重试策略

```
class MessageSendRetryPolicy {
    maxRetries = 5
    baseDelay = 1 second
    maxDelay  = 30 seconds
    
    func delay(attempt: Int) -> TimeInterval {
        // 指数退避 + 随机抖动
        let exponential = pow(2.0, Double(attempt))
        let jitter = Double.random(in: 0...0.5)
        return min(exponential + jitter, maxDelay)  // 封顶 30 秒
    }
    
    // 重试序列: 1s → 2.5s → 4.5s → 8.5s → 16.5s
    // 总计 ~33 秒后放弃
}
```

**触发重试的条件**：
- 网络超时（未收到 HTTP 响应）。
- 收到 5xx / 503 / 429（带 `Retry-After` 时优先遵循服务端建议）。

**不重试的条件**：
- 收到 4xx（参数错误、无权访问）→ 直接标记 `failed`。
- 幂等命中返回 `is_duplicate=true` → 直接推进状态，不再请求。

**服务端保护**：
- 网关/接口层应对同一 `(user_id, client_message_id)` 的请求做短时间窗口去重，防止客户端网络抖动导致突发重试。

### 6.3 死信处理

```
死信队列 (DLQ):
  outbox_events WHERE retry_count >= max_retries AND status = 'failed'
  
处理:
  1. 自动告警 → Slack/PagerDuty
  2. 人工检查原因 → 修复(如 MQ 恢复)
  3. 手动重置状态 → outbox_events SET status='pending', retry_count=0
```

## 7. 消息去重（客户端侧）

即使服务端保证幂等写入，投递路径也可能重复（Outbox 消费者 At-Least-Once）。客户端必须做第二次去重。

```
客户端本地 SQLite:

func insertMessageIfNeeded(message: ServerMessage) -> Bool {
    // UNIQUE(server_message_id) 保证不重复
    let sql = """
        INSERT OR IGNORE INTO messages (
            server_message_id, conversation_id, conversation_seq,
            sender_user_id, message_type, content, server_received_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?)
        """
    let result = try db.execute(sql, ...)
    return result.changes == 1  // true = 新消息, false = 已存在(重复)
}

// 如果返回 false → 不更新 UI, 不做任何通知
```

## 8. 交付物

- [ ] 发送主链路时序图（本 spec §5.2）
- [ ] Outbox 表 DDL（本 spec §4.2）
- [ ] Outbox 消费者逻辑（本 spec §4.3）
- [ ] 幂等键完整设计（Spec 02 §6）
- [ ] 失败补偿矩阵（本 spec §6）
- [ ] 客户端重试策略伪代码（本 spec §6.2）
- [ ] 消息去重客户端实现（本 spec §7）
