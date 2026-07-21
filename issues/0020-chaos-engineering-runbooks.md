---
id: "0020"
title: "故障演练手册与恢复流程"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0018"]
created_at: 2026-07-21
---

# 0020 — 故障演练手册与恢复流程

## Parent

Phase 3: 规模化与工程质量（P0/P1 交界），对应 Spec 12 §7。

## What to build

为 6 个典型故障场景编写可执行的演练手册 + 故障注入脚本 + 恢复校验脚本。每个场景定义明确的注入方式、预期系统行为、观察指标和验收标准。通过一轮完整演练验证 Phase 1–2 的降级路径是否真正有效。

端到端行为：运维人员选择一个演练场景（如 "Redis 不可用"） → 执行对应的故障注入脚本 → 监控仪表盘上出现预期告警（如 Gateway 路由降级告警）→ 检查系统降级行为是否符合预期（消息不丢，通过 Sync 补拉可达）→ 执行恢复脚本 → 系统指标恢复到 baseline → 填写复盘模板记录实际行为与预期的差异。

## Acceptance criteria

- [ ] 6 个故障演练场景，每个场景 1 个 Markdown 手册文件（`docs/chaos/` 目录）：
  - `01-redis-outage.md` — Redis 不可用
  - `02-outbox-backpressure.md` — Outbox Consumer 停止/堵塞
  - `03-db-primary-failover.md` — DB 主库宕机（模拟）
  - `04-push-delay.md` — 推送服务延迟/不可用
  - `05-gateway-pod-failure.md` — 单网关节点宕机
  - `06-hot-group-flood.md` — 热点群消息洪峰
- [ ] 每个手册包含以下标准章节：
  - **场景描述**：故障是什么、影响的组件
  - **注入方式**：手动执行的操作步骤（含具体命令）
  - **预期系统行为**：降级路径、报警触发、用户体感
  - **观察指标**：具体到 Prometheus metric 名称和预期阈值
  - **恢复步骤**：如何撤销注入
  - **验收标准**：恢复后哪些指标必须回到基线
- [ ] 手动故障注入脚本（`scripts/chaos/`）：
  - `redis-down.sh` — 停止 Redis（`brew services stop redis`）
  - `redis-up.sh` — 恢复 Redis（`brew services start redis`）
  - `outbox-pause.sh` — 发送 SIGSTOP 暂停 outbox-consumer 进程
  - `outbox-resume.sh` — 发送 SIGCONT 恢复
  - `db-pause.sh` — 暂停 PostgreSQL（`brew services stop postgresql@16`）
  - `db-resume.sh` — 恢复 PostgreSQL
  - `gateway-kill.sh` — 发送 SIGKILL 到 gateway 进程
- [ ] 恢复后系统状态校验脚本 `scripts/chaos/health-check.sh`：
  - 检查 3 个进程是否运行（message-service、gateway、outbox-consumer）
  - 检查 PostgreSQL 连接
  - 检查 Redis 连接
  - 检查 `outbox_pending_count` 是否在恢复后 30 秒内下降到接近 0
  - 通过 `GET /health` 检查所有服务健康状态
- [ ] 复盘报告模板 `docs/chaos/_postmortem-template.md`
- [ ] 至少完成 1 个场景的真实演练并填写复盘报告（推荐从 `01-redis-outage` 开始，因为它最可控且不涉及数据丢失）
- [ ] README.md 索引 6 个手册，说明首次演练从哪个场景开始

## Blocked by

- [0018 - 可观测性升级：Histogram 指标、分布式追踪与告警规则](0018-observability-histogram-tracing-alerts.md) — 演练的"观察指标"依赖可观测性就绪：没有 Prometheus 指标和 Grafana Dashboard，无法判断降级行为是否符合预期

## 技术难点与注意事项

### 1. 演练的安全边界

**问题：** 故障演练可能误操作到生产环境或导致数据损毁。

