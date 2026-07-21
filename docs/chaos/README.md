# Chaos Engineering — 故障演练

Phase 3 P0 交付物：6 个典型故障场景的演练手册 + 注入/恢复脚本 + 系统健康检查脚本。

## 演练手册

| 编号 | 场景 | 手册 | 风险等级 |
|------|------|------|----------|
| 01 | Redis 不可用 | [手册](01-redis-outage.md) | 低 |
| 02 | Outbox Consumer 堵塞 | [手册](02-outbox-backpressure.md) | 低 |
| 03 | DB 主库宕机（模拟） | [手册](03-db-primary-failover.md) | 中 |
| 04 | 推送服务延迟/不可用 | [手册](04-push-delay.md) | 低 |
| 05 | 单网关节点宕机 | [手册](05-gateway-pod-failure.md) | 中 |
| 06 | 热点群消息洪峰 | [手册](06-hot-group-flood.md) | 中 |

## 首次演练推荐

从 `01-redis-outage` 开始——它最可控、不涉及数据丢失、恢复最快。

## 注入/恢复脚本

脚本位于 `livechat-server/scripts/chaos/`：

```
redis-down.sh     → 停止 Redis
redis-up.sh       → 恢复 Redis
outbox-pause.sh   → 暂停 outbox-consumer 进程
outbox-resume.sh  → 恢复 outbox-consumer 进程
db-pause.sh       → 暂停 PostgreSQL
db-resume.sh      → 恢复 PostgreSQL
gateway-kill.sh   → 终止 gateway 进程
health-check.sh   → 系统状态校验
```

所有脚本包含环境检查：只在 `CHAT_ENV=dev` 时执行。

## 复盘模板

见 `_postmortem-template.md`。模板关注 3 个问题：
1. 预期 vs 实际：哪些行为符合预期？哪些偏离了？
2. 发现的新问题：演练中暴露了哪些规格/代码/配置中的 gap？
3. 行动项：需要改什么？
