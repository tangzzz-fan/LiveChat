---
id: "0055"
title: "iOS 发送中 loading + 取消发送（未 accepted 前）"
status: complete
labels: ["done", "p1"]
blocked_by: ["0046"]
created_at: 2026-07-27
---

# 0055 — 发送中 loading 与取消发送

## Context

弱网续跑与 `sending` 超时已在 [0046](0046-ios-weak-network-send-hardening.md) 落地，但气泡侧无明确 loading，也无用户主动「取消发送」。本票补齐产品交互，并与超时收回划清边界：

- **超时收回（0046）**：系统行为，`sending` → `queued` 再发。
- **取消发送（本票）**：用户行为，中断 in-flight 请求，本地收敛为取消态（或删除），**不再自动重试**。

参考：Spec 13 §5.1–5.2、§9；`docs/ios-high-load-client.md` #5。

## What to build

- [x] 己方 `queued` / `sending` 气泡展示 spinner +「取消」
- [x] Domain：`MessageStatus.cancelled`；`queued|sending → cancelled`；禁止 `accepted+ → cancelled`
- [x] `MessageSendExecutor.cancelSend`；超时 CancellationError → queued；用户取消 → cancelled
- [x] 长按菜单「取消发送」；工具栏整队「重试」不变
- [x] 单测：状态机 + 取消后保持 cancelled

## Acceptance criteria

- [x] `queued`/`sending` 气泡有明确 loading
- [x] 用户取消后本地为 `cancelled` 且不自动续跑
- [x] 与 0046 超时收回共存
- [x] Spec 13 / study-guide / high-load #5 补取消语义
- [x] Domain 状态机测试覆盖新转移

## Related

- `MessageSendExecutor` · `MessageStatus` · `ChatViews`
