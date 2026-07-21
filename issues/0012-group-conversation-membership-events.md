---
id: "0012"
title: "群会话创建 + 成员管理 + 群事件投影"
status: complete
labels: ["done"]
parent: "0010"
blocked_by: ["0011"]
created_at: 2026-07-20
updated_at: 2026-07-21
---

# 0012 — 群会话创建 + 成员管理 + 群事件投影

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

补齐 Spec 07 群模型中的 `group_events` 写入、`leave` 端点、群事件到 sync_events 的投影，以及新成员入群后的 conversation_summary 初始化。Phase 1 的 `internal/group/service.go` 已有 CreateGroup/AddMembers/RemoveMember/GetMembers，本票在其上补全闭环。

端到端行为：User A 创建群 → 原子创建 `groups` + `Conversation` + 群主 group_member → 写入 group_events(`created`) → User A 添加 B、C → B、C 的 conversation_summary 被初始化（含成员列表） → A 在群中发"成员已加入"系统消息 → 成员退出/被移除后，该成员的群会话标记隐藏（`is_hidden=true`）且不可再向群发消息（403） → 每次成员变更写入 `group_events` 并生成对应 sync_event → 非 owner/admin 加人/踢人返回 403。

## Acceptance criteria

- [ ] `POST /v1/groups` 创建群后，`groups`、`group_members`、`conversation_members` 三表数据一致，且写入 `group_events`（event_type: `created`）
- [ ] 创建者默认 `role='owner'`，owner 不可以被移除
- [ ] `POST /v1/groups/{gid}/members` 添加成员后，新成员的 `conversation_summary` 被初始化（同步写入，不要依赖后台 job）；添加者需为 owner/admin，否则 403
- [ ] `POST /v1/groups/{gid}/leave` 新增端点：主动退群 → `group_members.left_at = NOW()` → `conversation_summary.is_hidden = true` → 该成员不可再向群发消息
- [ ] `DELETE /v1/groups/{gid}/members/{uid}` （RemoveMember）补齐：被移除者的 conversation_summary 标记 `is_hidden=true`；只有 owner/admin 可踢人
- [ ] 被移除/已退群用户向该群发消息时返回 `403`
- [ ] 每次成员变更（joined/left/removed）写入 `group_events` 表和 `sync_events` 表（对所有群内其他成员）
- [ ] 群创建和成员加入后，群内成员通过 `GET /v1/conversations` 能看到该群会话
- [ ] `current_members` 计数在成员加入/退出/移除后保持正确

## Blocked by

- [0011 - 认证收敛 + 设备会话管理 + Push Token 注册](0011-auth-device-sessions-push-token.md)

## 技术难点与注意事项

### 1. group_events → sync_events 的投影时机

**问题：** 成员变更事件（如 "B 加入了群聊"）需要在群内其他成员的同步流中出现，但发送者（操作者）自己不需要收到。这与消息扇出不同——消息扇出发送者不投递（Phase 1 已有），但群事件是对**全体**成员（包括操作者？）。

**WhatsApp 做法：** 操作者自己也收到 `member_joined` 事件，客户端展示为 "你添加了 B"。P0 简化：所有活跃成员（包括操作者）都收到 sync_event。

**实现：**
- `AddMembers` / `RemoveMember` / `LeaveGroup` 在同事务中 `INSERT INTO group_events` + 批量 `INSERT INTO sync_events`（对所有活跃成员）
- sync_event 的 `event_type` 用 `group_member_joined` / `group_member_left` / `group_member_removed`
- 在 domain/types.go 新增对应常量

**坑点：** 不能依赖 Outbox 异步写入 sync_events——Phase 1 的 Outbox 只用于消息，群事件应同步写入，保证成员变更瞬间就能被查询到。

### 2. conversation_summary 的 is_hidden 字段

**问题：** 当前 `conversation_summaries` 表没有 `is_hidden` 列。

**方案：** 新增 migration 加列 `is_hidden BOOLEAN NOT NULL DEFAULT FALSE`。退群/被踢时 `UPDATE conversation_summaries SET is_hidden = true WHERE user_id = $1 AND conversation_id = $2`。会话列表查询加 `WHERE is_hidden = false`。

### 3. 成员变更 vs 消息发送的竞争

**问题：** 用户刚被移除群，但 Outbox consumer 可能正在处理一条该群的消息投递。如果投递时序晚于移除成员，用户可能收到一条"已不在群内"的消息。

**方案（P0 简化）：** 成员移除完成后，该用户的 conversation_summary 已标记 hidden，客户端下次同步时拉不到该群的消息。即使 WebSocket 投递了该群的最后一条消息，客户端按 conversation_id 匹配本地 hidden 状态忽略即可。服务端不在投递侧做过滤（成本过高）。

### 4. Owner 不可被移除的约束实现

**问题：** 当前 `RemoveMember` 通过 `creator_user_id` 判断是不是 owner。但 `groups.creator_user_id` 和 `group_members.role = 'owner'` 是两个字段，可能不一致。

**方案：** 统一使用 `group_members.role = 'owner'` 判断。CreateGroup 时写入 `role = 'owner'`。RemoveMember 时校验 `targetUserID` 的 role 不为 `'owner'`。

### 5. 涉及的关键文件

- `internal/group/service.go` — 补 LeaveGroup、完善 group_events 写入
- `internal/api/router.go` — 新增 `POST /v1/groups/{gid}/leave` 端点
- `internal/conversations/service.go` — 补 is_hidden 过滤 + 初始化新成员的 summary
- `internal/sync/service.go` — 补群事件到 sync_events 的写入
- `internal/domain/types.go` — 补 group_member_* 事件常量
- `migrations/` — conversation_summaries 加 is_hidden 列
- `internal/api/router_integration_test.go` — 补群 CRUD 集成测试
