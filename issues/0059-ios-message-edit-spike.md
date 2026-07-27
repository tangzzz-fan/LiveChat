---
id: "0059"
title: "iOS/服务端 已发消息编辑：协议 spike（先文档）"
status: ready-for-agent
labels: ["ready-for-agent", "p2", "spike"]
blocked_by: []
created_at: 2026-07-27
---

# 0059 — 消息编辑协议 Spike

## Context

仓库当前**无** message edit / recall 服务端 API 或 sync 事件。客户端若先做「编辑」UI 会产生假能力。本票仅做调研与规格草案，**不实现客户端编辑发布路径**。

## What to build（文档）

- 对照 Spec 02 / 04 / 06：是否引入 `message_edited` 事件、是否改 content 原地、seq 语义
- 草案：HTTP edit API、幂等、谁可编辑、时限、多端 sync、已读是否重置
- 明确 P0/P1 是否纳入；若暂不实现，在 Spec 13 §9 保持「依赖服务端」并关闭或挂起后续实现票
- 输出：短 ADR 或 `Specs/` 小节补丁 + 可选后续实现票编号预留

## Acceptance criteria

- [ ] 写清「当前无 edit」与推荐事件模型（或明确 wontfix 理由）
- [ ] Spec / ADR / engineering-problems 至少一处落档
- [ ] 不交付误导性的客户端「编辑已发送」完整链路

## Out of scope

客户端编辑 UI、服务端完整实现（另开实现票）

## Related

- Spec 02 生命周期 · Spec 06 多端 · Spec 13 §9
