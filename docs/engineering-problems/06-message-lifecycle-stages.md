# 服务端"消息已接收"不等于客户端"消息已送达"

标签: `durability`, `consistency`

## 问题是什么

客户端收到 HTTP 200 后以为对端收到了消息，但实际上 HTTP 200 只表示服务端持久化了。对端可能离线、可能没连 WebSocket、可能连了但投递帧在传输中。这个问题是聊天系统最常见的"假送达" bug。

## 典型场景

```
客户端理解:                         服务端真实状态:
"发送成功，对端应该看到了"    →    "消息已持久化，投递进行中"
"发送失败，需要重试"          →    "消息已持久化，HTTP 响应丢失"
"超时了，我重发一次"          →    "第一次已经持久化了，第二次是重复"
```

## 通用分析思路

1. **把消息生命周期拆成独立的阶段**：Accepted、Delivered、Read，每个阶段有独立的事件和确认方式。
2. **不要把 HTTP 响应语义泛化**：HTTP 的响应边界 = 服务端处理请求的边界，不等于"消息被实际投递"。
3. **异步投递 ≠ 无确认**：异步投递需要 ACK 机制闭合——服务端知道消息投递成功了，客户端也知道投递成功了。
4. **每个状态阶段有不同的超时和重试策略**：Accepted 重试用的是客户端幂等键，Delivered 重试用的是消费者的 at-least-once。

核心原则：**消息状态机是状态机，不是同步 RPC**。Accepted 是同步阶段（HTTP Response），Delivered 和 Read 是异步阶段（WebSocket 推送）。

## 当前项目方案

### 三阶段消息生命周期（Spec 02 + Spec 04）

```
客户端状态:              服务端状态:             触发条件:
─────────────────────────────────────────────────────────
queued                   (无)                   用户点发送
sending                  validated              发起 HTTP 请求
accepted                 persisted              HTTP 200
delivered                delivered_to_device    收到投递 ACK
read                     read_confirmed         收到已读回执
failed                   (根据情况)             超时或 4xx
```

对应 Spec 04 §2.1 的状态表格：

| 阶段 | 语义 | 确认方式 | UI 表现 |
|------|------|----------|--------|
| Accepted | 服务端已持久化 | HTTP 200 | 双灰勾 |
| Delivered | 消息已到达对方设备 | WebSocket ACK 事件 | 双蓝勾 |
| Read | 对方已查看 | Read Receipt 事件 | 双蓝勾（对方头像） |

代码：`livechat-server/internal/domain/types.go` — 状态常量定义。

### HTTP Response 只承诺 Accepted

`POST /v1/messages/send` 返回的 `server_message_id` + `conversation_seq` 只表示"消息已安全写入"，不代表对端已接收。客户端必须通过 WebSocket 的 `MESSAGE_DELIVERY` 帧和 `ACK` 帧来推进 Delivered/Read 状态。

## 替代方案及取舍

| 方案 | 优点 | 缺点 |
|------|------|------|
| **三阶段模型**（当前） | 精确、符合真实语义 | 客户端状态机复杂 |
| **HTTP 同步返回"是否送达"** | 客户端逻辑简单 | 阻塞等待对端在线 → 延迟不可控 |
| **不区分 accepted/delivered** | UI 简单 | 用户看到"已发送"实际没到 → 误判 |

## 踩坑记录

- `accepted` 和 `delivered` 的时间窗口通常在 100-500ms（Outbox 消费 + Fanout + WebSocket 投递）。这个窗口内的大部分用户感知不到差异，但在弱网条件下：HTTP 响应丢失 → 客户端超时重试 → 幂等命中 → `is_duplicate=true` → 消息状态是 accepted，但投递已经发生了。客户端在这个点上如果直接显示"发送失败"就是错的——正确做法是检查 `is_duplicate` 并显示"已发送"。
- "消息已发送但对方离线"的场景：用户 A 的 UI 永远是 accepted，直到 B 上线 + 收到投递 + 发送 ACK。A 的 UI 不能把 accepted 显示为 delivered，除非服务端明确推送了 `delivery_acked` 事件。
