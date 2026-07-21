# 跨服务追踪：Trace ID 如何在多进程链路中透传

标签: `observability`, `scale`

## 问题是什么

聊天系统的一个"消息发送"请求穿越 Gateway → Message Service → Outbox Consumer → Fanout → Gateway 共 5 个进程。如果每个进程各自生成日志，没有统一的关联 ID，排障时就像在 5 本不同的书里找同一件事——几乎不可能。

## 典型场景

```
Gateway 日志: "handshake ok user=1 device=ios-a trace_id=abc123"
Message Service 日志: "send message ok server_message_id=msg_001"
Outbox Consumer 日志: "event consumed id=42"
Gateway 日志: "deliver to device ios-a ok"

问题: 这些日志中没有任何共同标识符表明它们是同一条消息的全链路
→ 故障排查时无法串联
→ 延迟分析时不知道哪个环节慢
```

## 通用分析思路

1. **在入口生成 Trace ID**。Gateway 握手或 HTTP 请求到达时生成。
2. **在每个进程边界传递 Trace ID + Span ID**。使用 HTTP header / gRPC metadata / Outbox payload 等方式。
3. **Span ID 表示一个进程内的处理单元**。每个进程生成自己的 Span ID。
4. **结构化日志携带 trace_id 和 span_id**。JSON 格式优于纯文本格式——方便 grep 和聚合。

## 当前项目方案

LiveChat 采用手动 Trace + Span 传播（0018 实现），不引入 OpenTelemetry（P1 扩展）：

### Trace ID 生成与传递

```
Gateway (入口)
  → traceID = hex(random(16 bytes))
  → spanID = "gateway-" + hex(random(4 bytes))
  → 注入 gRPC metadata: x-trace-id, x-span-id

Message Service (中间)
  → 从 gRPC metadata 提取 traceID
  → spanID = "msg-svc-" + hex(random(4 bytes))
  → 写入 outbox_events 时携带 traceID

Outbox Consumer (异步)
  → 从 outbox_events.payload JSON 读取 traceID
  → spanID = "outbox-" + hex(random(4 bytes))
  → gRPC DeliverMessage 调用时携带 traceID

Gateway (出口)
  → 从 gRPC metadata 提取 traceID
  → spanID = "gateway-deliver-" + hex(random(4 bytes))
  → WebSocket 帧发送后记录日志
```

### gRPC Metadata 传播

```go
// 发送端
func OutgoingGRPCContext(ctx context.Context) context.Context {
    traceID := TraceIDFromContext(ctx)
    spanID := SpanIDFromContext(ctx)
    md := metadata.Pairs("x-trace-id", traceID, "x-span-id", spanID)
    return metadata.NewOutgoingContext(ctx, md)
}

// 接收端
func FromIncomingGRPC(ctx context.Context) context.Context {
    md, _ := metadata.FromIncomingContext(ctx)
    if vals := md.Get("x-trace-id"); len(vals) > 0 {
        ctx = WithTraceID(ctx, vals[0])
    }
    if vals := md.Get("x-span-id"); len(vals) > 0 {
        ctx = WithSpanID(ctx, vals[0])
    }
    return ctx
}
```

### Outbox（异步链路）的 Trace 传递

```json
// outbox_events.payload
{
  "server_message_id": "msg_conv_abc_000001",
  "conversation_id": "conv_abc",
  "trace_id": "abc123def4567890",
  ...
}
```

Outbox Consumer 轮询到事件后，从 payload JSON 中提取 `trace_id`。

### 结构化日志格式

```json
{
  "time": "2026-07-21T15:30:00Z",
  "level": "INFO",
  "msg": "fanout complete",
  "trace_id": "abc123def4567890",
  "span_id": "fanout-a1b2c3d4",
  "service": "outbox-consumer",
  "server_message_id": "msg_conv_abc_000001",
  "conversation_id": "conv_abc",
  "target_count": 199,
  "online_count": 15,
  "duration_ms": 45
}
```

**P0 简化：不引入 OpenTelemetry**
- `span_id` 在每个服务进程边界生成新的随机值（不需要 parent-child span 因果链）
- `trace_id` 保持不变贯穿全链路
- P1 再引入 OTel SDK，届时替换 `traceutil` 的实现即可，业务代码不用改

## 替代方案及取舍

| 方案 | 串联能力 | 引入成本 | 性能开销 | LiveChat 选择 |
|------|---------|---------|---------|--------------|
| 无追踪（各打各日志） | 无 | 零 | 零 | 不采用 |
| 手动 Trace ID（当前方案） | 中 | 低 | ~0 | **✅ P0 采用** |
| OpenTelemetry SDK | 高 | 中 | 低 | P1 计划 |
| 全链路 APM (Datadog/NewRelic) | 高 | 高（$$$） | 低 | 未采用 |

## 踩坑记录

1. **Outbox Consumer 不走 gRPC 链**——不能通过 metadata 传递 trace ID，因为它从 DB 轮询事件。解决方案：在 outbox_events payload JSON 中携带 trace_id。
2. **Span ID 不需要 parent-child 关系（P0）**——每个进程自己生成 span_id。这对串联追踪已经足够（所有日志有同一个 trace_id），但看不到调用拓扑图。P1 引入 OTel 后可建立 parent-child。
3. **P0 metrics 仍使用 atomic counter + 手动切片**——histogram 升级留到 P1。当前仅 trace/span 传递是 P0 就绪的。

### 代码位置

- `internal/traceutil/trace.go` → TraceID + SpanID + gRPC metadata 传播
- `internal/api/router.go` → withLogging（HTTP 层 trace_id 生成）
- `cmd/outbox-consumer/main.go` → message_created handler（trace_id 从 payload 提取）
