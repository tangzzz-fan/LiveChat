# 05 — Redis 降级

## 1. 问题是什么

Redis 不可用：在线路由、OTP 频控、热点标记等依赖失效。正确性应靠 Postgres + Sync 兜底，可用性可能下降。

## 2. 业界常见方案

- 路由缓存失效 → 回源 / 广播 / 降级只走 sync  
- 限流失败默认拒绝或默认放行（需明确语义）  
- 多级缓存与本地兜底

## 3. 本仓现状

| 项 | 状态 | 锚点 |
|----|------|------|
| 演练手册 + 脚本 | Implemented | `docs/chaos/01-redis-outage.md`、`scripts/chaos/redis-*.sh` |
| 路由依赖 Redis | Implemented | gateway / fanout 路径 |
| 离线可 sync 补拉 | Implemented | sync API |

## 4. 如何模拟

```bash
export CHAT_ENV=dev
bash livechat-server/scripts/chaos/redis-down.sh

# 尝试发消息 / WS；预期实时可能失败，持久化与后续 sync 仍可达（以手册为准）
bash livechat-server/scripts/chaos/redis-up.sh
bash livechat-server/scripts/chaos/health-check.sh
```

细节步骤以 [`../chaos/01-redis-outage.md`](../chaos/01-redis-outage.md) 为准。

## 5. 观察什么

| 信号 | 预期 |
|------|------|
| Redis 连接错误日志 | 出现 |
| send 持久化 | 仍应成功（不依赖 Redis 写消息） |
| 实时投递 | 可能失败或降级 |
| 恢复后路由 | 重建 |

## 6. 通过标准

- 符合 chaos 01 验收：消息不因 Redis 宕而静默丢失  
- 恢复后实时路径恢复  
- 复盘记录与预期差异  
