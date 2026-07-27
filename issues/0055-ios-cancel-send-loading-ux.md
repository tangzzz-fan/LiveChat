---
id: "0055"
title: "iOS 发送中 loading + 取消发送（未 accepted 前）"
status: ready-for-agent
labels: ["ready-for-agent", "p1"]
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

- 己方 `queued` / `sending` 气泡展示发送中指示（如小 spinner / ProgressView），与 `accepted+` 状态符并存策略写清
- Domain：扩展 `MessageStatus`（建议 `cancelled` 终态）及合法转移：`queued|sending → cancelled`；**禁止** `accepted+ → cancelled`
- `MessageSendExecutor`：按 `clientMessageID` 可取消 in-flight Task；取消后更新本地状态，队列跳过该条
- Presentation：发送中气泡可点「取消」；工具栏整队「重试」行为不变
- 单测：取消后不再发 HTTP；已 `accepted` 不可取消

## Acceptance criteria

- [ ] `queued`/`sending` 气泡有明确 loading，弱网长时间停留可感知
- [ ] 用户取消后：in-flight 被取消或结果被忽略；本地为 `cancelled`（或等价删除）且不自动续跑
- [ ] 与 0046 超时收回共存：未取消的 `sending` 仍可超时→`queued`
- [ ] Spec 13 / study-guide / high-load #5 补一句取消语义
- [ ] Domain 状态机测试覆盖新转移

## Related

- `MessageSendExecutor` · `SendPathResumeMonitor` · `MessageStatus` · `ChatViews` statusGlyph
