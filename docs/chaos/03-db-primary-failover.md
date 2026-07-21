# 03 — DB 主库宕机（模拟）

## 场景描述

PostgreSQL 主库不可用时，所有依赖事务写入的路径失败（发消息、认证写设备、Outbox）。本地开发为单实例，用停库模拟主库宕机；不验证真实主从切换。

**影响组件：** Message Service、Auth、Outbox Consumer、Sync

## 注入方式

```bash
bash livechat-server/scripts/chaos/db-pause.sh

# 或：brew services stop postgresql@16
```

## 预期系统行为

1. `POST /v1/messages/send` 返回 5xx
2. `POST /v1/auth/*` 写路径失败
3. 已建立的 WebSocket 可能仍存活（不依赖 DB 心跳），但 ACK/回执写库失败
4. Outbox Consumer 消费循环报错、pending 无法推进
5. 恢复后新写入成功；注入前已提交数据仍在（RPO ≈ 0 for committed rows）

**关键验证：** 停库不损坏已提交数据；恢复后无需手工修表即可继续发消息。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| `http_requests_total` 5xx 比例 | 显著上升 |
| `GET /health` | `{"postgres":"error: ..."}` |
| `outbox_pending_count` | 停滞或消费失败日志增多 |

## 恢复步骤

```bash
bash livechat-server/scripts/chaos/db-resume.sh
# 等待 Postgres ready
bash livechat-server/scripts/chaos/health-check.sh
```

## 验收标准

- [ ] 恢复后 10s 内非 5xx 请求恢复
- [ ] `GET /health` 中 postgres 为 ok
- [ ] 注入前已 Accepted 的消息仍可查询
- [ ] 无需 `schema_migrations` dirty 手工修复（若 dirty，记录为 gap）

## 本地限制

单机无主从：本场景验证的是**写失败可感知 + 恢复后可写**，不验证自动 failover / RTO < 30s。
