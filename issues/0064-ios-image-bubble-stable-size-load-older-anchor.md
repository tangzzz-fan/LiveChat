---
id: "0064"
title: "iOS 图片气泡固定占位消抖动 + 加载更早滚动锚定"
status: complete
labels: ["done", "p1", "bug", "ux"]
blocked_by: ["0063", "0049"]
created_at: 2026-07-27
---

# 0064 — 图片行抖动与加载更早

## Notes

**现象 1**：含图片的会话进页抖动；纯文本无。  
原因：`ProgressView(120×80)` → 实图 `max 220` 改行高，叠加多帧 `pinToLatest`。

**加载更早**：当前是顶部按钮（非系统下拉）；数据来自本地 GRDB `fetchOlderMessages`。远端缺口另走 sync / gapBackfill。

## What built

- [x] `ImageMessageContent.parseAttachment` 解析 width/height；气泡固定预留框
- [x] 减弱钉底（去掉 count/hasMoreOlder 误钉；重试改为 0+80ms）
- [x] 加载更早后 `scrollTo` 锚定加载前首条

## Acceptance criteria

- [x] 含图会话进页行高稳定
- [x] 「加载更早」只读本地库；加载后不强制钉最新
- [x] `docs/ios-app-testing.md` §11c

## Related

- `MessageImageBubble` · `ImageMessageContent` · `loadOlderMessagesTapped` · 0044
