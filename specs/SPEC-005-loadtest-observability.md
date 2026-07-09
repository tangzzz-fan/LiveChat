# SPEC-005 — 压测与可观测性：让"高并发"变成可证明的数字

> 状态: Draft | Milestone: M1 | 依赖: SPEC-002, 003 | 被依赖: M1 全局验收, 006

## 1. 背景与动机（Why）

没有压测和指标，"我实现了高并发"只是一句宣言。本 spec 交付两样东西：
一个能模拟 5w+ 客户端的 Go 压测器，和一套 Prometheus + Grafana 指标体系。
它们也是 SPEC-002/003 所有验收实验的执行工具——先有测量，才有优化。

## 2. 核心挑战与典型解法

### 挑战 A：一台机器怎么假装 5 万个客户端？

- **临时端口耗尽**：一个 (源IP, 目标IP, 目标端口) 组合只有 ~28k 可用源端口
  （`ip_local_port_range` 默认 32768~60999）。5w 连接必超。
  解法（按序尝试）：① 目标侧多端口（每个网关监听 4 个端口）；
  ② 压测容器配多个虚拟 IP（`ip addr add`）并轮转 bind；
  ③ 多个压测容器分摊。把踩坑过程写进实验笔记——这是所有压测新手的第一课。
- **压测器自身成为瓶颈**：5w goroutine 客户端本身吃 1~2GB 内存与大量 CPU
  （TLS 加解密）。压测器必须自我监控（报告自己的 CPU/内存/GC），
  否则测出来的"服务端延迟"其实是压测机的调度延迟。
- **客户端行为建模**：真实用户不是匀速机枪。场景配置化：
  在线挂机（只心跳）、活跃聊天（泊松到达，均值 6 msg/min）、
  重连风暴（全体同时断开重连）、慢消费者（读 goroutine 注入延迟）。

### 挑战 B：E2E 延迟怎么测才不骗自己？

- **测什么**：发送方 `send_msg` 发出 → 接收方收到 `msg_push` 的墙钟差。
  发送和接收 goroutine 在同一进程 → 无时钟偏差问题（这是单机压测的隐藏福利）。
- **coordinated omission**（Gil Tene 经典问题）：如果压测器"发完一条等响应
  再发下一条"，服务端卡顿时你恰好没在发压，尖峰延迟被系统性漏记。
  解法：按**预定时间表**发压（open-loop），消息应在 T 时刻发出而实际 T+Δ
  发出的，Δ 计入延迟。
- **分布不是平均数**：用 HDR Histogram 记录，报告 p50/p90/p99/p99.9/max。
  IM 的用户体验死在 p99，不在均值。

### 挑战 C：指标体系（观察什么才能诊断问题？）

| 层 | 关键指标 |
|----|---------|
| Gateway | 活跃连接数、conn 建立/关闭速率、sendCh 溢出次数、心跳超时踢人数、goroutine 数、每连接内存 |
| MsgSvc | msg/s 吞吐、发送 ACK 延迟直方图、扇出批大小、PG 事务耗时、幂等冲突计数 |
| 存储 | PG: TPS/锁等待/表膨胀；Redis: ops/s、路由表大小、内存 |
| Go 运行时 | GC pause、heap、sched latency（`/debug/pprof` 常开） |
| 业务正确性 | **投递完整率**（应收 vs 实收——不丢的持续证明）、重复计数（恒 0） |

Grafana 一块总览板：连接数、吞吐、E2E p99、错误率四张图讲完系统状态。

### 挑战 D：正确性校验模式（correctness mode）

压测不仅测性能，还要**证明语义**（SPEC-003 验收 1~4 的自动化）：
每个虚拟接收者维护 per-conversation 的期望 `conv_seq` 集合，实收后断言
无缺口、无重复、无乱序；压测报告最后一节输出 PASS/FAIL。
性能模式和正确性模式共享场景配置，一个开关切换。

## 3. 交付物

```
loadtest/
├── cmd/loadtest/       # CLI: --scenario xxx.yaml --mode perf|correctness
├── scenarios/          # idle_50k.yaml / chat_1kmsg.yaml / reconnect_storm.yaml / slow_consumer.yaml
└── report/             # HDR 直方图输出 + markdown 报告生成
deploy/
├── docker-compose.yml  # 2×gateway + msgsvc + api + PG + Redis + prometheus + grafana
└── grafana/dashboards/
```

## 4. 范围

**In**：上述全部；SPEC-002/003 验收实验的 runbook 脚本化。
**Out**：iOS 端性能剖析（004 用 Instruments 自测）、云上分布式压测（D8 排除）。

## 5. 验收标准

1. `idle_50k` 场景在开发机跑通：5w 连接 30 分钟，报告生成，含压测器自身
   资源占用（证明压测器不是瓶颈：CPU < 70%）。
2. `chat_1kmsg` 场景：1,000 msg/s × 10 分钟，E2E p99 < 500ms，投递完整率 100%。
3. correctness 模式在 `kill -9` 混沌注入下（脚本随机杀 gateway/msgsvc）
   仍报告零丢失、零重复——这一条跑通，M1 就毕业了。
4. Grafana 面板导入即用；实验期间的任何指标问题（如 sendCh 溢出）能在面板
   上直接看到并对应到日志。

## 6. 测试计划

压测器核心逻辑（时间表调度、断言器）单测；场景 YAML 有 schema 校验；
`make loadtest-smoke`（1k 连接缩小版）进 CI 防回归。
