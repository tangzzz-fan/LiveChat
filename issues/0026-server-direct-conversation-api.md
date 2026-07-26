---
id: "0026"
title: "服务端 1:1 建会话 API"
status: complete
labels: ["complete", "p1"]
parent: "0021"
blocked_by: []
created_at: 2026-07-21
---

# 0026 — 服务端 1:1 建会话 API

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)（解锁 iOS 私聊体验；也可独立服务端改进）

## What to build

提供创建 direct conversation 的 HTTP API，避免 iOS 用「两人小群」workaround。端到端：已登录用户 A 指定 peer 用户 B → 获得稳定 `conversation_id` → 双方均可发消息且成员校验通过；重复调用同一对用户返回同一会话（幂等）。

## Acceptance criteria

- [x] `POST /v1/conversations/direct`（需 JWT），body `{ "peer_user_id": <int64> }` → 200 `{ conversation_id, type, peer_user_id, created }`
- [x] 幂等：同一无序用户对只对应一个 direct conversation；从任一侧调用结果相同
- [x] 写入 `conversations`（type=direct）与双方 `conversation_members`（单事务）
- [x] 错误语义：peer 不存在 → 404；peer 等于自己 → 400；缺 `peer_user_id` → 400；未鉴权 → 401
- [x] 集成测试覆盖：创建、重复创建、反向创建、双方发消息成功、非成员 403、ID 推导规则
- [x] [API 参考 §3.2](../docs/API参考.md) 已补文档并删除「无 1:1 API」缺口说明

## Blocked by

None — can start immediately（可与 0022–0025 并行）。

## 技术难点与注意事项

- 会话 ID 生成策略需稳定可复现（例如规范化 user_id 对后哈希/排序拼接），避免双端各建一条。
- 与群会话 ID 命名空间区分（现有 `conv_grp_` 前缀）。

## 实施记录（2026-07-26）

**关键决策：ID 推导而非生成**

`conv_dm_<小 id>_<大 id>`（`conversations.DirectConversationID`）。这一个选择同时解决三件事：

- **幂等不需要查表**：主键天然去重，`ON CONFLICT DO NOTHING` 即可。
- **并发安全**：双端同时打开同一会话时冲突在主键上，不会产生两条会话。
- **顺序无关**：排序后拼接，A→B 与 B→A 落到同一行。

长度上限：`conv_dm_` + 两个 int64 + 分隔符 = 47 字符，在 `VARCHAR(64)` 内。

**会话列表可见性**

只为调用方写 `conversation_summaries` 行。若两边都写，对端会看到一个空会话——空会话没有可展示内容，应等第一条消息由摘要投影补上。这与建群只为创建者初始化摘要的行为一致。

**实测（本机）**

A(1135) 打开与 B(1136) 的会话 → `conv_dm_1135_1136`, `created=true`；B 反向调用 → 同一 ID, `created=false`；双方发消息均 200；第三方发送 403；B 的会话列表在首条消息后出现该会话且 `unread_count=1`。

**踩坑：验证脚本里的 `$RANDOM`**

zsh 中两个子 shell 会拿到相同的 `$RANDOM` 种子副本，`A=$(reg); B=$(reg)` 生成了同一个手机号，两个"用户"实际是同一人，于是接口正确地报了「不能和自己建会话」。造多用户数据时用时间戳而非 `$RANDOM`。
