---
id: "0043"
title: "iOS 高负载剩余落地：UI 列表基础 + 对照 0033 未完成项"
status: open
labels: ["ready-for-agent", "p0"]
parent: null
blocked_by: ["0035", "0033"]
created_at: 2026-07-27
---

# 0043 — iOS 高负载剩余落地（父票）

## Parent

无。承接 [0033](0033-ios-high-load-client-design.md) 设计文档与 [0035](0035-ios-client-rewrite.md) 主链路完成后的缺口。  
对照：[docs/ios-high-load-client.md](../docs/ios-high-load-client.md) · [docs/ios-client-study-guide.md](../docs/ios-client-study-guide.md) §4。

## 要不要先做 UI 列表基础？

**要。** 高负载文档 10 项里，下列依赖「正经列表 / 消息窗」才能验收或实现：

| 文档项 | 为何依赖 UI 基础 |
|--------|------------------|
| #2 大会话滚动 | 必须有 `conversation_seq` 游标分页 + 懒加载列表 |
| #10 按 seq 渲染 | 当前按 `created_at`；需窗口投影按 seq |
| #1 ValueObservation 去抖 | 需稳定「可见窗」查询再观察，而不是每次 middleware 手刷 |
| #9 图片 | 需消息行 UI 容器（本父票子票可选延期） |

**可不阻塞 UI、可并行**的项：

| 文档项 | 说明 |
|--------|------|
| #5 弱网续跑 / sending 超时 | 基建 + 现有发送队列即可演示 |
| #7 后台预算硬化 | 在 0042 SilentSync 上加超时 |
| #3/#4/#6/#8 | 主链路已基本落地，本父票只验收不重做 |

## What to build

按依赖顺序完成子票；**实现等用户指令**，本票只跟踪范围。

## Acceptance criteria

- [ ] 子票 0044–0048（及可选 0049）按依赖完成或明确延期
- [ ] `docs/ios-high-load-client.md` 实现对照表与票号一致
- [ ] 不复活 0022–0028；编号从 0044 起

## 子票

| ID | 标题 | 依赖 UI 基础？ | blocked_by |
|----|------|----------------|------------|
| [0044](0044-ios-chat-list-seq-window.md) | **UI 基础**：会话列表 + 消息窗按 seq + 分页 | 本身即基础 | — |
| [0045](0045-ios-valueobservation-debounce.md) | ValueObservation 去抖投影 | 是 | 0044 |
| [0046](0046-ios-weak-network-send-hardening.md) | 弱网：path 续跑 + sending 超时 | 否 | — |
| [0047](0047-ios-conversation-gap-backfill.md) | 会话 seq 缺口探测 + 历史补拉 | 演示依赖 0044 | 0044 |
| [0048](0048-ios-silent-wake-budget.md) | 静默唤醒预算硬化 | 否 | 0042 |
| [0049](0049-ios-image-message.md) | 图片消息（高负载 #9，可延期） | 是 | 0044 |
| [0050](0050-ios-high-load-crosscut-verify.md) | 横切验收（Instruments 记录） | 是 | 0044,0045,0046 |

推荐实现序：`0044 → (0045 ∥ 0047) / (0046 ∥ 0048) → 0049? → 0050`。

## 后端依赖结论（2026-07-27）

| 子票 | 是否需新后端 | 说明 |
|------|--------------|------|
| 0044 | 否（本地窗）| 冷开「最新一页」可用已有补拉 + `latest_seq` |
| 0045 / 0046 / 0048 / 0050 | 否 | 纯客户端 |
| 0047 | 否 | `GET .../messages` 已提供 `from_seq` + **`latest_seq` / `has_more`**（对齐 Spec 06 §4.4） |
| 0049 | 否 | 媒体 API 已在 0014；本仓库已有 initiate/complete/download |

**结论：无阻塞性后端前置票；iOS 可从 0044 起做。** 消息补拉响应增强已合入 `livechat-server`（重启 message-service 后生效）。
