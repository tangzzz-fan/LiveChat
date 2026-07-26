---
id: "0034"
title: "压测客户端补齐 protobuf WS 握手：覆盖握手之后的行为"
status: complete
labels: ["complete", "p1"]
parent: "0029"
blocked_by: []
created_at: 2026-07-26
---

# 0034 — 压测客户端 protobuf WS 握手

## Parent

[0029 - 高负载 IM 验证](0029-high-load-im-validation.md)（本票是 0029 验收记录里明确留下的盲区）

## What to build

`load_test/core/client.py` 目前发送的握手帧是 **JSON 占位**，网关要求 protobuf，实测回复 `expected HANDSHAKE_REQ`。因此现有压测只能验证 **HTTP upgrade**，握手之后的一切（在线路由注册、实时投递、心跳、`MESSAGE_DELIVERY`、断线语义）全部覆盖不到。

端到端：压测客户端能完成真实 protobuf 握手 → 收到 `HANDSHAKE_ACK` → 用户 A 发消息，已连接的用户 B 在同一进程内收到实时投递帧 → 场景能输出「投递成功率 / 端到端投递延迟」。

## 为什么值得做

这个盲区让两类结论无法验证：

1. **在线投递是否真的发生**。当前 `send_message` 只证明写入成功，不证明对端收到。
2. **Redis 中断时的降级究竟发生在哪一层**。chaos 01 实测发现 HTTP upgrade 在 Redis 挂掉时**依然成功**，客户端可能握着一条对 Fanout 不可见的连接。握手是否被拒、连接是否可路由，目前测不到。

## Acceptance criteria

- [x] 压测侧能生成/使用 Python protobuf 绑定（`./gen_proto.sh` / `make loadtest-proto`；生成物在 `core/gen/`，不入库）
- [x] `core/ws_protocol.py` 发送真实 `HANDSHAKE_REQ` 并解析 `HANDSHAKE_RESP`；失败时抛 `HandshakeRejected`（可读原因）
- [x] `connect` 场景区分三态：`upgrade_throttled` / `handshake_rejected` / `handshake_ok`
- [x] 新增 `realtime_delivery` 场景：A 发 → B 收到 `MESSAGE_DELIVERY`，输出投递成功率与端到端 P50/P95
- [x] 心跳保活：握手后后台心跳，实测收到 `HEARTBEAT_ACK`
- [x] 用该客户端复跑 chaos 01（`drills/chaos01_redis_outage.py`），补上握手/路由行为结论
- [x] 更新 [`local-measured-baseline.md`](../load_test/baselines/local-measured-baseline.md) 与 [chaos 01](../docs/chaos/01-redis-outage.md)

## Blocked by

None.

## 技术难点与注意事项

- **帧格式必须以服务端实现为准**（`internal/gateway` 的 opcode 与长度前缀），不要按文档猜；实测遇到过 opcode/长度解析对不上的情况。
- 协议若变更，压测客户端会静默失效。生成绑定应可重复执行，最好挂到 Makefile，避免 schema 漂移后无人发现。
- 收投递帧需要独立读循环，不能和发送共用同一个 await 链，否则延迟统计会被自身阻塞污染。
- 接入限流（每 IP 5 conn/s、每用户 2 conn/s）仍在：投递场景应预建少量长连接并复用，而不是反复建连。
- 端到端延迟的时钟来源要说明清楚（同机同进程时可用单调时钟，跨机不可直接比较）。

## 实施记录（2026-07-26）

**帧格式的关键发现**

WS binary message 内是**裸的 `WsFrame` protobuf**，没有长度前缀。4 字节长度前缀只用于 `ReadFrame`/`WriteFrame` 的 io.Reader 路径（测试用），不走 WebSocket。旧的 JSON + `>HI` 前缀是完全错误的。

**首次证明端到端实时投递**

- 900 条消息 100% 投递率，端到端 P50 91ms / P95 156ms
- 这是此前所有场景都测不到的：`send_message` 只证明写入

**chaos 01 用真握手复跑的结论**

| 阶段 | 结果 |
|------|------|
| Redis 在线 | 握手成功，实时投递收到 |
| Redis 中断 | **握手仍成功**；既有连接 **8s 内收不到投递** |
| Redis 恢复后重连 | 投递立刻恢复；中断期间 3 条消息全部可补拉 |

含义：客户端会握着一条「握手成功、心跳正常、但收不到任何东西」的连接。静默降级比直接失败更难排查。服务端目前**没有**在路由注册失败时给客户端发降级信号——这是开放缺口。

**顺带发现：握手里的 `device_id` 被忽略**

`HandshakeRequest.device_id` 在 `manager.go` 中未被使用，session 身份完全取自 JWT claims。一个 token 只能对应一条连接，换设备必须换 token。
