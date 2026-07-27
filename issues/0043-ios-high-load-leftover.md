---
id: "0043"
title: "iOS 高负载剩余落地：UI 列表基础 + 对照 0033 未完成项"
status: complete
labels: ["done", "p0"]
parent: null
blocked_by: ["0035", "0033"]
created_at: 2026-07-27
---

# 0043 — iOS 高负载剩余落地（父票）

## Parent

无。承接 [0033](0033-ios-high-load-client-design.md) 设计文档与 [0035](0035-ios-client-rewrite.md) 主链路完成后的缺口。  
对照：[docs/ios-high-load-client.md](../docs/ios-high-load-client.md) · [docs/ios-client-study-guide.md](../docs/ios-client-study-guide.md) §4。

## Acceptance criteria

- [x] 子票 0044–0050 按依赖完成
- [x] `docs/ios-high-load-client.md` 实现对照表与票号一致
- [x] 不复活 0022–0028；编号从 0044 起

## 子票

| ID | 标题 | 状态 |
|----|------|------|
| [0044](0044-ios-chat-list-seq-window.md) | UI 基础：会话列表 + 消息窗按 seq + 分页 | complete |
| [0045](0045-ios-valueobservation-debounce.md) | ValueObservation 去抖投影 | complete |
| [0046](0046-ios-weak-network-send-hardening.md) | 弱网：path 续跑 + sending 超时 | complete |
| [0047](0047-ios-conversation-gap-backfill.md) | 会话 seq 缺口探测 + 历史补拉 | complete |
| [0048](0048-ios-silent-wake-budget.md) | 静默唤醒预算硬化 | complete |
| [0049](0049-ios-image-message.md) | 图片消息（高负载 #9） | complete |
| [0050](0050-ios-high-load-crosscut-verify.md) | 横切验收 | complete |

## 后端依赖结论（2026-07-27）

无阻塞性后端前置票；媒体 API 已在 0014。
