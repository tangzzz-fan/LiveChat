# 04 — 推送服务延迟 / 不可用

## 场景描述

离线推送（APNs/Mock）延迟或失败时，用户可能收不到系统通知，但消息真相仍在服务端：在线走 WebSocket，离线走 sync 补拉。

**影响组件：** Push Orchestrator、Outbox Fanout 后的 `NotifyOffline`

## 注入方式

本地无真实 APNs 时，用以下之一：

```bash
# A. 临时让 push provider 指向不可达地址（若配置支持）
#    或在 push orchestrator 中打开 fail-closed 测试开关

# B. 行为级验证（不注入故障）：
#    1. 接收方不下线 WebSocket
#    2. 发送消息
#    3. 确认仅靠 WS/sync 可达，不依赖 push
```

若实现了 mock 延迟注入，可在环境变量中设置例如 `PUSH_INJECT_DELAY_MS=5000` 后重启 outbox-consumer。

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

撤销延迟/错误注入，重启 outbox-consumer（若改了配置），执行：

```bash
bash livechat-server/scripts/chaos/health-check.sh
```

## 验收标准

- [ ] 注入期间发送的消息，离线端上线后 sync 可拉到
- [ ] 消息发送 API 不受 push 故障影响
- [ ] 恢复后无异常推送风暴（频控窗口生效）
