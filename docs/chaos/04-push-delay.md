# 04 — 推送服务延迟 / 不可用

## 场景描述

离线推送（APNs/Mock）延迟或失败时，用户可能收不到系统通知，但消息真相仍在服务端：在线走 WebSocket，离线走 sync 补拉。

**影响组件：** Push Orchestrator、Outbox Fanout 后的 `NotifyOffline`

## 注入方式

本地无真实 APNs 时，用环境变量注入（实现于 `MockAPNsClient.Send`）：

```bash
export CHAT_ENV=dev
# 打印注入说明
bash livechat-server/scripts/chaos/push-delay-on.sh

# 延迟 5s（示例）
export PUSH_INJECT_DELAY_MS=5000
# 或强制失败
# export PUSH_INJECT_FAIL=1

# 必须重启 outbox-consumer 使环境变量生效
cd livechat-server && make run-outbox-consumer
```

行为级验证（不注入故障）仍可用：接收方不上 WS，确认仅靠 sync 可达。


## 预期系统行为

1. 在线设备：WebSocket 投递不受影响
2. 离线设备：sync_events 仍写入；上线后增量同步可拿到消息
3. 推送失败记入日志 / `push_events`，不回滚消息 Accepted
4. 推送去重窗口（见工程问题 11）仍生效，恢复后不风暴补推

**关键验证：** 推送通道故障 ≠ 消息丢失。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| 推送成功/失败计数 | 失败上升 |
| `outbox_pending_count` | 不应因 push 失败而无限重试堵塞（push 应与 fanout 解耦） |
| sync 补拉可达率 | 100% |

## 恢复步骤

撤销延迟/错误注入，重启 outbox-consumer：

```bash
bash livechat-server/scripts/chaos/push-delay-off.sh
# unset 环境变量后重启 consumer
bash livechat-server/scripts/chaos/health-check.sh
```

## 验收标准

- [ ] 注入期间发送的消息，离线端上线后 sync 可拉到
- [ ] 消息发送 API 不受 push 故障影响
- [ ] 恢复后无异常推送风暴（频控窗口生效）
