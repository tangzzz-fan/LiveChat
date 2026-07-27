---
id: "0050"
title: "iOS 高负载横切验收：Instruments / 弱网记录"
status: complete
labels: ["done", "p1"]
parent: "0043"
blocked_by: ["0044", "0045", "0046"]
created_at: 2026-07-27
---

# 0050 — 高负载横切验收记录

## Parent

[0043](0043-ios-high-load-leftover.md)

## What to build

把 `ios-high-load-client.md`「建议的横切验收项」跑成**可复查证据**（不必自动化满分）：

- 约 1k 历史首屏：Time Profiler / 主观卡顿说明
- 弱网发送：Network Link Conditioner 下 queued → 恢复可达
- 突发投递：列表不掉帧级卡顿（0045 生效后）
- 重连：不并发多条 WS（已有实现，补观察记录）

产出：短文或小节追加到 `docs/ios-high-load-client.md` / `docs/ios-app-testing.md`。

## Acceptance criteria

- [x] 四条横切验收均有文字结论（通过 / 已知限制）— 见 `docs/ios-high-load-client.md`
- [x] 链接到所用 load_test / 操作步骤

## Blocked by

- [0044](0044-ios-chat-list-seq-window.md)
- [0045](0045-ios-valueobservation-debounce.md)
- [0046](0046-ios-weak-network-send-hardening.md)
