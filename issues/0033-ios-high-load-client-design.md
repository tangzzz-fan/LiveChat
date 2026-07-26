---
id: "0033"
title: "iOS 高负载/弱网客户端方案与坑点（文档先行）"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0029"
blocked_by: []
created_at: 2026-07-26
---

# 0033 — iOS 高负载 / 弱网客户端方案与坑点

## Parent

[0029 - 高负载 IM 验证](0029-high-load-im-validation.md)  
对齐 Spec 13（iOS 客户端架构）与 [docs/iOS多端接入评估与实现.md](../docs/iOS多端接入评估与实现.md)。

## What to build

**仅文档**：落地 iOS 端高性能/高负载整体方案与坑点，作为后续实现票的设计源。不写 Swift 业务实现。

产出建议路径：`docs/ios-high-load-client.md`（可与多端接入评估交叉链接）。

端到端：实现者阅读该文档 → 能按子问题对照 Spec 13 模块（SendExecutor / SyncExecutor / GRDB / WS）→ 知道本地如何用 `load_test` + Instruments + Network Link Conditioner 复现 → 开实现票时有明确 AC 候选。

## Acceptance criteria

- [ ] 新建 `docs/ios-high-load-client.md`，沿用工程问题库风格：问题 → 业界/常见方案 → 本仓落点（Spec 13 / `ios/` 协议）→ 本地复现 → 坑点
- [ ] 至少覆盖以下 **10** 个子问题：
  1. 突发投递刷新风暴（批量落库 + ValueObservation 去抖）
  2. 大会话滚动（conversation_seq 游标分页 + 懒加载）
  3. 发送队列写风暴（actor MessageSendExecutor + 有界队列 + 幂等键）
  4. GRDB 写并发（单 DatabaseQueue + WAL + 批量事务）
  5. 弱网 / 断网（离线优先 + NWPathMonitor + 队列续跑）
  6. 客户端重连风暴（退避 + jitter + single-flight）
  7. 后台唤醒预算（只触发 sync、心跳降频）
  8. sync 追赶洪流（分批 apply + cursor 单调）
  9. 内存 / 图片（缩略图优先 + 双层缓存 + 取消离屏）
  10. 时钟与顺序（按 conversation_seq 渲染 + server_message_id 去重）
- [ ] 写明本地复现手法：`load_test` 灌 1k+ 消息、group_fanout + 前台观察、Network Link Conditioner
- [ ] 明确 **不做**：用 iOS 模拟器集群打服务端容量
- [ ] 给出建议的后续实现横切验收项（供补进 0023/0024/0025 或新票）：首屏 1k 不卡主线程、弱网发送不丢、突发投递不掉帧
- [ ] 更新 [docs/高负载IM验证计划.md](../docs/高负载IM验证计划.md)、[iOS多端接入评估](../docs/iOS多端接入评估与实现.md)、`ios/README.md` 导航链接

## Blocked by

None — 与服务端 0030–0032 并行。

## 技术难点与注意事项

- 忽略当前 iOS 业务未实现；文档以 Spec 13 与现有骨架（MessageStatus、Database schema）为锚点。
- 避免与 Spec 13 重复粘贴大段架构；本文件聚焦「高负载失效模式与对策」。
- 实现票由维护者后续创建；本票关闭条件是文档与导航就绪，不是代码落地。
