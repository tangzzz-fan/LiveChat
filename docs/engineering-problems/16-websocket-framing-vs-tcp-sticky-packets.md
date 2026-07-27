# WebSocket 帧边界：为什么不是 TCP 粘包，却仍会踩边界

标签: `connection`, `ordering`, `idempotency`

## 问题是什么

复习 IM 收发时，常会问：「会不会粘包？」若把 WebSocket 当成裸 TCP 字节流，会误造一套长度前缀解析；若以为「有了 WS 就没有任何边界问题」，又会漏掉双通道重复、超大帧、错误拼帧等坑。

## 典型场景

- 同学用 socket 经验在客户端手写「半包缓冲 + 粘包拆包」。
- 把网关 `ReadFrame`（4 字节长度前缀）误用到 **WebSocket binary message** 上，导致握手永远失败。
- 一条 WS message 里拼多个 protobuf，或把多个 `WsFrame` 粘在一个 `Data` 里发送。
- WS 与 sync 各投递一次同一 `server_message_id`，UI 出现重复气泡。

## 通用分析思路

分层看「谁负责消息边界」：

```text
应用语义（一条聊天消息）
    ↑
业务 envelope（本仓：WsFrame protobuf）
    ↑
WebSocket message（RFC 6455：有帧头、可分片、API 常交付完整 message）
    ↑
TCP 字节流（无消息边界 → 经典粘包/半包出在这里）
```

1. 若 API 已是 **message-oriented**（`URLSessionWebSocketTask.receive`），应用层**不要**再解 TCP 粘包。  
2. 仍要约定：**一个 WS message 对应几个业务 envelope**。  
3. 重复投递是 **幂等** 问题，不是粘包问题。

## 当前项目方案

### 协议约定（与压测客户端一致）

- Gateway WS：`ReadMessage` / `WriteMessage` 得到的是**裸** `WsFrame` protobuf 字节。  
- **没有**应用层 4 字节长度前缀（该前缀只用于 `internal/gateway/frame.go` 的 `ReadFrame`/`WriteFrame` 非 WS 路径）。  
- 约定：**1 WS binary message = 1 `WsFrame`**。  
- iOS：`WsCodec.encodeFrame` / `decodeFrame`；见 `load_test/core/ws_protocol.py` 注释。

### 粘包结论

| 层级 | 本仓是否需手写粘包拆包 |
|------|------------------------|
| TCP | 否（不直接碰 socket） |
| WebSocket | 否（`receive()` 等完整 message） |
| 业务多消息合并 | 否（禁止一 message 多 envelope） |

### 仍必须处理的「收发边界」（不是粘包）

| 边界 | 做法 |
|------|------|
| WS + sync 重复 | `server_message_id` 幂等落库（`IncomingMessageApplier`） |
| 突发多帧 | RealtimeSession ~24ms 批量写 GRDB，再刷新 UI |
| 发送重试 | `client_message_id` 幂等 |
| 超大 message | 依赖系统 `maximumMessageSize`；文本阶段默认够用 |
| 错误拼帧 | 握手 opcode 校验失败则关连 / 不重连 |

复习总表：[ios-client-study-guide.md](../ios-client-study-guide.md) §4。

## 替代方案及取舍

| 方案 | 优点 | 代价 |
|------|------|------|
| 继续 WS message = 单 envelope（当前） | 简单、与 Go gateway/压测一致 | 大批量要用多条 message 或另开批量 opcode |
| 自研长度前缀流（类 gRPC） | 可在一条连接上自由粘包拆包 | 与现网关不兼容；重复造 WS 已提供的能力 |
| 一条 WS message 内 length-delimited protobuf 流 | 可批量 | 双端都要改；调试成本高 |

## 踩坑记录

- 初读 `frame.go` 时易把 `ReadFrame` 的长度前缀带到 iOS WS 实现 → 握手解不出 `HANDSHAKE_RESP`。以 **gorilla WriteMessage 路径** 与压测客户端为准。  
- 「粘包」搜索关键词容易把复习带偏；本仓优先记：**帧边界在 WS；业务边界在约定；重复在幂等**。
