# 0005: 已发消息编辑 — 暂缓实现，先定事件模型

## 状态

已采纳（2026-07-27）— **P0/P1 不实现客户端编辑发布路径**；协议草案如下，实现另开票。

## 背景

仓库当前**无** message edit / recall HTTP API，也无 `message_edited` sync/WS 事件。若客户端先做「编辑已发送」UI，会形成假能力（仅改本机投影，对端与多端不一致）。

对照 Spec 02（生命周期）、04（发送主链路）、06（离线同步）：消息一旦 `accepted` 即带不可变 `server_message_id` + 单调 `conversation_seq`。编辑若原地改 `content` 而不发新事件，离线端与已拉窗端无法收敛。

## 决策

1. **现阶段：不实现编辑/撤回端到端能力**（Spec 13 §9 保持「依赖服务端；先 spike」）。
2. **若未来做编辑，推荐事件模型**（非原地静默改库）：
   - HTTP：`POST /v1/messages/{server_message_id}/edit`，body `{ client_edit_id, content }`；幂等键 `(editor_user_id, client_edit_id)`。
   - 权限：仅原 `sender_user_id`；可选时限（如 15 分钟）。
   - 存储：`messages.content` 更新为最新正文；另写 `message_edits` 审计或在 sync payload 带 `edited_at`。
   - 扇出：Outbox `message_edited`（含 `server_message_id`, `conversation_id`, `conversation_seq` **不变**, `content`, `edited_at_ms`）。
   - Seq 语义：**不**分配新 `conversation_seq`（编辑不是新消息）；会话摘要 preview 若指向该消息则更新。
   - 已读：不重置 `last_read_seq`；UI 显示「已编辑」标记即可。
3. **撤回（recall）另议**：更接近 tombstone / `message_recalled`，与编辑不同票。

## 理由

- 无服务端事件则多端/离线无法对齐，违反「单一可信源」。
- 学习阶段优先保证发送/投递/已读正确性；编辑属用户可感知增强但非正确性骨架。
- Spike 先锁模型，避免客户端过早绑定错误假设（例如「只改本地 content」）。

## 影响

- 客户端：不提供「编辑已发送」完整链路；菜单可暂不出现「编辑」。
- 后续实现票建议：`0065`（服务端 edit API + outbox）→ `0066`（iOS 应用 `message_edited`）。

## 相关

- Ticket [0059](../../issues/0059-ios-message-edit-spike.md)
- Spec 02 / 04 / 06 / Spec 13 §9
