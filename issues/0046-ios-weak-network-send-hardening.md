---
id: "0046"
title: "iOS 弱网发送硬化：path 恢复续跑 + sending 超时"
status: complete
labels: ["p0"]
parent: "0043"
blocked_by: []
created_at: 2026-07-27
---

# 0046 — 弱网发送硬化

## Parent

[0043](0043-ios-high-load-leftover.md)

## What to build

对齐高负载 #5：

- `NWPathMonitor`（或复用 Realtime 监控信号）在路径恢复时触发 `MessageSendExecutor.processPending`
- `sending` 超时 → 回 `queued` 或 `failed`（策略写清，避免永久卡 sending）
- 与 429 退避共存，不形成本地重试风暴

## Acceptance criteria

- [x] 断网发消息留在 queued/sending；恢复网络后自动或可预期地续跑至 accepted
- [x] sending 超时可收敛（可测：人为挂起 API）
- [x] 文档更新 `ios-high-load-client.md` #5

## 实现备注

- `SendPathResumeMonitor`：独立 path 监控，恢复时 `reclaimStaleSendingAndProcess`
- sending 超时 30s / 孤儿 sending（无内存计时）→ **queued**（允许 `sending→queued`）
- API 调用 `withThrowingTaskGroup` 超时；429 仍串行退避，不叠风暴

## Blocked by

无
