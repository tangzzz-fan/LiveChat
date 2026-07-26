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
| 注入中 | WebSocket HTTP upgrade | **成功**（见下方偏差） |
| 恢复后 | `/health` / `request_code` | `ok` / 200 |
| 恢复后 | `GET /v1/conversations/{cid}/messages` | **3/3 条消息可补拉**，seq 1,2,3 连续 |
| 恢复后 | `health-check.sh` | All checks passed |

结论：**消息可达性不依赖 Redis**。Redis 影响的是「实时投递与登录」，不是「消息是否存在」。

### 与预期行为的一处偏差（值得记住）

手册原先写「Gateway 不再能注册新连接的路由（握手可能失败）」。实测 **HTTP upgrade 依然成功**：升级发生在应用层握手之前，不碰 Redis。也就是说客户端可能握着一条**自认在线、但对 Fanout 不可见**的连接——静默降级比直接失败更难排查。

客户端启示：不要把「WebSocket 连上了」当作在线判据，应以应用层握手响应为准，并在缺失时回退到 sync 轮询。

未验证部分：应用层握手在 Redis 中断时是否被拒——`load_test` 的握手帧是 JSON 占位，网关要求 protobuf（返回 `expected HANDSHAKE_REQ`），因此本轮无法覆盖握手之后的行为。需要 protobuf 客户端才能补上。

## 验收标准

- [x] 恢复后 `GET /health` 返回 `{"redis":"ok"}`
- [x] 恢复后 30s 内新 WebSocket 连接可以成功建立
- [x] Redis 故障期间发送的消息在恢复后可同步
- [x] 无数据丢失
