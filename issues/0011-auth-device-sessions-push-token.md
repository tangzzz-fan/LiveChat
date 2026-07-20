---
id: "0011"
title: "认证收敛 + 设备会话管理 + Push Token 注册"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: []
created_at: 2026-07-20
---

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

将当前 Phase 1 的 mock auth 收敛到 `Spec 03` 的 P0 账号模型：保留可本地开发的 mock OTP，但 API 形状、DeviceSession 字段、设备列表与吊销语义、Push Token 绑定方式全部对齐 Phase 2。

端到端行为：User 输入手机号请求验证码 → 使用 mock OTP 完成验证并拿到 `access_token + refresh_token + device_id` → App 上报 Push Token → `GET /devices` 能看到当前设备与最近活跃时间 → 当前用户吊销另一台设备后，被吊销设备的后续请求返回 `401 + device_revoked`。

具体交付：

- `POST /v1/auth/request_code` 和 `POST /v1/auth/verify_code`，替代当前偏演示性质的 register/login 形态
- `device_sessions` 数据模型补齐 `platform`、`push_token`、`session_version`、`last_seen_at`
- `GET /v1/devices` 列出全部设备会话
- `POST /v1/devices/{device_id}/revoke` 吊销指定设备
- `POST /v1/devices/push-token` 绑定或更新当前设备的 APNs token
- 登录与设备安全事件写入审计表

## Acceptance criteria

- [ ] `POST /v1/auth/request_code` 对合法 E.164 手机号返回 200 和 `retry_after_sec`
- [ ] `POST /v1/auth/verify_code` 在 mock OTP 场景下返回有效 `access_token`、`refresh_token`、`user_id`、`device_id`
- [ ] 新登录或刷新登录会创建或更新一条 `device_session`
- [ ] `GET /v1/devices` 返回当前用户的设备列表，且包含 `platform`、`last_seen_at`、`is_current`
- [ ] `POST /v1/devices/{device_id}/revoke` 成功后，被吊销设备再次访问受保护端点返回 `401`，错误码为 `device_revoked`
- [ ] `POST /v1/devices/push-token` 能为当前设备绑定或更新 Push Token
- [ ] 登录成功、登录失败、设备吊销、Push Token 更新都写入审计事件表

## Blocked by

None - can start immediately.
