---
id: "0030"
title: "高负载业界实践模拟手册（load-practice）"
status: open
labels: ["ready-for-agent", "p0"]
parent: "0029"
blocked_by: []
created_at: 2026-07-26
---

# 0030 — 高负载业界实践模拟手册

## Parent

[0029 - 高负载 IM 验证](0029-high-load-im-validation.md)

## What to build

在 `docs/load-practice/`（或与 `docs/engineering-problems/15-high-concurrency-failure-modes.md` 交叉链接）交付一套**可执行**的实践手册：每个问题统一模板，能直接对照本仓命令与指标跑通。

端到端：新人打开手册 → 选「热点群」或「重连风暴」→ 复制命令执行 → 在 `/metrics` / 日志中看到预期现象 → 对照「通过标准」勾选。

## Acceptance criteria

- [ ] 新建 `docs/load-practice/README.md` 索引 + 至少 **8** 篇场景文档，覆盖：
  - 写高峰
  - 热点群 / 写扩散
  - 重连风暴
  - Outbox 背压（消费侧积压）
  - Redis 降级
  - DB 暂停 / 主库不可用
  - Gateway kill
  - 离线 sync 积压
- [ ] 每篇统一模板：
  1. 问题是什么（含放大因子公式或定性说明）
  2. 业界常见方案
  3. 本仓现状（Implemented / Partial / Missing + 代码锚点）
  4. 如何模拟（精确命令：`load_test` / `scripts/chaos/*`）
  5. 观察什么（metrics、日志关键字、Redis key 等）
  6. 通过标准（学习型行为预期，非 Spec 01 绝对数字）
- [ ] 与 [15-high-concurrency-failure-modes.md](../docs/engineering-problems/15-high-concurrency-failure-modes.md)、[docs/chaos/](../docs/chaos/) 交叉链接，避免重复叙述冲突
- [ ] 更新 [docs/高负载IM验证计划.md](../docs/高负载IM验证计划.md) / 架构总览导航入口

## Blocked by

None。命令细节可先写「目标命令」，在 0031 硬化后回填实测参数。

## 技术难点与注意事项

- 手册必须可执行；禁止只写概念无命令。
- 若某场景依赖 0031（如 sync_backfill stub），在「如何模拟」中标注 blocked，仍保留预期观察项。
- 用语与 `CONTEXT.md` 对齐（conversation_seq、outbox、fanout 等）。
