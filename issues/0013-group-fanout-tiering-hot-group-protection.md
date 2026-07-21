---
id: "0013"
title: "群消息扇出 + 分级策略 + 热点群保护"
status: complete
labels: ["done"]
parent: "0010"
blocked_by: ["0012"]
created_at: 2026-07-20
updated_at: 2026-07-21
---

# 0013 — 群消息扇出 + 分级策略 + 热点群保护

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

在 Phase 1 的 1:1 消息投递链路上扩展群消息路径：Outbox consumer 处理群消息事件时，查询群成员列表进行写扩散；按群规模分级（小群全写扩散、中群在线投递+离线只写 sync）；热点群自动启用频控、批量和去抖动。

端到端行为：在小群（≤50 人）中发文本消息 → 除发送者外，在线成员通过 WebSocket 实时收到 MESSAGE_DELIVERY 帧 → 所有成员（含发送者）写入 sync_event → conversation_summary 对除发送者外的成员更新。在中群（51-200 人）中发消息 → 在线成员实时收到 → 离线成员只写 sync_event，不在此时更新 summary。热点群（1 分钟内消息 > 50 条）→ 服务端启用频控，发送接口返回 `429 "群聊繁忙"`，投递合并为批次（每 500ms 一批），摘要更新去抖动（每 2s）。

## Acceptance criteria

- [ ] 小群（≤50 人）消息：除发送者外的在线成员实时收到 WebSocket 投递，所有成员（含发送者）写入 sync_event
- [ ] 中群（51-200 人）消息：在线成员实时收到 WebSocket 投递，离线成员只写 sync_event，当前不更新 conversation_summary（按需计算）
- [ ] 群成员退出或被移除后，不再出现在后续消息的扇出目标中
- [ ] 群消息的 conversation_summary 更新：对除发送者外的成员，更新 `last_message_preview`、`last_message_at`、`unread_count + 1`
- [ ] 群消息的 ConversationSummary 包含 member 列表，客户端可据此展示群头像/名称
- [ ] 热点群：1 分钟窗口内消息 > 50 条 → Redis key `hot_group:{group_id}` TTL 60s → 发送接口返回 `429 {"error_code": "group_busy", "retry_after_ms": 2000}`
- [ ] 热点群：投递降级为批量模式（每 500ms 合并一次，不是逐条投递）
- [ ] 热点群：conversation_summary 更新去抖动到每 2 秒一次
- [ ] 非热点群恢复正常后，自动切回逐条实时模式
- [ ] 群消息链路保持 Phase 1 的 `Message` 生命周期和 `MessageReceipt` 语义不变
- [ ] 限流不影响同一用户的 1:1 消息发送（热点群保护只作用在群消息路径）

## Blocked by

- [0012 - 群会话创建 + 成员管理 + 群事件投影](0012-group-conversation-membership-events.md)

## 技术难点与注意事项

### 1. fanout.Service 从 1:1 到 1:N 的重构

**问题：** Phase 1 的 `fanout.Service.Fanout()` 通过 `conversation_members` 找到"对方 user_id"（1:1 场景只有 1 个对端）。群聊场景需要枚举 N 个成员。

**方案：**
- 抽取成员枚举逻辑：`func (s *Service) resolveTargets(ctx context.Context, conversationID string, senderUserID int64) ([]TargetMember, error)`
- 1:1 会话：查询 `conversation_members WHERE user_id != senderUserID`
- 群会话：查询 `group_members WHERE group_id = $gid AND left_at IS NULL AND user_id != senderUserID`
- 对每个 target member，查询 `devices` 表获取在线设备列表（通过 Redis 路由表判断在线状态）

**坑点：** 不要用 `conversation_members` 代替 `group_members` 查群成员——前者在 Phase 1 只有"当前在会话中的人"语义，且 RemoveMember 时已从 conversation_members 中删除。务必以 `group_members LEFT JOIN ... WHERE left_at IS NULL` 为权威。

