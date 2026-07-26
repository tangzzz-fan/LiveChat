---
id: "0032"
title: "发送侧背压：outbox pending 超阈返回 429"
status: open
labels: ["ready-for-agent", "p0"]
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

- [ ] message-service 在发送路径检查 outbox 积压指标（pending count 或 lag；阈值可配置，默认值写入文档）
- [ ] 超阈时 `POST /v1/messages/send` 返回 HTTP 429，响应含 `Retry-After`（秒）
- [ ] 未超阈时行为不变；幂等键逻辑不受误伤（已 accepted 的重放仍返回既有结果）
- [ ] 指标/日志可观测背压触发次数（如 `send_backpressure_total` 或等价日志）
- [ ] 单元或集成测试覆盖：超阈 → 429；恢复后 → 200
- [ ] 更新 `docs/load-practice/`（或 chaos 02）对照演练：有/无背压的 pending 曲线与客户端重试语义
- [ ] 更新 API 参考中 send 的 429 说明

## Blocked by

- [0031](0031-harden-load-test-chaos.md) — 建议先能稳定打出 pending 上涨场景，再做对照；若 0031 未完成但 chaos 02 已可注入，可提前实现并在复盘中注明。

## 技术难点与注意事项

- 阈值过低会误伤正常突发；过高则失去保护。默认偏保守并文档化调参。
- 检查积压的查询不能成为新热点：可用缓存计数 / 周期采样，避免每次 send 全表 count。
- 客户端（含未来 iOS）应对 429 做退避；本票服务端语义优先，客户端实现属后续票。
- 可选后续（不阻塞本票）：幂等窗口缓存、把 `internal/cache` 接到热路径。
