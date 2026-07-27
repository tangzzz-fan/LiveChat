---
id: "0063"
title: "iOS 进会话偶发未滚到最新消息"
status: complete
labels: ["done", "p0", "bug", "ux"]
blocked_by: ["0044"]
created_at: 2026-07-27
---

# 0063 — 进会话滚动锚定最新消息

## Notes

**现象**：从会话列表进入聊天页，偶发停在偏上位置（看不到最新气泡），或先错位再「往下滚」。

**根因**：
1. `selectConversation` 不清理 `visibleMessages`，与异步 `conversationOpened` 竞态
2. `scrollTo` 在 List 尚未挂上对应 `.id` / 布局未完成时执行会静默失败；仅单帧 `async` 不够
3. `safeAreaInset` 输入栏与 `hasMoreOlder` 后置布局改变可视高度

## What to build

- [x] 选中会话时清空消息窗
- [x] 进会话 pin 底部：可取消多帧重试 `scrollTo`（0 / 50ms / 150ms）+ `defaultScrollAnchor(.bottom)`
- [x] 离开会话取消 scroll Task；`selectConversation` 单测

## Acceptance criteria

- [x] `selectConversation` 清空 `visibleMessages` / `oldestLoadedSeq` / `hasMoreOlder`
- [x] 消息窗变化 / hasMoreOlder / 键盘 触发重新钉底
- [ ] HITL：反复进出多消息会话，稳定停在最新（请确认）

## Related

- `ChatThreadView` · `ChatFeature.selectConversation`
