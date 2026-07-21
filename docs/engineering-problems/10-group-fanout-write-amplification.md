# 群消息写扩散：1 条消息 N 倍写入的代价与控制

标签: `fanout`, `scale`, `consistency`

## 问题是什么

在 200 人群里发 1 条文本消息，服务端需要为 199 个成员分别写入 `sync_events`（离线同步流）和更新 `conversation_summaries`（会话列表投影）。1 条消息 → 199 次 `sync_events` 写入 → 199 次 `conversation_summaries` 更新。这种 **O(N)** 的放大效应是群聊系统的核心架构挑战。

## 典型场景

```
200 人群，每秒 100 条消息：

messages INSERT:              100 次/秒
outbox_events INSERT:         100 次/秒  
sync_events INSERT:        19,900 次/秒  ← 199 倍放大
conversation_summaries UPSERT: 19,900 次/秒  ← 199 倍放大
在线投递 RPC:              ~10,000 次/秒
```

如果每增加一个群成员就线性增加写入量，系统在群规模扩大时会迅速崩溃。

## 通用分析思路

1. **先分场景，再定策略**：小群（家庭群 5 人）和大群（校友群 200 人）对延迟和吞吐的要求不同。
2. **写扩散 vs 读扩散**：这是群聊的根本取舍。WhatsApp 限定 256 人用写扩散，Telegram 支持 20 万人用读扩散。
3. **热点群隔离**：少数群产生大部分消息，需要独立资源池保护系统整体不被拖垮。

### 写扩散（Inbox Model）
- 写入时：为每个成员创建一条消息副本/投递任务
- 读取时：直接读自己的收件箱
- 适用：≤256 人的群
- 优势：读取快、离线同步简单
- 劣势：写入放大 N 倍

### 读扩散（Outbox Model）
- 写入时：只存 1 份消息（群信箱）
- 读取时：每个成员拉取时再去群信箱读
- 适用：任意规模
- 优势：写入只 1 次
- 劣势：读取需要追踪游标，离线同步更复杂

## 当前项目方案

LiveChat 采用**三级分层策略**（0013 实现）：

| 群规模 | 策略 | 在线投递 | sync_events | conversation_summary |
|-------|------|---------|-------------|---------------------|
| ≤50 人（小群） | 全写扩散 | 实时投递 | 所有成员写入 | 所有成员更新 |
| 51-200 人（中群） | 混合 | 实时投递 | 所有成员写入 | **不更新（按需计算）** |
| >200 人（大群） | 纯读扩散 | **不投递** | 仅写入 | 不更新 |

**关键设计决策：**

1. **sync_events 对所有人都写入**——即使是中群和大群。`sync_events` 是用户离线恢复的唯一事实源，不能省。
2. **conversation_summary 在中群时不更新**——这是最大头的写入节约。离线成员上线时按需计算 summary。
3. **fanout.resolveTargets 使用 `group_members` 而非 `conversation_members`**——因为成员退出时 `conversation_members` 被删除，而我们需要知道"消息发出时的成员集合"。

### 热点群保护

```
判定: 60s 窗口内消息 > 50 条 → 标记为热点群
Redis Key: hot_group:{group_id} (Sorted Set, TTL 120s)

保护措施:
  1. 写入限流: fanout.Fanout() 返回 ErrGroupBusy → outbox consumer 丢弃不重试
  2. 消息仍然已写入 messages 表 → 客户端通过 sync 补拉
  3. 热点群恢复后（窗口内消息 < 50）→ 自动恢复实时投递
```

**热点群的降级语义：**
- 消息**不会丢**——它已经在 `messages` 表中持久化
- 投递**被跳过**——Outbox Consumer 看到 `ErrGroupBusy` 直接 return nil
- 客户端**通过 sync 补拉**——下次增量同步时拿到消息

## 替代方案及取舍

| 方案 | 写入复杂度 | 读取复杂度 | 群人数上限 | LiveChat 选择 |
|------|-----------|-----------|-----------|--------------|
| 全写扩散 | O(N) per msg | O(1) | ~200 | 小群 ✅ |
| 在线投递 + 离线仅 sync | O(online) + O(N) for sync | O(1) | ~500 | 中群 ✅ |
| 纯读扩散 | O(1) | O(N) | 无上限 | P1 保留 |
| 独立分区 | O(1) 集群内 | O(1) | 无上限 | P1（详见 Spec 11） |

## 踩坑记录

1. **fanout 和 group_members 的竞态**：用户刚被移除群但 Outbox Consumer 正在处理一条之前的群消息。Phase 2 实现中 fanout 不检查"目标是否仍在群内"——消息投递给了已退群用户。客户端按 `conversation_id` 匹配本地 is_hidden 状态自行过滤。服务端不在投递侧做过滤（成本太高）。
2. **Redis Sorted Set 的清理**：`hot_group:{gid}` 的 Sorted Set 用 ZADD + ZREMRANGEBYSCORE + EXPIRE 三个命令，不是原子的。P0 接受这个竞态窗口（60s 计数近似即可）。
3. **Pipeline 查在线状态**：200 人群如果用逐个 Redis GET 会导致 200 次 round-trip。`resolveTargets` 应在批次中查询，但当前实现是逐个成员的——这是 P1 优化点。

### 代码位置

- `internal/fanout/service.go` → resolveTargets（群成员 vs 1:1 查成员路由）
- `internal/fanout/service.go` → Fanout（分级逻辑 + 热点群检测）
- `internal/fanout/service.go` → isHotGroup, trackGroupMessage（滑动窗口）
- `cmd/outbox-consumer/main.go` → message_created handler（ErrGroupBusy 处理）
