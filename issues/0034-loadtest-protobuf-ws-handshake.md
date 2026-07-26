---
id: "0034"
title: "压测客户端补齐 protobuf WS 握手：覆盖握手之后的行为"
status: open
labels: ["ready-for-agent", "p1"]
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

- [ ] 压测侧能生成/使用 Python protobuf 绑定（`livechat-server/proto/` 为唯一 schema 源；生成方式写进 `load_test/README.md`，不手写 wire format）
- [ ] `core/client.py` 发送真实 `HANDSHAKE_REQ` 并解析 `HANDSHAKE_ACK`；失败时给出可读原因而非静默 `pass`
- [ ] `connect` 场景区分三种结果：upgrade 失败 / 握手失败 / 握手成功，分别计数
- [ ] 新增或扩展场景：A 发消息 → 已连接的 B 收到 `MESSAGE_DELIVERY`，输出投递成功率与端到端延迟 P50/P95
- [ ] 心跳保活：连接能维持超过一个心跳周期而不被网关断开
- [ ] 用该客户端复跑 chaos 01，补上「Redis 中断时握手/路由注册行为」的实测结论
- [ ] 更新 [`local-measured-baseline.md`](../load_test/baselines/local-measured-baseline.md) 与 [chaos 01](../docs/chaos/01-redis-outage.md)

## Blocked by

None。

## 技术难点与注意事项

- **帧格式必须以服务端实现为准**（`internal/gateway` 的 opcode 与长度前缀），不要按文档猜；实测遇到过 opcode/长度解析对不上的情况。
- 协议若变更，压测客户端会静默失效。生成绑定应可重复执行，最好挂到 Makefile，避免 schema 漂移后无人发现。
- 收投递帧需要独立读循环，不能和发送共用同一个 await 链，否则延迟统计会被自身阻塞污染。
- 接入限流（每 IP 5 conn/s、每用户 2 conn/s）仍在：投递场景应预建少量长连接并复用，而不是反复建连。
- 端到端延迟的时钟来源要说明清楚（同机同进程时可用单调时钟，跨机不可直接比较）。