**方案：**
- 所有故障注入脚本顶部增加环境检查：`ENV=${CHAT_ENV:-dev}`，只有当 `ENV=dev` 时才执行
- DB 相关的脚本（`db-pause.sh`）使用 `pg_ctl` 的 `-m fast` 模式，保证正在执行的事务能回滚
- Outbox events 在演练后允许积压（反正 Consumer 恢复后会追上），不要手动清理
- 如果 `HEALTH_CHECK_STRICT=true`，脚本在检测到非预期状态（如另一个进程也挂了）时自动终止并提示

### 2. 演练的验收标准量化

**问题：** "系统恢复正常"太模糊。需要量化的验收标准。

**方案：** 每个手册的验收标准使用具体的 Prometheus metric：

| 场景 | 主要验收指标 | 阈值 |
|------|-------------|------|
| Redis 不可用 | `ws_connections_active` 是否在恢复后重新增长 | 恢复后 30s 内 > 0 |
| Outbox 堵塞 | `outbox_pending_count` 是否在恢复后下降 | 恢复后 60s 内降为 0 |
| DB 宕机 | `http_requests_total` 是否恢复接收非 5xx 请求 | 恢复后 10s 内 5xx 率 < 1% |
| 推送延迟 | `message_delivery_via_sync_total` 是否补偿投递 | 恢复后消息可达率 100% |
| 网关宕机 | client 重连成功率 | 重连成功率 > 99% |
| 热点群 | `group_busy_429_total` 是否被触发，非热点群消息是否不受影响 | 非热点群 P95 < 500ms |

**坑点：** 本地开发环境的指标基数太低（可能只有个位数），验收标准的百分比意义不大。P0 的演练验收以"行为符合预期"为主，数字只作为辅助。

### 3. Gateway 宕机的模拟方式

**问题：** 本地开发只有一个 gateway 进程（单节点），宕机后没有第二个节点可以 failover。如何模拟单节点宕机后客户端重连的行为？

**方案：**
- 方案 A（推荐）：启动两个 gateway 进程（不同端口），两个进程注册到同一个 Redis 路由表。客户端在第一个 gateway 断开后重连到第二个。这需要 `configs/` 增加第二个 gateway profile。
- 方案 B（简化）：只验证客户端重连机制的正确性：monitor 检测断线 → 退避重试 → 重连成功后触发 Sync。GW 节点的 failover 本身无法在单机验证。

**P0 选方案 B**——但手册中注明"生产环境多节点部署时，此场景应扩展到验证跨节点重连"。

### 4. 复盘模板的设计

**问题：** 复盘不是为了找责任人，而是为了验证架构假设是否成立。

**方案：** 模板关注 3 个问题：
1. **预期 vs 实际**：哪些行为符合预期？哪些偏离了？
2. **发现的新问题**：演练中暴露了哪些规格/代码/配置中的 gap？
3. **行动项**：需要改什么？是改代码、改配置、改 spec、还是改手册？

不包含"责任人"、"严重程度"等传统 incident review 字段——这是学习项目，不是生产 incident response。

### 5. 演练的时机选择

**问题：** 在 Phase 3 执行故障演练前，Phase 2 的群聊、媒体、推送模块可能尚未经过充分验证，引入故障注入会让问题更难定位。

**方案：**
- 第一个演练（Redis 不可用）可以在 Phase 1 已有基础上立即执行——不依赖 Phase 2
- 后续演练建议在对应模块实现完成后再执行
- 手册可以提前写好，演练延后到 Phase 2 完成 + 0018 可观测性就绪之后

### 6. 涉及的关键文件

- `docs/chaos/01-redis-outage.md` ～ `06-hot-group-flood.md`
- `docs/chaos/_postmortem-template.md`
- `docs/chaos/README.md`
- `scripts/chaos/redis-down.sh`、`redis-up.sh`
- `scripts/chaos/outbox-pause.sh`、`outbox-resume.sh`
- `scripts/chaos/db-pause.sh`、`db-resume.sh`
- `scripts/chaos/gateway-kill.sh`
- `scripts/chaos/health-check.sh`
