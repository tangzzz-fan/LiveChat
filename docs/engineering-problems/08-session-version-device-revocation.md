# 设备吊销与会话版本号：为什么不能只靠 JWT 过期

标签: `security`, `consistency`

## 问题是什么

JWT 是无状态的——一旦签发，在过期前持续有效。当用户吊销某台设备时，该设备的 JWT 仍然可以访问系统，最长可达 1 小时（JWT 默认有效期）。这是一个安全窗口。

## 典型场景

1. 用户的手机被盗 → 用户在另一台设备上吊销被盗手机
2. 被盗手机上的 JWT 还有 50 分钟才过期
3. 如果没有额外的吊销机制，攻击者在 50 分钟内仍可发消息、读消息

**这不是 JWT 的设计缺陷**——JWT 的"无状态"特性恰恰是它的核心价值（不需要每次请求查 DB）。吊销问题的本质是：**如何在保持 JWT 无状态优势的同时，增加有状态的吊销能力**。

## 通用分析思路

有几种经典方案：

1. **JWT 黑名单**：维护一个被吊销 JWT 的 `jti` 集合（Redis Set），中间件校验时检查。代价：每次请求查 Redis。
2. **短 TTL + 频繁刷新**：JWT 有效期设为 5 分钟，吊销后只要不刷新就自动失效。代价：刷新频率高，用户体验差。
3. **Session Version**：JWT 中携带一个版本号，DB 中也存一份。吊销时递增 DB 版本号。代价：每次请求需查 DB（或缓存）。

## 当前项目方案

LiveChat 采用 **Session Version** 方案（0011 实现）：

```
JWT Claims:
  user_id: 1
  device_id: "ios-abc"
  sv: 3            ← session_version

devices 表:
  id = "ios-abc"
  session_version = 5   ← 已经被吊销了 2 次
```

**中间件校验流程：**

```go
// 1. 解析 JWT → claims.SessionVersion = 3
claims, _ := authSvc.VerifyAccessToken(token)

// 2. 查 DB 的当前 session_version
var dbVersion int
db.QueryRow("SELECT session_version FROM devices WHERE id=$1 AND user_id=$2",
    claims.DeviceID, claims.UserID).Scan(&dbVersion)

// 3. 比较：如果 JWT 的版本 < DB 版本 → 已被吊销
if int64(dbVersion) > claims.SessionVersion {
    return 401 {"error_code": "device_revoked"}
}
```

**版本号何时递增？**
- 用户主动吊销设备（`POST /v1/devices/{did}/revoke`）
- Refresh Token 轮换时（每次刷新都递增，保证旧 refresh_token 失效）

**关键设计决策：P0 不用 Redis 缓存 session_version**
- `devices` 表很小（每个用户只有几个设备），直接查 DB 已经足够快
- 避免引入 Redis 缓存一致性问题（revoke 时需要同步失效缓存）

## 替代方案及取舍

| 方案 | 每次请求延迟 | 吊销延迟 | 复杂度 | LiveChat 选择 |
|------|------------|---------|--------|--------------|
| JWT 黑名单 (Redis Set) | ~1ms (Redis) | 即时 | 中 | 未采用 |
| 短 TTL (5min) | 0 | ≤5min | 低 | 未采用 |
| Session Version (DB) | ~1ms (DB) | 即时 | 中 | **✅ 采用** |
| Session Version (Redis) | ~1ms (Redis) | 即时 | 高（缓存一致性） | P1 可选优化 |

## 踩坑记录

1. **device_id 跨用户冲突**：不同用户使用相同 device_id（如 `"ios-dev-001"`）会导致 PK 冲突。0011 实现时在 `verify_code` 中加了 `DELETE FROM devices WHERE id=$1 AND user_id!=$2` 来处理。
2. **旧 register 端点用固定 sv=1**：deprecated 的 `handleRegister` 和 `handleLogin` 在签发 JWT 时硬编码 `sessionVersion=1`。如果用户先用旧端点登录然后用新端点吊销，旧的 JWT 不会被吊销——这是 backward compat 的已知限制。
3. **Revoke 后 Refresh 仍可用**：吊销设备只递增 `session_version`，但 refresh_token 仍然有效。用户可以用 refresh_token 获取新 JWT（携带新的 session_version），从而"复活"设备。这在 WhatsApp 中也是合理行为——refresh_token 被视为设备密钥，吊销 JWT 但不吊销 refresh_token。

### 代码位置

- `internal/auth/auth.go` → Claims.SessionVersion
- `internal/api/router.go` → authMiddleware.Wrap（session_version 校验）
- `internal/api/router.go` → handleRevokeDevice（递增 session_version）
- `internal/api/router.go` → handleVerifyCode（签发时读取 session_version）
- `migrations/011_auth_convergence.up.sql` → ALTER TABLE devices ADD session_version
