---
id: "0038"
title: "iOS 登录：OTP + Keychain + 最小登录 UI（重写）"
status: complete
labels: ["complete", "p0"]
parent: "0035"
blocked_by: []
created_at: 2026-07-27
---

# 0038 — iOS 登录：OTP + Keychain + 最小登录 UI

## Parent

[0035 - iOS 客户端从零重写](0035-ios-client-rewrite.md)

## What to build

两台模拟器（iPhone 17 Pro / Pro Max，iOS 26.5）用不同 `device_id` 完成 OTP 登录；token 与 device 写入 Keychain；冷启动恢复会话；最小登录 UI。Presentation 经 Store/Middleware → UseCase，不直接打 URLSession。

## Acceptance criteria

- [x] `AuthRepositoryLive`：`request_code` / `verify_code` / `refresh` + `listDevices`
- [x] 每台安装持久化唯一 `device_id`（Keychain）；`platform=ios`
- [x] token / user_id 存 Keychain；bootstrap 冷启动可恢复
- [x] 登录 UI：手机号 → 验证码 → 已登录页（显示 user/device + `/v1/devices`）
- [x] 默认 `baseURL` `http://127.0.0.1:8080`；ATS 本地例外已就绪
- [x] App target 联编成功并装到双模拟器；登录页可演示（本机 message-service 可达）
- [x] Presentation 经 Middleware 调 `AppServices.auth`，不直接 URLSession

## Blocked by

None

## 验收记录（2026-07-27）

- 包测：ChatDomain / Infrastructure / Application / Presentation `swift test` 绿
- `xcodebuild` iPhone 17 Pro（iOS 26.5）`BUILD SUCCEEDED`；双模拟器安装启动显示登录表单
- 服务端 `POST /v1/auth/request_code` 本机 200
