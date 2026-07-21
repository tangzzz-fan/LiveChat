# 01 — Redis 不可用

## 场景描述

Redis 实例不可用时，Gateway 无法维护新的在线路由，但已建立的 WebSocket 连接不受影响。消息投递降级为纯离线同步路径。

**影响组件：** Gateway（路由注册/查找）、Push Orchestrator（频控窗口）、Auth（验证码存储）、Fanout（在线设备查找）

## 注入方式

```bash
# 停止 Redis
bash livechat-server/scripts/chaos/redis-down.sh

# 或者手动
brew services stop redis
```

## 预期系统行为

1. Gateway 不再能注册新的 WebSocket 连接的路由（握手可能失败或降级）
2. 已建立的 WebSocket 连接继续工作（gorilla/websocket 不依赖 Redis）
3. 消息发送继续成功——写入了 sync_events，离线同步路径完整
4. `POST /v1/auth/request_code` 失败（验证码存储依赖 Redis）
5. Fanout 找不到在线设备，全部走 sync_events 路径
6. 消息不会丢失——客户端重连后通过 sync 补拉

**关键验证：** 消息可达性不受 Redis 故障影响（通过 sync 补拉）

## 观察指标

| 指标 | 预期变化 |
|------|----------|
| `ws_connections_active` | 逐渐下降（新连接无法注册路由） |
| `outbox_pending_count` | 可能短暂上升（fanout 延迟增加） |
| `http_requests_total{path="/v1/auth/request_code"}` | 5xx 增加 |
| `GET /health` | `{"redis":"error: ..."}` |

## 恢复步骤

```bash
bash livechat-server/scripts/chaos/redis-up.sh

# 等待 5 秒后检查
bash livechat-server/scripts/chaos/health-check.sh

# 验证：注册新用户，发送消息，确认投递
curl -s -X POST http://localhost:8080/v1/auth/request_code \
  -H 'Content-Type: application/json' \
  -d '{"phone_e164":"+8613800000101"}'
```

## 验收标准

- [ ] 恢复后 `GET /health` 返回 `{"redis":"ok"}`
- [ ] 恢复后 30s 内新 WebSocket 连接可以成功建立
- [ ] Redis 故障期间发送的消息在恢复后可同步
- [ ] 无数据丢失
