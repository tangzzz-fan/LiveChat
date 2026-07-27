---
id: "0048"
title: "iOS 静默唤醒预算硬化"
status: complete
labels: ["p1"]
parent: "0043"
blocked_by: ["0042"]
created_at: 2026-07-27
---

# 0048 — 静默唤醒预算硬化

## Parent

[0043](0043-ios-high-load-leftover.md)

## What to build

在 0042 `SilentSyncWakeHandler` 上：

- 明确预算（如 ≤25s）内取消未完成 sync 分页
- `UIBackgroundFetchResult` 与取消/超时对齐
- 后台路径仍：**不**启动 WS、不做重 UI / 大媒体

## Acceptance criteria

- [x] 超时可取消且 completionHandler 总会调用
- [x] 代码/注释与 Spec 13 §8.2 一致
- [x] `ios-high-load-client.md` #7 更新

## 实现备注

- `SilentWakeOutcome`：success / timedOut / failure
- `SyncExecutor` 分页间 `Task.checkCancellation`
- AppDelegate：timedOut → `.noData`（仍必调 completionHandler）

## Blocked by

- [0042](0042-ios-push-token-silent-sync.md)
