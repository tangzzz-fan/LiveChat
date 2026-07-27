# iOS 0041 实时投递：测试如何设计、怎么跑

> 对应 issue [0041](../issues/0041-ios-websocket-realtime.md)。  
> 写法对齐 [验证链路设计方法论](./验证链路设计方法论.md)：先定「这一层只证明什么」，再写步骤与反例。  
> 操作速查仍见 [ios-app-testing.md](./ios-app-testing.md)。

---

## 1. 0041 在验证分层里证明什么

| 层级 | 0041 是否证明 | 说明 |
|------|----------------|------|
| L0 正确性（对端可见） | **主目标** | A 发 → B **前台、不点 sync** 也能看到同一 `server_message_id` |
| L0 帧/握手正确 | **主目标** | protobuf `HANDSHAKE_REQ/RESP` + 心跳；不是「TCP 连上了」就算过 |
| L0 生命周期 | **主目标** | 后台断 WS；回前台重连 + sync（与 0040 衔接） |
| L1 性能/容量 | 不作主证据 | 突发去抖有横切 AC，但不用模拟器集群压 gateway |
| L2 混沌 | 不作主证据 | 网关 kill 等见 chaos / load-practice |

**一句话**：0041 验证的是 **「前台实时通道 + 后台放弃长连」的产品模型是否闭环**，不是「推送」也不是「纯 sync」。

与 0040 的分工：

```text
0040  SyncExecutor     离线/缺口补齐（HTTP）
0041  RealtimeSession  前台实时（WS MESSAGE_DELIVERY）
两者落库入口统一 → server_message_id 幂等（避免双通道重复气泡）
```

---

## 2. 测试对象与非目标

### 2.1 测什么（必要）

| # | 考察点 | 主信号 | 常见假阳性 |
|---|--------|--------|------------|
| T1 | 握手成功 | 首页 `WS 已连接 · <session_id>` | 只看到 TCP upgrade，未发/未解 protobuf |
| T2 | 实时投递 | B **未**点「手动同步」即出现气泡 + `server_message_id` | B 其实回前台触发了 sync，误当成 WS |
| T3 | 依赖链 | outbox-consumer + gateway 都活着 | 只起 message-service，发了 accepted 但对端永收不到 |
| T4 | 后台模型 | 进后台横幅 `WS 已断开`；前台再连 | 后台仍「看起来连着」但系统已掐断 |
| T5 | 重连不风暴 | 反复前后台，不疯狂打满 gateway | 无退避、多 Task 并发 connect |
| T6 | 包级回归 | `swift test`（编解码 / 退避窗口） | 仅 Xcode 点编译通过 |

### 2.2 不测什么（主动声明）

- 真 APNs / Silent Push → **0042**
- 会话 seq 缺口探测、投递 ACK、已读 UI
- Gateway 极限连接数、压测延迟分布 → `load_test/scenarios/realtime_delivery.py`
- TCP 粘包手写拆包 → WS 已是 message 边界（见工程问题 16）

---

## 3. 流程设计（为什么是这个剧本）

### 3.1 设计原则

1. **一条主链路只问一个问题**：T2 必须锁死「B 不手动 sync」。  
2. **依赖显式**：实时路径 = send → outbox → fanout → gateway → WS；少一环则 T2 失败。  
3. **对照路径**：同一账号在断 WS 时仍可用 0040 sync 证明消息在服务端（避免「发丢了」误判）。  
4. **可自动化的部分下放包测试**；端到端保留双模拟器人工/半自动。

### 3.2 端到端主流程（推荐）

```text
前置   PG+Redis · message-service · outbox-consumer · gateway
       双模拟器登录 A/B，记下 user_id
       B 首页确认「WS 已连接」

主路径 A 打开与 B 的 1:1 → 发文本 → accepted
       B 保持前台，禁止点「手动同步」
       期望 ≤ 数秒内：会话 preview / 气泡出现，副标题 = server_message_id

对照   （可选）杀 gateway 后再发一条：B 收不到实时；
       B 点同步或回前台 sync → 仍能补齐（证明 0040 兜底）

生命周期
       B 进后台 → 「WS 已断开（后台）」
       A 再发一条
       B 回前台 → 重连横幅 + syncFinished；消息可见
```

### 3.3 包级 / 组件级（无模拟器）

| 测试 | 位置 | 证明 |
|------|------|------|
| `wsCodecRoundTripHandshake` | ChatInfrastructureTests | 帧编解码与 opcode |
| `reconnectDelayStaysWithinWindow` | 同上 | 退避落在 Spec 窗口 |
| `upsertIncomingMessageIsIdempotentByServerID` | 同上 | 双通道幂等落库 |
| `ProtobufScaffold.libraryLinked` | 同上 | Generated 入库可链 |

```bash
cd ios/Packages/ChatInfrastructure && swift test
cd ../ChatPresentation && swift test
```

服务端/压测侧对照（可选、更强证据）：

```bash
# load_test：A 发 → B WS 收 MESSAGE_DELIVERY
cd load_test && .venv/bin/python -m scenarios.realtime_delivery
```

---

## 4. 失败归因树（联调时用）

```text
B 收不到实时消息？
├─ B 横幅无「WS 已连接」→ gateway / 握手 / token
├─ A 非 accepted → message-service / 成员资格
├─ A accepted 且 B 已连接仍无 → outbox-consumer 未跑或 fanout 失败
├─ B 点了同步才出现 → 实时路径未通，仅 sync 兜底（0041 未过）
└─ 后台期间期望实时 → 产品模型不允许；应靠 sync/Push(0042)
```

---

## 5. 与横切 AC 的关系

| AC（0041 票） | 本测试如何覆盖 |
|---------------|----------------|
| gen_proto + pb.swift 入库 | 脚本可跑 + Infrastructure 编译/测试 |
| 握手 + 心跳 + 断线兜底 | T1；心跳为协商间隔发帧（日志/代理可选观察） |
| 批量落库禁止每帧 Store | 代码审查 RealtimeSession flush；主观 UI 不掉帧 |
| 双模拟器实时可见 | T2 主路径 |
| 后台断 / 前台重连+sync | T4 |
| 去抖 + 重连不风暴 | T5 + 高负载文档原则 |

---

## 6. 文档与代码锚点

| 用途 | 路径 |
|------|------|
| 操作步骤 | [ios-app-testing.md](./ios-app-testing.md) §3.2 |
| 复习 / 边界 | [ios-client-study-guide.md](./ios-client-study-guide.md) |
| WS 生命周期坑 | [engineering-problems/17-…](./engineering-problems/17-urlsession-websocket-lifecycle-asyncstream.md) |
| 帧边界 | [engineering-problems/16-…](./engineering-problems/16-websocket-framing-vs-tcp-sticky-packets.md) |
| 实现 | `RealtimeSession` · `URLSessionWebSocketTransport` · `WsCodec` |
