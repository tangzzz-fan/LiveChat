---
id: "0022"
title: "iOS 登录：OTP + Keychain + 最小登录 UI"
status: open
labels: ["ready-for-agent", "p1"]
parent: "0021"
blocked_by: []
created_at: 2026-07-21
---

# 0022 — iOS 登录：OTP + Keychain + 最小登录 UI

## Parent

[0021 - iOS 客户端架构骨架](0021-ios-client-architecture-skeleton.md)（Spec 13 后续实现）

## What to build

把 AuthRepository stub 换成真实 HTTP 实现：两步 OTP 登录，token 与 device_id 写入 Keychain，并提供最小登录 UI。端到端：两台模拟器用不同 `device_id` 对同一或不同手机号完成登录，App 显示已登录用户信息，重启 App 后仍保持登录态（直到 token 失效）。

## Acceptance criteria

- [ ] `AuthRepository` 实现 `request_code` / `verify_code` / `refresh`（对接 message-service 现有认证 API）
- [ ] 每台安装生成并持久化唯一 `device_id`（Keychain）；`platform=ios`
- [ ] `access_token` / `refresh_token` / `user_id` 存 Keychain；冷启动可恢复会话
- [ ] 最小登录 UI：输入手机号 → 请求验证码 → 输入验证码 → 进入「已登录」态
- [ ] 可配置 `baseURL`（默认本机 `http://127.0.0.1:8080`）
- [ ] 两台模拟器联调：`GET /v1/devices`（或等价客户端展示）能看到对应设备记录
- [ ] Presentation 不直接调用 URLSession；经 Application / Repository

## Blocked by

None — can start immediately（0021 骨架已 complete）。

## 技术难点与注意事项

- Mock OTP：本地服务端验证码为开发 mock，客户端勿写死生产短信逻辑。
- ATS：开发期明文 HTTP 需 Info.plist 例外或仅用模拟器访问本机。
- 刷新：access 过期时用 refresh 旋转；失败回登录页。
