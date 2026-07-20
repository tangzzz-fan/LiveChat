---
id: "0012"
title: "群会话创建 + 成员管理 + 群事件投影"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: []
created_at: 2026-07-20
---

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

落地 `Spec 07` 的群数据模型与成员变更事件流，让群聊先具备“能创建、能加人、能退群、能踢人、能在会话列表出现”的基础闭环。

端到端行为：User A 创建一个群 `Conversation` 并邀请 User B、User C → 三个人都能在会话列表看到这个群 → A 添加新成员时，群内其他成员能通过同步或实时事件看到“某成员已加入” → 被移除成员的群会话被隐藏且不能继续向该群发消息。

具体交付：

- `groups`、`group_members`、`group_events` 表及对应领域类型
- `POST /v1/groups` 创建群，同时原子创建对应 `Conversation`
- `POST /v1/groups/{gid}/members` 添加成员
- `POST /v1/groups/{gid}/leave` 退群
- `POST /v1/groups/{gid}/remove` 移除成员
- 成员变更写入 `group_events` 并投影到会话列表 / 同步事件

## Acceptance criteria

- [ ] 创建群成功后，`groups`、`group_members`、`conversation_members` 三处数据一致
- [ ] 群创建者默认拥有 `owner` 角色
- [ ] 添加成员后，新成员能在自己的会话列表或同步结果中看到该群
- [ ] 退群后，该成员的群会话被标记为隐藏或不可继续发送消息
- [ ] 被移除成员继续向该群发消息时返回 `403`
- [ ] 每次成员加入、退出、移除都写入一条 `group_events`
- [ ] 非 owner/admin 调用加人或踢人接口时返回 `403`

## Blocked by

None - can start immediately.
