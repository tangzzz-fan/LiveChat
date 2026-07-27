---
id: "0044"
title: "iOS UI 基础：会话列表 + 消息窗按 conversation_seq + 分页"
status: complete
labels: ["ready-for-agent", "p0"]
parent: "0043"
blocked_by: ["0043"]
created_at: 2026-07-27
---

# 0044 — UI 列表基础（高负载前置）

## Parent

[0043](0043-ios-high-load-leftover.md)

## Why first

高负载 #2 / #10 / 后续 ValueObservation 与图片行都依赖「Store 只持可见窗、按 `conversation_seq` 排序、可向上翻页」。当前 `fetchMessages(... order created_at limit 200)` 不够。

## What to build

- 会话列表：继续 GRDB `conversation_summaries` 投影；打开会话时加载**最新一页**（按 `conversation_seq` DESC 取再正序展示，或 ASC + 锚点）。
- 消息窗：`visibleMessages` 仅窗口；上拉/按钮「加载更早」用 `(conversation_id, before_seq)` 分页。
- 排序真相：`conversation_seq`（缺 seq 的本地 queued 可用本地规则置底/置顶，需文档化）。
- 保留索引 `idx_messages_conversation_seq`。

## Acceptance criteria

- [x] 聊天列表按 `conversation_seq` 升序展示（同会话内）
- [x] 历史超过一页时可加载更早消息，Store 不灌全表
- [x] 新消息（发/收）进入窗口后顺序仍正确
- [x] 文档：在 `ios-high-load-client.md` 标注 #2/#10 本票覆盖范围

## 实现备注

- GRDB：`fetchLatestMessageWindow` / `fetchOlderMessages` / `fetchMessageWindow(fromSeq:)`；pending（无 `conversation_seq`）置底。
- UI：`ChatState.oldestLoadedSeq` + `hasMoreOlder`；线程内「加载更早消息」。

## Blocked by

- [0043](0043-ios-high-load-leftover.md)（范围确认；实现时可与父票同时 `in-progress`）

## Backend

**不阻塞。** 可见窗可先吃本地 GRDB（sync/WS 已写入）。冷开最新页：`GET /v1/conversations/{cid}/messages` 响应含 `latest_seq`，再 `from_seq = max(1, latest_seq - page + 1)`；更早页用更小的 `from_seq`（无需 `before_seq`）。
