---
id: "0032"
title: "发送侧背压：outbox pending 超阈返回 429"
status: complete
labels: ["complete", "p0"]
parent: "0029"
blocked_by: ["0031"]
created_at: 2026-07-26
---

# 0032 — 发送侧 Outbox 背压（429）

## Parent

[0029 - 高负载 IM 验证](0029-high-load-im-validation.md)

## What to build

在 Outbox 积压可观测（chaos 02）的基础上，补生产侧背压：当 `outbox_pending`（或 lag）超过阈值时，`POST /v1/messages/send` 返回 **429** 并带 **Retry-After**，避免积压时仍无限写入。

端到端：暂停/拖慢 outbox-consumer → pending 上涨越过阈值 → 新发送返回 429 → 恢复 consumer → pending 下降后发送恢复 200；手册中有「无背压 vs 有背压」对照说明。

## Acceptance criteria

- [x] message-service 在发送路径检查 outbox 积压指标（`internal/backpressure` 后台采样 `pending + processing`；阈值 `SEND_BACKPRESSURE_PENDING_THRESHOLD`，默认 2000）
- [x] 超阈时 `POST /v1/messages/send` 返回 HTTP 429，响应含 `Retry-After`（秒）与 `code=outbox_backpressure`
- [x] 未超阈时行为不变；429 在写入前返回，不产生任何行，重试沿用同一 `client_message_id` 走正常幂等路径
- [x] 指标可观测：`send_backpressure_rejected_total` / `_pending_sample` / `_threshold`，另有 WARN 日志带 trace_id
- [x] 测试覆盖：`internal/backpressure`（阈值/恢复/禁用/配置）+ `internal/api`（429 + `Retry-After` + 未写入 + 恢复 200 + 无 limiter 时行为不变）
- [x] [chaos 02](../docs/chaos/02-outbox-backpressure.md) 加入三阶段对照演练与本机实测数字
- [x] [API 参考](../docs/API参考.md) 补 send 的 429 语义与客户端退避要求

## Blocked by

- [0031](0031-harden-load-test-chaos.md) — 建议先能稳定打出 pending 上涨场景，再做对照；若 0031 未完成但 chaos 02 已可注入，可提前实现并在复盘中注明。

## 技术难点与注意事项

- 阈值过低会误伤正常突发；过高则失去保护。默认偏保守并文档化调参。
- 检查积压的查询不能成为新热点：可用缓存计数 / 周期采样，避免每次 send 全表 count。
- 客户端（含未来 iOS）应对 429 做退避；本票服务端语义优先，客户端实现属后续票。
- 可选后续（不阻塞本票）：幂等窗口缓存、把 `internal/cache` 接到热路径。

## 实施记录（2026-07-26）

**设计取舍**

- 采样而非实时查询：后台每 `SEND_BACKPRESSURE_SAMPLE_MS`（默认 2s）做一次 `COUNT(*)`，send 路径只读 atomic。代价是样本最多滞后一个间隔，对「方向正确即可」的信号可以接受，避免把积压检查本身变成新热点。
- **采样失败时 fail open**：查询出错只记 WARN，不阻塞发送。监控挂掉不应该等于业务挂掉。
- 阈值 `<= 0` 表示关闭，默认 2000 偏保守。
- 用可选参数 `api.WithSendLimiter()` 接入 router，避免为新增一个依赖去改 26 处已有调用点，同时保证未接入时行为完全不变。

**实测对照（阈值 50，见 chaos 02）**

| 阶段 | send | pending |
|------|------|---------|
| consumer 正常 | 167 rps，err 0.6% | 0–8 |
| consumer 暂停 | 95.2% 返回 429 | 停在 88 |
| consumer 恢复 | 82 rps，err 0.2% | 回到 0 |

**顺带修掉的两个坑**

- `scripts/chaos/outbox-pause.sh` 用 `pgrep -f` 会命中 shell 包装进程，SIGSTOP 停错对象、consumer 照常消费，导致演练无效。改为 `pgrep -x` 匹配二进制。
- `scripts/chaos/health-check.sh` 的 `check()` 把标签当命令执行（`"$@"` 未 shift），7 项检查全部误报失败。已修并加入背压指标输出。
