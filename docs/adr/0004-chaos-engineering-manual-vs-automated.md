# 0004: 故障演练策略 — 手动触发 vs 自动化 Chaos Engineering

## 状态

已采用（2026-07-21）

## 背景

Phase 3 P0 要求在 6 个典型故障场景上验证系统降级行为。选择是在本地手动执行故障注入还是引入 Chaos Mesh / Litmus 等自动化平台。

## 决策

**P0 手动注入脚本 + 演练手册，自动化 Chaos 留到 P1。**

## 理由

1. **自动化 Chaos 需要 K8s 环境**：Chaos Mesh 和 Litmus 都依赖 Kubernetes Pod 级别的故障注入。当前项目在本地运行 3 个 Go 进程，不需要容器编排。
2. **学习目标是验证架构假设，不是演练自动化**：每次手动演练都是一次"有问题吗"的思考——这个流程比自动化更有学习价值。自动化 Chaos 更适合在 CI 中做回归测试。
3. **演练手册本身就是交付物**：清晰记录"注入什么 → 期望看到什么 → 实际发生了什么 → 差距在哪"的流程，比自动化脚本的覆盖面更大。

## 实现

- 6 个场景手册（`docs/chaos/`）
- 注入/恢复脚本（`scripts/chaos/`）
- 复盘模板（`docs/chaos/_postmortem-template.md`）
- 所有脚本包含 `CHAT_ENV=dev` 环境检查（防误操作）

## 影响

- 演练需要开发者手动执行（不在 CI 中自动运行）
- 脚本仅支持本地 macOS 环境（`brew services` 管理 PostgreSQL/Redis）
- P1 升级到容器部署时，注入方式需重新设计

## 相关

- Spec 12 §7
- Ticket 0020
- `docs/chaos/`
- `scripts/chaos/`
