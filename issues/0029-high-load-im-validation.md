---
id: "0029"
title: "高负载 IM 验证：实践手册、压测硬化、发送背压与 iOS 抗压方案"
status: complete
labels: ["complete", "p0"]
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

- [x] 子票 0030、0031、0032、0033 均 `complete`
- [x] `docs/高负载IM验证计划.md` 与子票状态一致
- [x] 学习型门禁达成：五场景绿 + [填数基线](../load_test/baselines/local-measured-baseline.md) + 2 份 chaos 实测复盘（[01 Redis](../docs/chaos/01-redis-outage.md)、[02 Outbox 背压对照](../docs/chaos/02-outbox-backpressure.md)）
- [x] [架构总览 §4.0](../docs/架构设计总览.md) 注明容量验证口径 = 本地放大因子实证，非 20 万连接
- [x] 打 annotated tag `v0.3.0-high-load-validation`

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

## 验收记录（2026-07-26）

**三个实测结论**

1. 本机最先撞到的天花板是 Gateway 接入限流（每 IP 5 conn/s），不是数据库。单 IP 压测不能外推连接容量。
2. 重连 jitter 直接决定恢复成功率：500ms jitter 下 2/10 成功，无 jitter 0/10 全被拒。
3. 发送侧背压把积压封了顶：暂停 consumer 后 95.2% 的 send 返回 429，pending 停在 88 而非无限增长，恢复后自动放行。

**演练暴露的工具链问题（已修）**

- `outbox-pause.sh` 用 `pgrep -f` 停到 shell 包装进程，演练无效 → 改 `pgrep -x`。
- `health-check.sh` 的 `check()` 把标签当命令执行，7 项检查全部误报 → 已修并加入背压指标。
- `connect` / `send_message` / `sync_backfill` 每次迭代注册新用户，撞 OTP 频控产生假错误 → 改为预建用户池复用。

**遗留（不阻塞本票）**

- `load_test` 的 WS 握手帧是 JSON 占位，网关要求 protobuf（`expected HANDSHAKE_REQ`），因此握手之后的行为（含 Redis 中断时路由注册是否被拒）未覆盖。
- 客户端 429 退避实现属 iOS 侧（0022–0028）。
- 多机压测缺失，单 IP 无法绕过接入限流。
