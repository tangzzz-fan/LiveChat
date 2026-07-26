---
id: "0029"
title: "高负载 IM 验证：实践手册、压测硬化、发送背压与 iOS 抗压方案"
status: open
labels: ["ready-for-agent", "p0"]
parent: null
blocked_by: []
created_at: 2026-07-26
---

# 0029 — 高负载 IM 验证（父票）

## Parent

无。承接 Phase 3 已完成的压测框架（0019）与混沌手册（0020），把「可模拟、可度量」升级为对照 Spec 01 **学习型**容量假设的验证闭环；同时把 iOS 作为高负载客户端一等公民（方案先行）。

计划正文：[docs/高负载IM验证计划.md](../docs/高负载IM验证计划.md)

## What to build

完成子票 `0030`–`0033`，使仓库具备：

1. 可执行的业界实践模拟手册（问题→方案→本仓→命令→指标→通过标准）
2. 可跑通的压测五场景 + 填数基线 + chaos 04 注入
3. 一个教学价值明确的服务端解法：Outbox 积压时发送侧 429 背压
4. iOS 高负载/弱网整体方案与坑点文档（实现另开票）

端到端：工程师按手册跑一轮 `--quick` 压测与至少 2 个 chaos 场景 → 指标行为符合预期 → 有/无背压对照可演示 → iOS 抗压设计可指导后续实现票。

## Acceptance criteria

- [ ] 子票 0030、0031、0032、0033 均 `complete`
- [ ] `docs/高负载IM验证计划.md` 与子票状态一致
- [ ] 学习型门禁达成：五场景绿 + 填数基线 + ≥2 chaos 复盘；**不**要求打满 Spec 01 数字
- [ ] CLAUDE.md 或架构总览注明「容量验证 = 本地放大因子实证，非 20万连接」
- [ ] 达标后打 annotated tag（建议 `v0.x.0-high-load-validation` 或与仓库 tag 约定对齐）

## Blocked by

None — 子票可按依赖并行启动。

## 子票

| ID | 标题 | 阶段 |
|----|------|------|
| [0030](0030-load-practice-playbook.md) | 业界实践模拟手册 | A |
| [0031](0031-harden-load-test-chaos.md) | 压测/演练硬化 | B |
| [0032](0032-send-side-outbox-backpressure.md) | 发送侧 Outbox 背压 | C |
| [0033](0033-ios-high-load-client-design.md) | iOS 高负载方案与坑点 | D-1 |

已有正确性票（不归本父票关闭，但参与验证）：[0022](0022-ios-auth-otp-keychain-login-ui.md)–[0028](0028-ios-push-token-silent-sync.md)

## 技术难点与注意事项

- 本地单机无法实证 Spec 01 峰值；验收看「放大因子与降级语义」而非绝对 QPS。
- 负载源 = Python `load_test/`；iOS 不做容量打压。
- 与 0019/0020 的关系：补齐 stub、基线数字与缺失注入，而非推倒重来。
