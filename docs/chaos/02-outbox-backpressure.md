# 02 — Outbox Consumer 堵塞 / 背压

## 场景描述

Outbox Consumer 进程被暂停或处理过慢时，`outbox_events` 积压增长。消息已持久化（Accepted），但实时投递与 sync 投影滞后。

**影响组件：** Outbox Consumer、Fanout、Gateway 实时投递、Sync 事件流

## 注入方式

```bash
# 暂停 outbox-consumer（SIGSTOP，进程仍在但不再调度）
bash livechat-server/scripts/chaos/outbox-pause.sh

# 或者手动：找到 PID 后
# kill -STOP $(pgrep -f outbox-consumer)
```

## 预期系统行为

1. `POST /v1/messages/send` 仍返回 200（消息 + outbox 同行事务写入成功）
2. `outbox_pending_count` / pending 行数持续上升
3. 在线 WebSocket 收不到新消息（Fanout 未消费）
4. 客户端离线同步游标也停滞（sync_events 由 Fanout 写入）
5. Consumer 恢复后追平积压，消息最终可达（最终一致，非实时）

**关键验证：** 暂停期间写入的消息在恢复后全部被消费，无死信、无丢失。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| `outbox_pending_count` | 单调上升 |
| `outbox_lag_seconds` | 上升 |
| `ws` 投递相关计数 | 停滞或骤降 |
| HTTP send 5xx | 应保持接近 0 |

## 恢复步骤

```bash
bash livechat-server/scripts/chaos/outbox-resume.sh
bash livechat-server/scripts/chaos/health-check.sh
```

观察 pending 在恢复后 60s 内下降。

## 验收标准

- [ ] 恢复后 60s 内 `outbox_pending_count` 接近 0（或回到注入前基线）
- [ ] 注入期间发送的消息可通过 sync / 实时投递到达
- [ ] 无死信堆积；重试计数未异常飙升
- [ ] 消息发送 API 在注入期间保持可用
