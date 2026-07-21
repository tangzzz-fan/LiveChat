---
id: "0019"
title: "压测框架与容量基线报告"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0011", "0012", "0013", "0018"]
created_at: 2026-07-21
---

# 0019 — 压测框架与容量基线报告

## Parent

Phase 3: 规模化与工程质量（P0/P1 交界），对应 Spec 12 §6。

## What to build

交付一套 Python asyncio 压测框架 + 5 个场景脚本 + 基线报告模板。压测框架接收 `--concurrency`、`--duration`、`--scenario` 参数，自动执行登录→建连→发消息→测量延迟→输出聚合报告。目标不是做生产级大规模压测，而是让系统具备"可测量、可对照基线的容量评估手段"。

端到端行为：工程师执行 `python load_test/run.py --scenario send_message --concurrency 100 --duration 60s` → 压测框架启动 100 个虚拟用户并发发送消息 60 秒 → 实时输出中间统计（当前 QPS、P95 延迟、错误数）→ 压测结束后自动生成 Markdown 基线报告（含并发数、总请求数、P50/P95/P99 延迟、错误率、吞吐、结论）→ 报告可 diff 对比不同版本之间的容量变化。

## Acceptance criteria

- [ ] Python asyncio 压测框架 `load_test/` 目录：
  - `run.py` 主入口，支持 `--base-url`、`--ws-url`、`--concurrency`、`--duration`、`--scenario`、`--output`
  - `core/tester.py` 核心调度器：管理 user pool、信号量、结果聚合
  - `core/client.py` HTTP + WebSocket 封装（复用连接池、自动 JWT 管理）
  - `core/reporter.py` 聚合结果并渲染 Markdown 报告
  - `scenarios/send_message.py` 文本消息发送压测
  - `scenarios/connect.py` 登录 + WebSocket 连接建立压测
  - `scenarios/group_fanout.py` 群消息扇出压测（200 人群，测量扇出延迟）
  - `scenarios/sync_backfill.py` 离线同步回补压测（模拟大游标落后）
  - `scenarios/reconnect_storm.py` 重连风暴压测（同时断开 N 个连接 → 同时重连 → 测量成功率和恢复时间）
- [ ] 每个 scenario 脚本输出端到端延迟、错误率、吞吐量（msg/s 或 conn/s）
- [ ] 报告模板包含：测试日期、系统版本（git commit SHA）、并发数、持续时长、关键指标表格、与上一基线对比（diff 模式）、结论与建议
- [ ] 提供 `requirements.txt`（`httpx`、`websockets`、`rich` 用于终端进度展示）
- [ ] 生成一份初始基线报告 `load_test/baselines/phase2-baseline.md`，在 Phase 2 完成后执行 5 个场景并填入结果
- [ ] README.md 说明本地运行方式（依赖安装 + 启动服务 + 执行压测）

## Blocked by

- [0011 - 认证收敛 + 设备会话管理 + Push Token 注册](0011-auth-device-sessions-push-token.md)（压测需要完整的 auth 流程，不能依赖 Phase 1 的 register/login mock）
- [0012 - 群会话创建 + 成员管理 + 群事件投影](0012-group-conversation-membership-events.md)（群消息扇出压测需要群存在）
- [0013 - 群消息扇出 + 分级策略 + 热点群保护](0013-group-fanout-tiering-hot-group-protection.md)（扇出压测的测量目标）
- 注意：压测框架代码本身可以在 Phase 2 期间并行编写，但实际执行压测 + 生成基线报告需要 Phase 2 核心链路就绪

## 技术难点与注意事项

### 1. WebSocket 投递延迟的测量方法

**问题：** 如何精确测量"消息从发送客户端到接收客户端 WebSocket 收到投递"的端到端延迟？两台机器的时钟不同步。

**方案：**
- 同一台压测机器上模拟 sender + receiver 两个角色
- sender 记录 `send_time`（HTTP POST 发出前的本地时间）
- receiver 记录 `receive_time`（WebSocket 收到 MESSAGE_DELIVERY 帧的本地时间）
- `delivery_delay = receive_time - send_time`（单机时钟无偏差）
- 对于群消息：多个 receiver 同时监听，取 P50/P95/P99

**坑点：** 压测机器本身的 CPU 负载会影响计时。在高并发下，Python asyncio 的事件循环延迟可能比网络延迟更大。`delivery_delay` 需要和 baseline 对比才有意义——绝对值靠不住，相对变化可信。

### 2. 虚拟用户的状态管理

**问题：** 压测前需要登录 N 个用户、建立会话、加入群。这些准备步骤本身耗时且不可靠。

**方案：**
- `scenario.setup()` 阶段：批量注册用户 + 获取 token + 建立 WebSocket + 创建群并加入成员
- `scenario.run()` 阶段：执行实际压测
- `scenario.teardown()` 阶段：关闭连接、可选清理资源
- 用户池用 `dataclass` 管理：`User(id, phone, token, device_id, websocket)`
- 注册过的用户可复用（如果用户已存在，调用 login 而非 register）

### 3. 群消息扇出的正确性验证

**问题：** 压测不仅要测吞吐和延迟，还要测**正确性**——是否所有群成员都收到了消息？消息顺序是否一致？

**方案：**
- sender 发送 100 条有序消息（seq 0–99），记录每条消息的 `client_message_id`
- 每个 receiver 在 WebSocket 收到消息后记录 `(client_message_id, receive_time, expected_seq)`
- 压测结束后检查：每个 receiver 是否收到了全部 100 条（或接近全部，允许少量投递失败）
- conversation_seq 是否严格递增（无乱序）
- 报告中增加"消息可达率"和"乱序检测率"

### 4. 重连风暴的 Jitter 模拟

**问题：** 真实重连风暴中，客户端不会在精确同一毫秒重连——会有自然的随机抖动。如果压测脚本同时建连，比真实场景更恶劣。

**方案：**
- `reconnect_storm.py` 提供 `--jitter-ms` 参数（默认 500），断开后在 0–500ms 随机延迟后重连
- 同时提供 `--no-jitter` 模式用于极限测试
- 测量指标：重连成功率、P95 重连完成时间、网关 CPU/内存峰值（从 `/metrics` 读取）

### 5. 压测框架的 CI 友好性

**问题：** 压测可能需要 60 秒甚至更长时间，不适合每次 commit 都跑。但需要能手动触发。

**方案：**
- `run.py` 增加 `--quick` 模式：每个场景只跑 10s、concurrency=10，用于 CI sanity check（不应过载系统）
- CI 只跑 `--quick` 模式，完整压测由开发者手动执行
- 输出 JSON 格式结果（`--output json`），方便 CI 解析

### 6. 涉及的关键文件

- `load_test/run.py`、`load_test/core/`、`load_test/scenarios/`
- `load_test/requirements.txt`
- `load_test/README.md`
- `load_test/baselines/phase2-baseline.md`