### 2. 分级策略的阈值配置

**问题：** Spec 07 定义了 ≤50 / 51-200 的分级。这些数字应该是可配置的。

**方案：** 在 config 中增加：
```
fanout.small_group_threshold: 50
fanout.medium_group_threshold: 200
```
P0 硬编码默认值，不引入配置文件变更（后续 ticket 统一做）。代码中用常量 `SmallGroupThreshold = 50`。

### 3. 在线设备查询的性能

**问题：** 200 人群 × 每成员 2 设备 = 400 次 Redis 查询。如果每次发送都逐个查 Redis，延迟不可接受。

**方案：** 使用 Redis Pipeline 批量查。Go 的 `redis.Client.Pipelined()` 可以将 N 个 `GET` 命令一次 round-trip 发送。

伪代码：
```
pipe := rdb.Pipeline()
cmds := make([]*redis.StringCmd, len(members))
for i, m := range members {
    cmds[i] = pipe.Get(ctx, "user_route:"+strconv.FormatInt(m.UserID, 10))
}
pipe.Exec(ctx)
```

**坑点：** Pipeline 中的单个 Key 不存在会返回 `redis.Nil`，不等于 pipeline 失败。需要逐个检查 `cmds[i].Err()`。

### 4. Write Amplification —— sync_events 的批量插入

**问题：** 200 人群发一条消息需要写入 199 条 sync_event。逐条 INSERT 会导致 199 个 round-trip。

**方案：** 使用 PostgreSQL `COPY` 或 `unnest` 批量插入：
```
INSERT INTO sync_events (user_id, conversation_id, event_seq, event_type, payload, created_at)
SELECT * FROM unnest($1::bigint[], $2::text[], $3::bigint[], $4::text[], $5::jsonb[], $6::timestamptz[])
```
Go 侧用 `pq.Array()` 构建参数数组。

**坑点：** `event_seq` 需要为每个 user 单独递增，不能全局共用一个序列。用 `nextval` 不行——需要用窗口函数或应用层分配。P0 简化：为每个 user 单独 `SELECT nextval('sync_events_seq')` 分配 batch 大小的 seq 范围，然后应用层分配。

### 5. 热点群检测的 Redis 实现

**问题：** 需要在一个滑动窗口内计数，不能用简单的 counter + TTL（counter 在 TTL 刷新后会归零）。

**方案：** 使用 Redis Sorted Set 实现滑动窗口计数器：
```
ZADD hot_group:{group_id} {timestamp_ms} {message_id}
ZREMRANGEBYSCORE hot_group:{group_id} 0 {now - 60000}
ZCARD hot_group:{group_id}  # 得到最近 60 秒的消息数
EXPIRE hot_group:{group_id} 120  # 2 分钟无新消息自动清除
```
如果 ZCARD > 50 → 触发热点保护。

**坑点：** 这个操作本身有 3 个 Redis 命令，不是原子的。P0 可以接受竞态窗口（60s 的计数近似即可）。更精确的做法用 Lua script。

### 6. 不使用 Redis 的降级路径

**问题：** Redis 不可用时，热点群检测和在线状态查询都失效。

**方案：** 
- 在线状态降级：Redis 不可用时，假设全部设备离线（只写 sync_event，不尝试投递）。消息不会丢，客户端通过增量同步补拉。
- 热点群降级：Redis 不可用时，跳过热点检测，按正常路径处理。极端情况下群消息可能压垮 fanout，但这是 Redis 故障时的合理取舍。

### 7. 涉及的关键文件

- `internal/fanout/service.go` — 重构 resolveTargets，接群成员枚举，加分级逻辑
- `internal/outbox/consumer.go` — 群消息 Outbox 事件路由（已在 Phase 1 有 pending 逻辑）
- `internal/domain/types.go` — 补 GroupMessage 结构体
- `configs/` — 扇出阈值配置项
- `internal/fanout/service_test.go` — 新增分级策略与热点群单测
