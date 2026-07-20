---
id: "0005"
title: "Outbox 消费者：事件拉取、重试、死信"
status: complete
labels: ["done"]
parent: "0001"
blocked_by: ["0003"]
created_at: 2026-07-20
---

## Parent

[0001 - 阶段一：消息正确性骨架](0001-phase-1-message-correctness-skeleton.md)

## What to build

实现 Outbox Consumer：轮询 `outbox_events` 表中 `pending` 记录 → 按 `event_type` 分发到 handler → 成功后标记 `done` → 失败则退避重试 → 超过 max_retries 进入 dead。

端到端行为：消息发送后 outbox_events 中产生一条 `status='pending'` 的记录 → Consumer 拉取并标记 `processing` → 调用对应的 fanout/sync/push handler（此时 handler 可为空实现，写 log 即可）→ 成功后标记 `done` → 若 handler 返回 error → `retry_count++` + exponential backoff + 重回 `pending` → 超过 10 次标记 `failed`。

具体交付：

- `internal/outbox/consumer.go`：消费者主循环——`fetchPending(limit=100)` → worker pool（默认 4 workers，可配置）→ 逐条处理 → 更新状态。
- `internal/outbox/repository.go`：`FetchPending()`、`MarkProcessing()`、`MarkDone()`、`MarkRetry()`、`MarkFailed()`、`ReapStaleProcessing()`（lease 超时 60s 接管）。
- `internal/outbox/dispatcher.go`：`event_type` → handler 注册表。初始 handler：`message_created` → 写 log + mark done；`delivery_acked` → 写 log + mark done；`read_receipt` → 写 log + mark done。这些 stub handler 在后续 ticket 中被替换为真实实现。
- 轮询间隔：100ms（pending 为空时升至 500ms）。
- 重试退避：`min(1s * 2^retry_count, 30s)` + ±25% 随机 jitter。
- 结构化日志：每条事件处理日志含 `aggregate_id`、`event_type`、`status`、`latency_ms`。

## Acceptance criteria

- [x] 发送消息后 1 秒内，对应的 outbox_event status 从 `pending` → `done`
- [x] 两条消息同时发送，两个事件都被消费（不互斥、不遗漏）
- [x] handler 返回 error 时，`retry_count` 递增，事件重回 `pending`，下次轮询重试
- [x] 重试退避间隔符合指数增长 + jitter（第一次重试 ~1s，第二次 ~2s，以此类推）
- [x] `retry_count` 达到 10 次后，status 变为 `failed`，不再轮询
- [x] `processing` 状态超过 60s 的事件被 `ReapStaleProcessing` 接管并重置为 `pending`
- [x] Consumer 优雅退出（收到 SIGTERM → 完成正在处理的事件 → 关闭 DB 连接 → 退出）
- [x] Consumer 启动时不干扰其他已完成的 processing 事件（只接管超时的）

## Current implementation status

- 已实现：Outbox Consumer 主循环、handler 注册、worker pool、优雅退出框架；事件领取已收敛为原子 `pending -> processing`；handler 失败后按票据语义回到 `pending`；lease 超时的 `processing` 会被接管回 `pending`。
- 已新增实现：worker 在收到取消信号后使用非取消上下文完成 in-flight 事件的最终状态写回，避免“退出过程中事件卡在 processing”。
- 已新增验证：`internal/outbox/consumer_test.go` 已覆盖指数退避上限、原子领取为 `processing`、handler 失败后 `retry_count++` 且回到 `pending`、stale processing 只重置超时事件、`TestProcessEventRetryThenRecoveryMarksDoneWithoutLoss` 对“短暂失败后恢复成功、不丢消息、不进入 failed”的固定证明，以及 `TestProcessEventMarksFailedAfterMaxRetries`、`TestRunProcessesMultipleEventsConcurrentlyWithoutLoss`、`TestRunGracefulShutdownWaitsForInflightEvent` 对 failed/并发消费/优雅退出的固定验证。
- 说明：`delivery_acked` 仍是当前阶段允许的 stub handler，但这不再阻塞本票关闭，因为本票目标是 Outbox 消费语义本身，而不是后续票据中的完整业务处理链。

## Blocked by

- [0003 - 消息发送 API + 幂等写入 + Outbox 事件](0003-message-send-api.md)
