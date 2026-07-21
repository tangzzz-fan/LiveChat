# 05 — 单网关节点宕机

## 场景描述

Gateway 进程被杀死后，该节点上所有 WebSocket 断开。客户端应按 Spec 05 退避重连；路由键在 Redis 中 TTL 过期或主动注销。

**影响组件：** Gateway、Redis 路由、客户端重连 / Sync

## 注入方式

```bash
bash livechat-server/scripts/chaos/gateway-kill.sh

# 或：kill -9 $(pgrep -f 'cmd/gateway')
```

## 预期系统行为

1. 该节点全部 WS 连接断开
2. Redis 中对应 `route:*` 最终过期或被清理
3. 客户端指数退避 + jitter 后重连（见 `internal/gateway/reconnect.go`）
4. 重连成功后用 `LatestEventSeq` 触发增量 sync，补上断连期间消息
5. Message Service / Outbox 继续工作；离线路径不依赖 Gateway 存活

**关键验证：** 网关宕机不丢已 Accepted 消息；重连后状态收敛。

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| `ws_connections_active` | 骤降到 0（该节点） |
| 客户端重连成功率 | 本地单节点：重启 gateway 后 > 99% |
| sync 请求量 | 重连后短暂上升 |

## 恢复步骤

```bash
# 重新启动 gateway
cd livechat-server && make run-gateway

bash livechat-server/scripts/chaos/health-check.sh
```

## 验收标准

- [ ] Gateway 重启后新握手成功
- [ ] 断连期间发送的消息可被 sync 拉回
- [ ] 无“幽灵路由”导致投递到已死连接（路由 TTL / 注销生效）

## 本地限制（P0）

单机通常只有一个 Gateway：**无法验证跨节点 failover**。本场景验证客户端重连 + sync 补偿。生产多节点时应扩展为：停 Pod A，连接落到 Pod B。
