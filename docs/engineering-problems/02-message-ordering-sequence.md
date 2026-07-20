# 消息顺序：弱网重试下的乱序风险

标签: `ordering`, `idempotency`

## 问题是什么

客户端因网络抖动对同一会话连续发送 M1、M2、M3。M1 超时重试，M2 和 M3 先到达服务端——服务端按到达时间分配 conversation_seq。结果：M1 的 seq > M2 的 seq，客户端看到消息乱序。

## 典型场景

```
时间轴（客户端视角）：
  t0: 发送 M1 (client_msg_id = "abc-001")
  t1: 发送 M2 (client_msg_id = "abc-002")    -- M1 正在重试中
  t2: 发送 M3 (client_msg_id = "abc-003")
  
时间轴（服务端视角）：
  先收到: M3 → seq=1
  再收到: M2 → seq=2
  最后收到: M1（重试）→ seq=3（但它的语义是"第一条"）
```

## 通用分析思路

1. **序号谁来管？** 如果客户端自己编序号，依赖客户端时钟 → 不同设备的时钟偏差导致更严重的问题。
2. **单写点原则**：如果所有并发写入都经过同一个服务端序列化点，序号分配就是天然有序的。
3. **服务端序号 vs 客户端序号**：两种序号的用途不同——
   - 客户端序号 = 幂等键（去重）
   - 服务端序号 = 会话内展示顺序

核心思想：**服务端是会话内顺序的唯一真理源**。客户端可以决定"发送什么"，但不能决定"排在哪个位置"。

## 当前项目方案

### PostgreSQL SEQUENCE 分配 conversation_seq（Spec 04 §5.1）

```
会话 conv-abc 的所有消息写入都通过同一个 SEQUENCE:
  SELECT nextval('conversation_seq_conv_abc') AS seq
```

- 单写点：一个会话一个 SEQUENCE，所有并发写入串行化。
- 不回滚：`nextval()` 即使在事务 rollback 后也不会回收已分配的序号。
- 客户端用 `SELECT * FROM messages ORDER BY conversation_seq ASC` 渲染消息列表。

代码：`livechat-server/internal/messages/service.go` — `ensureSeq()` 动态创建 SEQUENCE。

### 序号间隙是正常的

因为 `nextval()` 不回滚，如果事务 A 拿到 seq=10 但 rollback 了，seq=10 永远不会出现在这个会话里。客户端不应假设 seq 连号，应始终按 `ORDER BY seq ASC` 渲染。

### 为什么不让客户端编序号

WhatsApp 等系统使用 "sender timestamp + sequence" 组合排序，但这依赖于所有设备时钟同步——不是 P0 应该解决的复杂度。

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| **SEQUENCE**（当前） | 简单、严格递增 | 间隙、同一会话有写入瓶颈 |
| **服务端时间戳** | 无间隙 | NTP 时钟偏差导致逆序 |
| **Lamport 逻辑时钟** | 分布式友好 | 客户端驱动、信任客户端 |
| **CRDT** | 冲突自动合并 | 只能用于可交换操作、消息排序不适用 |

## 踩坑记录

- PostgreSQL `nextval()` 默认 cache=1，在高并发写入同一会话时可能有轻微的序列化开销。但对于 < 1000 msg/s/会话的场景，这不是瓶颈。
- SEQUENCE 名需要 sanitize——conversation_id 可能含 `-` 等字符，PostgreSQL 标识符不允许。当前用正则 `[^a-zA-Z0-9_]` → `_` 替换。
