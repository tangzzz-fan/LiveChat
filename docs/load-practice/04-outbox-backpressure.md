# 04 — Outbox 背压（消费侧积压）

## 1. 问题是什么

Consumer 停/慢 → `outbox_events` pending 线性涨 → 实时投递与 sync 投影滞后。消息已 Accepted，但用户体感「发了对方收不到」。

## 2. 业界常见方案

- 消费并行 / lease / 重试（本仓已有）  
- **生产者背压**（pending 超阈拒绝新写）  
- 降级：只保持久，实时可延迟

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| Consumer lease/retry | Implemented | `internal/outbox/consumer.go` |
| pending/lag metrics | Implemented | `:8082/metrics` |
| 发送侧 429 | Missing → **0032** | — |
| 演练 | Implemented | `docs/chaos/02-outbox-backpressure.md` |

## 4. 如何模拟

```bash
export CHAT_ENV=dev
bash livechat-server/scripts/chaos/outbox-pause.sh

# 另一终端继续发消息
cd load_test && python run.py --scenario send_message --quick

# 观察 pending 上涨后恢复
bash livechat-server/scripts/chaos/outbox-resume.sh
bash livechat-server/scripts/chaos/health-check.sh
```

0032 完成后：同一注入下对比「仍 200」vs「超阈 429」。

## 5. 观察什么

| 信号 | 预期（当前，无 0032） |
|------|----------------------|
| `outbox_pending_count` | 单调上升 |
| `outbox_lag_seconds` | 上升 |
| send HTTP | 仍约 200 |
| WS 实时 | 停滞；恢复后追平 |

## 6. 通过标准

- 暂停期间写入的消息在 resume 后全部消费、无静默丢失  
- 复盘填写 [`../chaos/_postmortem-template.md`](../chaos/_postmortem-template.md)  
- （0032 后）超阈出现 429 + Retry-After  
