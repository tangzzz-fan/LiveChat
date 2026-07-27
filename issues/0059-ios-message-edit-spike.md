---
id: "0059"
title: "iOS/服务端 已发消息编辑：协议 spike（先文档）"
status: complete
labels: ["done", "p2", "spike"]
blocked_by: []
created_at: 2026-07-27
---

# 0059 — 消息编辑协议 Spike

## Context

仓库当前**无** message edit / recall 服务端 API 或 sync 事件。客户端若先做「编辑」UI 会产生假能力。本票仅做调研与规格草案，**不实现客户端编辑发布路径**。

## What to build（文档）

- [x] ADR：[0005-message-edit-deferred-event-model](../docs/adr/0005-message-edit-deferred-event-model.md)
- [x] Spec 13 §9 标注暂缓
- [x] 推荐：`message_edited` 事件、HTTP edit、seq 不变、已读不重置；P0/P1 不实现

## Acceptance criteria

- [x] 写清「当前无 edit」与推荐事件模型
- [x] Spec / ADR 落档
- [x] 不交付误导性的客户端「编辑已发送」完整链路

## Out of scope

客户端编辑 UI、服务端完整实现（建议后续 0065/0066）

## Related

- Spec 02 · Spec 06 · Spec 13 §9 · ADR 0005
