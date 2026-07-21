---
id: "0018"
title: "可观测性升级：Histogram 指标、分布式追踪与告警规则"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0016"]
created_at: 2026-07-21
---

# 0018 — 可观测性升级：Histogram 指标、分布式追踪与告警规则

## Parent

Phase 3: 规模化与工程质量（P0/P1 交界），对应 Spec 12 §3–§5、§8。

## What to build

将 Phase 1 的 `internal/metrics/` 从 atomic counter + 自维护切片的基础形态升级为符合 Prometheus 最佳实践的可观测性体系：延迟从 per-request 手动切片改为 histogram bucket 自动聚合、Trace ID 从单服务透传升级为跨进程链式传递、结构化日志补齐 span_id 和业务字段、定义 3 层仪表盘与 P0/P1/P2 告警规则。

端到端行为：运维打开 Grafana 能看到 3 层仪表盘（业务层 → 服务层 → 基础设施层）→ 每次消息发送全链路日志携带同一个 `trace_id`，从 Gateway → Message Service → Outbox Consumer → Fanout → Gateway 串联 → 当在线投递延迟 P95 > 1.5s 持续 5 分钟时，Prometheus 触发 P1 告警 → 当消息重复率 > 0.05% 时触发 P0 告警 → 正确性指标（消息重复计数、消息乱序计数）首次被系统化采集。

## Acceptance criteria

- [ ] `internal/metrics/` 重构为 Prometheus 标准 histogram：`http_request_duration_seconds`（bucket: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5）、`message_send_duration_seconds`、`fanout_duration_seconds`、`sync_duration_seconds`
- [ ] 新增 gauge：`ws_connections_active`（已有）、`outbox_pending_count`（已有）、`sync_lag_events`（用户最大游标落后量）
- [ ] 新增 counter：`message_duplicate_detected_total`（幂等命中计数）、`message_seq_gap_detected_total`（conversation_seq 缺口计数）、`push_sent_total`、`push_rejected_total`
- [ ] Trace ID 跨服务传递：Gateway 在握手时生成 trace_id → 注入到 gRPC metadata → Message Service 从 context 提取 → Outbox Consumer 从 outbox_events 的 trace 字段读取 → Fanout 继续传递到投递 gRPC 调用
- [ ] `outbox_events` 表已有的记录增加 `trace_id` 字段（Phase 1 的 trace 可能在 payload JSON 中）；本票将其提升为正式列并索引
- [ ] 结构化日志：所有 `slog.Info/Error` 调用携带 `trace_id`、`span_id`（至少 service 级别）、`user_id`（如可获取）、`conversation_id`、`server_message_id`；Span ID 用 `service_name + random_hex(4)` 格式
- [ ] Grafana 仪表盘 JSON 定义文件 `deploy/grafana/dashboards/`：
  - `business.json`：消息 QPS、延迟 P50/P95/P99、投递成功率、消息重复率、推送成功率
  - `services.json`：HTTP/gRPC 延迟、Outbox 消费速率/pending/失败率、Fanout 成功率、Sync 延迟分布
  - `infrastructure.json`：DB QPS/连接池/慢查询、Redis 内存/命中率/连接数
- [ ] Prometheus alert rules 文件 `deploy/prometheus/alerts.yml`：
  - P0：`message_duplicate_rate > 0.05%`（5min）、`service_down`（instant）
  - P1：`fanout_duration_p95 > 1.5s`（5min）、`gateway_connect_failure_rate > 1%`（5min）
  - P2：`outbox_consumer_lag > 500ms`（10min）、`single_node_cpu > 85%`（10min）
- [ ] 现有 `GET /metrics` 端点切换到新 histogram 实现，向后兼容：counter 和 gauge 保持现有名称不变，histogram 以 `_bucket`/`_sum`/`_count` 后缀追加

## Blocked by

- [0016 - 安全基线加固与审计收敛](0016-security-baseline-audit.md) — 需要审计事件被监控覆盖（login_failed 计数、token_replay_detected 计数等）

## 技术难点与注意事项

### 1. Prometheus Histogram —— 从手写到标准库

**问题：** Phase 1 的 `requestDuration` 用 `map[string]*requestSeries` + 手动切片存储延迟，内存会无限增长且不支持 Prometheus histogram 的 `_bucket` 语义。

**方案：** 使用 `github.com/prometheus/client_golang/prometheus` 的标准 `HistogramVec`：

