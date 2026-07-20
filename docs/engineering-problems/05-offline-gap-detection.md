# 离线消息缺口：断连期间的消息如何高效补回

标签: `offline`, `consistency`

## 问题是什么

设备离线 3 小时，期间产生了 500 条消息。恢复时系统必须：
1. 检测到有消息缺失（gap detection）
2. 高效拉取缺失的消息（不能把所有 500 条一次拉完）
3. 不出现消息重复或顺序错乱

## 典型场景

```
设备离线前: sync_cursor = 100
设备离线
期间别人发了 200 条新消息
设备重连: sync_cursor = 100, 但 latest_event_seq = 300
→ 有 200 个事件没收到
→ 找到缺失的 conversation_seq 范围
→ 逐页拉取具体消息
```

## 通用分析思路

1. **游标是什么？** 一个全局递增的序号，设备记住"我消费到哪里了"。新事件序号 > 游标 = 没收到。
2. **一个游标够吗？** 不够——游标告诉你"有事件没收到"，但不告诉你"哪些 conversation 里的哪些 seq 缺失"。所以需要二级检查。
3. **分层同步**：
   - L1: 全局事件流（告诉你有事件发生了）
   - L2: 会话消息补拉（给你具体消息内容）
4. **游标过期怎么办？** 如果一个用户 30 天不上线，不可能保留 30 天前的所有 sync_events。这时游标回退，客户端全量重建会话列表。

核心原则：**增量同步和全量同步必须共存**。增量是正常态，全量是降级兜底。

## 当前项目方案

### 两层同步（Spec 06）

**L1 — 全局事件流：**
```
GET /v1/sync/events?cursor=N&limit=100
→ 返回: [event_seq = N+1, N+2, ..., N+100]
→ 客户端处理后更新本地 cursor = N+100
```

代码：`livechat-server/internal/sync/service.go` — `GetEvents()`

**L2 — 会话消息补拉：**
```
对每个活跃 conversation:
  本地 max_seq = SELECT MAX(conversation_seq) FROM messages WHERE cid=$cid
  服务端 seq 来自 sync_event payload 中的 conversation_seq
  IF local_max_seq < sync_event_seq THEN
    GET /v1/conversations/{cid}/messages?from_seq={local_max_seq+1}&limit=50
```

代码：`livechat-server/internal/sync/service.go` — `GetMessages()`

### cursor 更新策略

每次 `GET /v1/sync/events` 拉取后，自动将 cursor 更新为最后一条事件的 event_seq。使用 `GREATEST` 保证 cursor 不回退。

### 游标过期的降级

未在 P0 实现（sync_events 没有 TTL 清理）。P0 规模下 sync_events 的体量不会达到 "百万级游标缺口" 的降级阈值。TTL 清理 + 降级逻辑在后续 P1 迭代中补齐。

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| **两层同步**（当前） | 高效、分层清晰 | 需要客户端做两次查询 |
| **纯全量同步** | 逻辑简单 | 每次同步都拉所有数据 |
| **时间戳驱动** | 无需游标 | 时钟偏差导致漏拉 |
| **日志序列 + 快照** | Kafka 式设计 | 需要额外的 snapshot 管理 |

## 踩坑记录

- 服务端分配 conversation_seq 时不依赖 sync_events——sync_events 是投递的**通知**，不是消息的**来源**。即使用户错过了 sync_event，也可以通过 `GET /v1/conversations/{cid}/messages` 直接补拉。避免"事件流兜底数据正确性"的设计——事件流只用于告诉客户端"有什么变化"，具体数据从源表取。
