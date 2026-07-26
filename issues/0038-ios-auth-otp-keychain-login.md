---
id: "0038"
title: "iOS 登录：OTP + Keychain + 最小登录 UI（重写）"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0035"
blocked_by: []
created_at: 2026-07-27
---

# 0038 — iOS 登录：OTP + Keychain + 最小登录 UI

## Parent

[0035 - iOS 客户端从零重写](0035-ios-client-rewrite.md)  
设计源：[ios-client-rewrite.md](../docs/ios-client-rewrite.md) / Spec 13 / [API参考](../docs/API参考.md)

## What to build

两台模拟器（iPhone 17 Pro / Pro Max，iOS 26.5）用不同 `device_id` 完成 OTP 登录；token 与 device 写入 Keychain；冷启动恢复会话；最小登录 UI。Presentation 经 Store/Middleware → UseCase，不直接打 URLSession。

## Acceptance criteria

- [ ] `AuthRepository` Live 实现：`request_code` / `verify_code` / `refresh`（对接 message-service）
- [ ] 每台安装持久化唯一 `device_id`（Keychain）；`platform=ios`
- [ ] `access_token` / `refresh_token` / `user_id` 存 Keychain；冷启动可恢复
- [ ] 登录 UI：手机号 → 验证码 → 进入已登录态（Store 视图真相）
- [ ] 可配置 `baseURL`（默认 `http://127.0.0.1:8080`）；ATS 本地例外已就绪
- [ ] 双模拟器联调：`GET /v1/devices`（或客户端展示）可见两台设备
- [ ] Presentation 不直接调用 URLSession

## Blocked by

None — can start immediately（0037 complete）。

## 技术难点与注意事项

- Mock OTP：本地验证码以服务端 mock 为准（常见 `123456`）。
- Keychain 自写薄封装，不引入 KeychainAccess。
- access 过期用 refresh；失败回登录页。
