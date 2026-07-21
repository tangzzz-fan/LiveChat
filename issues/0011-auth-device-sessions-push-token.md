---
id: "0011"
title: "认证收敛 + 设备会话管理 + Push Token 注册"
status: complete
labels: ["done"]
parent: "0010"
blocked_by: []
created_at: 2026-07-20
updated_at: 2026-07-21
---

# 0011 — 认证收敛 + 设备会话管理 + Push Token 注册

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

将 Phase 1 的 mock auth（`register`/`login` 一站式）收敛到 Spec 03 的 P0 两步认证模型。核心变化：API 从 `POST /v1/auth/register` + `POST /v1/auth/login` 演进为 `POST /v1/auth/request_code` + `POST /v1/auth/verify_code`，设备表补齐 `session_version` 字段，增加 Push Token 绑定端点和审计事件写入。

端到端行为：User 输入手机号请求验证码 → 拿到 `retry_after_sec` → 使用 mock OTP（任意 6 位数字）完成验证 → 拿到 `access_token + refresh_token + user_id + device_id` → 新老用户统一走 `verify_code` 路径（首次自动创建 user）→ App 上报 Push Token → `GET /devices` 能看到所有设备 → 吊销某设备后，被吊销设备的后续请求返回 `401 device_revoked` → 所有安全事件写入审计表。

## Acceptance criteria

- [ ] `POST /v1/auth/request_code` 接受 `{phone_e164}`，对合法 E.164 返回 `200 {retry_after_sec, expires_in_sec}`；单号码 3 次/小时以上返回 `429`
- [ ] `POST /v1/auth/verify_code` 接受 `{phone_e164, verification_code, device_id, platform}`，mock OTP 接受任意 6 位数字，返回 `{access_token, refresh_token, user_id, device_id, expires_in}`；首次调用自动创建 user
- [ ] 旧的 `POST /v1/auth/register` 和 `POST /v1/auth/login` 端点移除或标记废弃，所有现有测试迁移到新端点
- [ ] `devices` 表新增 `session_version INT NOT NULL DEFAULT 1` 列；`verify_code` 对已存在设备更新 `last_seen_at` 并递增 `session_version`，对新设备插入新行
- [ ] `GET /v1/devices` 返回当前用户全部设备，含 `platform`、`last_seen_at`、`is_current`（当前请求的 device_id）
- [ ] `POST /v1/devices/{device_id}/revoke` 递增目标设备的 `session_version`，后续该设备任何请求返回 `401 {error_code: "device_revoked"}`
- [ ] `POST /v1/devices/push-token` 接受 `{push_token, platform}`，更新当前设备的 push_token 字段
- [ ] 新增 `login_audit_events` 表（DDL 参考 Spec 03 §7.2），写入 login_success / login_failed / code_request / device_added / device_revoked / token_refreshed / token_replay_detected 事件
- [ ] `POST /v1/auth/refresh` 复用 Phase 1 已有端点，增加审计写入
- [ ] 中间件在 JWT 校验后增加 `session_version` 检查：从 claims 中取 `device_id` → 查 `devices` 表 → 如果 claims 中的 `session_version` < 表中的版本 → 返回 `401 device_revoked`

## Blocked by

None — can start immediately.

## 技术难点与注意事项

### 1. 两步认证的状态管理

**问题：** `request_code` 和 `verify_code` 是两个独立 HTTP 请求，服务端需要在两步之间记住"这个手机号请求过验证码、验证码是多少"。但 P0 使用 mock OTP，不需要真正发送短信。

**方案：** 用 Redis 存储 `code_verification:{phone_e164}` → `{code, attempts, expires_at}`，TTL 5 分钟。`verify_code` 时校验并删除。Mock OTP 固定用 `"123456"` 或读取 Redis 中的 mock code。

**坑点：** 不能把验证码放在 JWT 里让客户端回传——那样用户可以伪造验证码。必须是服务端状态。

### 2. session_version 吊销的原子语义

**问题：** 吊销设备后，该设备可能持有尚未过期的 JWT。仅靠 JWT 过期时间（1h）来做吊销窗口太慢。

**方案：**
- JWT payload 中包含 `session_version`（签在 `verify_code` 时从 devices 表读取）
- 中间件校验 JWT 后，用 `device_id` 查 `devices.session_version`，若 claims 中的版本 < 表中版本 → 401
- `revoke` 端点执行 `UPDATE devices SET session_version = session_version + 1 WHERE id = $1`

**坑点：** 每次请求多一次 DB 查询。可以用 Redis 缓存 `device:{id}:session_version`，revoke 时同时失效缓存，但 P0 可以先直接查 DB（devices 表很小）。

### 3. 旧 API 的迁移策略

**问题：** Phase 1 的 `register`/`login` 端点被 router_integration_test.go 大量引用。直接删除会破坏现有测试。

**方案：** 保留旧 handler 函数但标记为 deprecated，新端点用新 handler。更新集成测试指向新端点。Phase 1 smoke test 脚本需要相应更新。

### 4. E.164 手机号校验

**问题：** 国际号码格式不一致。

**方案：** P0 做基本正则校验：必须以 `+` 开头，后跟 7-15 位数字。不引入完整的 libphonenumber 依赖（太重）。存储时 trim 空格和横线。

### 5. Refresh Token Rotation 的简化

**问题：** Spec 03 §4.3 描述了 Rotation + Reuse Detection，P0 可简化但接口要预留。

**方案：** P0 使用静态 refresh_token（Phase 1 已有实现）。`devices` 表的 `refresh_token_hash` 字段继续使用。P1 再引入 rotation。但审计表预留 `token_replay_detected` 事件类型。

### 6. 涉及的关键文件

- `internal/auth/auth.go` — 需增加 session_version 到 Claims
- `internal/api/router.go` — 新端点 + 中间件改造
- `internal/api/router_integration_test.go` — 迁移到新端点
- `migrations/` — 新增 011 号迁移（login_audit_events 表 + devices 表加 session_version 列）
- `internal/domain/types.go` — Device 结构体增加 SessionVersion
- `scripts/phase1-smoke.sh` — 更新 curl 命令
