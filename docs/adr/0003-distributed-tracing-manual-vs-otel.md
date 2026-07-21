# 0003: 分布式追踪方案 — 手动 Trace ID vs OpenTelemetry

## 状态

已采用（2026-07-21）

## 背景

Phase 3 需要跨服务 Trace 传递以支持排障。选择是在 P0 引入 OpenTelemetry SDK 还是手动 Trace ID 传递。

## 决策

**P0 手动 Trace ID + Span ID 传递，P1 引入 OpenTelemetry。**

## 理由

1. **OpenTelemetry SDK 的引入成本高**：需要配置 exporter（Jaeger/Zipkin/OTLP）、collector、存储后端。这些基础设施在本地学习项目中是额外负担。
2. **核心价值是 trace_id 串联**：有了统一的 trace_id 就能串联全链路日志。span 的 parent-child 因果链在 P0 阶段不是必需品。
3. **接口抽象 = 零业务代码变更**：`traceutil.OutgoingGRPCContext()` 和 `traceutil.FromIncomingGRPC()` 包装了 gRPC metadata 操作。P1 换 OTel SDK 时，只需改这些函数的实现，**所有 handler 代码不用改**。

## 实现

```go
// 发送端
md := metadata.Pairs("x-trace-id", traceID, "x-span-id", spanID)
ctx = metadata.NewOutgoingContext(ctx, md)

// 接收端
md, _ := metadata.FromIncomingContext(ctx)
traceID = md.Get("x-trace-id")[0]
spanID = md.Get("x-span-id")[0]
```

## 影响

- 所有 gRPC 调用需要手动调用 `traceutil.OutgoingGRPCContext(ctx)` 和 `traceutil.FromIncomingGRPC(ctx)`
- Outbox Consumer 不走 gRPC，trace_id 从 `outbox_events.payload` JSON 中提取
- 结构化日志统一携带 `trace_id`、`span_id`、`service` 字段

## 相关

- Ticket 0018
- `internal/traceutil/trace.go`