```go
var (
    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latency in seconds.",
            Buckets: prometheus.DefBuckets, // 或自定义 buckets
        },
        []string{"method", "path", "status"},
    )
)
```

在每个 handler 中使用 `promhttp.InstrumentHandlerDuration()` 或手动 `timer := prometheus.NewTimer(HTTPRequestDuration.WithLabelValues(...))`。

**坑点：**
- `DefBuckets` 是 `.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10`——对消息延迟来说偏大。要用自定义 buckets。
- `HistogramVec` 的 label 组合会创建多个时间序列，注意 cardinality 不要太高（不要用 `user_id` 做 label）。

### 2. Trace ID 跨 gRPC 传递

**问题：** Gateway 和 Message Service 之间、Outbox Consumer 和 Gateway 之间通过 gRPC 通信。Phase 1 只透传了 `trace_id` 在 Go context 中，gRPC 跨进程时丢失。

**方案：** 使用 gRPC metadata（`grpc-go` 的 `metadata` 包）：
- 发送端：`md := metadata.Pairs("x-trace-id", traceID, "x-span-id", spanID)` → `metadata.NewOutgoingContext(ctx, md)`
- 接收端：`md, ok := metadata.FromIncomingContext(ctx)` → 提取 `x-trace-id` 和 `x-span-id` → 注入到当前 span context
- 统一封装 `internal/traceutil/` 的 `PropagateGRPC(ctx, traceID, spanID)` 和 `ExtractGRPC(ctx)` 函数

**坑点：** Outbox Consumer 不是 gRPC 调用链——它是从 DB 轮询 event。事件 JSON 中需要新增 `trace_id` 字段。Phase 1 的 `outbox_events.payload` JSON 中可能已有 `trace_id`，但本票要把它提到 `outbox_events` 的独立列。

### 3. 正确性指标的采集

**问题：** "消息重复率" 和 "消息乱序率" 需要额外的业务逻辑判断，不能通过 middleware 统计。

**方案：**
- `message_duplicate_detected_total`：在 `messages.Service.Send()` 的幂等分支（`is_duplicate == true`）中 increment counter
- `message_seq_gap_detected_total`：在 `sync.Service` 查询 `sync_events` 时，如果发现 client cursor 和最新 event_seq 之间的差值 > 1，可视为可能的缺口（不是绝对的乱序，因为 event_seq 按 user 分配，一个 user 不同 conversation 的 event 间隔是正常的）

**坑点：** `message_seq_gap_detected_total` 的界定需要谨慎。P0 简化：只在 `GET /v1/sync/events` 的 handler 中比较 `cursor` 与 `latest_event_seq` 的差值。差值 > 100 记一次 `sync_gap_large`；差值 > 1000 记一次 `sync_gap_critical`。

### 4. Grafana Dashboard 的交付物形式

**问题：** Grafana JSON 文件依赖数据源 UID 和具体的 Prometheus 实例地址。

**方案：** 交付 Grafana JSON 的同时，交付一个 `README.md` 说明变量配置（`$datasource` 变量指向 Prometheus）。JSON 中使用 `${DS_PROMETHEUS}` 变量而非硬编码 UID。提供 `deploy/grafana/provisioning/` 目录用于 Grafana 的 file-based provisioning。

### 5. 结构化日志的 span_id 策略

**问题：** Phase 1 只有 `trace_id`，没有 `span_id`。需要定义 span 的边界。

**方案（P0 简化——不引入 OpenTelemetry）：**
- `span_id` 在每个服务进程边界生成一个新的随机值（不需要父子 span 的因果链）
- `trace_id` 保持不变贯穿全链路
- 每个服务的日志中携带 `trace_id` + `span_id` + `service`（`gateway` / `message-service` / `outbox-consumer`）
- 不需要完整的 OpenTelemetry SDK——P0 手动管理即可。P1 再引入 OTel。

### 6. 涉及的关键文件

- `internal/metrics/metrics.go` — 重写为 Prometheus client_golang 标准实现
- `internal/traceutil/trace.go` — 增加 gRPC metadata 传递 + span_id
- `internal/api/router.go` — handler 包装 promhttp instrumentation
- `cmd/gateway/main.go` — gRPC client 端 metadata 注入
- `cmd/message-service/main.go` — gRPC server 端 metadata 提取
- `migrations/` — outbox_events 表增加 trace_id 列
- `deploy/grafana/dashboards/` — 3 个 JSON dashboard
- `deploy/prometheus/alerts.yml` — 告警规则
- `go.mod` — 增加 `prometheus/client_golang`
