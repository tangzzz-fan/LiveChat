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

## 本机实测（2026-07-26）

前置：**先取到 access_token 再注入**。中断期间 `request_code` 不可用，事后无法登录，演练会卡住。

| 阶段 | 观察 | 结果 |
|------|------|------|
| 注入前 | `/health` | `{"postgres":"ok","redis":"ok"}` |
| 注入中 | `/health` | `status=degraded`，`redis: connection refused` |
| 注入中 | `POST /v1/auth/request_code` | **500**（验证码存储依赖 Redis） |
| 注入中 | `POST /v1/messages/send` ×3 | **全部 200**（messages + outbox 只依赖 Postgres） |
| 注入中 | WebSocket upgrade + **protobuf 握手** | **都成功**（见下方偏差） |
| 注入中 | 既有连接的实时投递 | **收不到**（Fanout 无法解析路由） |
| 恢复后 | `/health` / `request_code` | `ok` / 200 |
| 恢复后 | `GET /v1/conversations/{cid}/messages` | **3/3 条消息可补拉**，seq 1,2,3 连续 |
| 恢复后 | `health-check.sh` | All checks passed |

结论：**消息可达性不依赖 Redis**。Redis 影响的是「实时投递与登录」，不是「消息是否存在」。

### 与预期行为的偏差（重要，已用真握手客户端验证）

手册原先写「Gateway 不再能注册新连接的路由（握手可能失败）」。实测**握手照样成功**：

```
中断期间：新连接 HANDSHAKE SUCCEEDED session_id=1160:load-test-dev-1-35446
中断期间：既有连接 NO DELIVERY within 8s
```

也就是说客户端会握着一条**握手成功、心跳正常、但收不到任何投递**的连接。路由注册失败不会反馈给客户端，静默降级比直接失败更难排查。

**客户端启示（对 iOS 尤其重要）**：

- 「握手成功」不等于「在线可达」。不要只靠握手结果判定在线。
- 需要一个独立的活性判据：例如收不到预期投递时按 sync 游标兜底轮询，或让服务端在路由注册失败时主动下发降级信号（当前**没有**这个信号，属开放缺口）。
- 恢复靠重连即可：Redis 回来后重连握手 → 投递立刻恢复，中断期间的消息全部可补拉。

复现：`cd load_test && .venv/bin/python drills/chaos01_redis_outage.py`（自动完成注入与恢复）。

### 顺带发现：握手里的 `device_id` 被忽略

`HandshakeRequest.device_id` 在服务端**未被使用**，session 身份完全取自 JWT claims（`internal/gateway/manager.go`：`deviceID := claims.DeviceID`）。演练中用不同 `device_id` 发起的新连接拿到了同一个 `session_id`，因而顶掉了原连接。

含义：一个 token 只能对应一条连接，客户端无法靠握手字段区分多设备；换设备必须换 token。

## 验收标准

- [x] 恢复后 `GET /health` 返回 `{"redis":"ok"}`
- [x] 恢复后 30s 内新 WebSocket 连接可以成功建立
- [x] Redis 故障期间发送的消息在恢复后可同步
- [x] 无数据丢失
