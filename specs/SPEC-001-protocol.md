# SPEC-001 — 消息协议与数据模型

> 状态: Draft | Milestone: M1 | 依赖: 无（全系统地基） | 被依赖: 全部后续 spec

## 1. 背景与动机（Why）

所有后续难题（去重、有序、离线同步、多设备）的解法都编码在**消息模型**里。
这份 spec 定义线上传输格式（Protobuf over WebSocket）和三个核心不变量：
消息身份、会话内顺序、幂等语义。地基打歪，后面每个 spec 都要返工。

## 2. 核心挑战与典型解法

### 挑战 A：消息的"身份"是什么？

天真做法：客户端生成一个 UUID 就是消息 ID。问题：UUID 无序，无法用来做
"拉取比 X 新的消息"；且客户端时钟不可信，不能用客户端时间排序。

**典型解法 —— 双 ID 制（WhatsApp/微信/Telegram 通用模式）：**

| ID | 生成方 | 用途 |
|----|--------|------|
| `client_msg_id` (UUIDv7) | 客户端 | 幂等去重：重发时 ID 不变，服务端据此识别重复 |
| `server_msg_id` (Snowflake) | 服务端 | 全局唯一、粗略时间有序，用于存储主键 |
| `conv_seq` (int64) | 服务端 | **会话内严格单调递增**，是排序和同步游标的唯一依据 |

### 挑战 B：分布式系统里的"有序"

没有全局时钟，跨会话的全局序既做不到也不需要。**业界共识：只承诺会话内有序。**
定序权在服务端：Message Service 落库时为每条消息分配 `conv_seq`
（PG 里用 `UPDATE conversations SET last_seq = last_seq + 1 RETURNING last_seq`，
单会话串行化，跨会话完全并行）。客户端按 `conv_seq` 排序渲染，
按 `conv_seq` 缺口检测丢失并触发补拉。

### 挑战 C：at-least-once 送达必然产生重复

网络超时后客户端必须重发（否则丢消息），重发必然可能重复（服务端其实收到了，
只是 ACK 丢了）。**解法：at-least-once 传输 + 服务端按 `client_msg_id` 幂等
= 业务层 exactly-once。** 服务端在 `(sender_id, client_msg_id)` 上建唯一索引，
重复写入冲突时直接返回首次写入的结果（含已分配的 `conv_seq`）——重发者拿到
和首发完全一致的 ACK。

## 3. 线上协议设计

### 3.1 信封（所有帧的统一外壳）

```protobuf
syntax = "proto3";

message Envelope {
  uint64 seq       = 1;  // 连接内帧序号，用于帧级 ACK
  oneof payload {
    // 客户端 → 服务端
    AuthRequest     auth_req      = 10;
    SendMessage     send_msg      = 11;
    SyncRequest     sync_req      = 12;  // 增量拉取: since conv_seq
    AckDelivered    ack_delivered = 13;  // 客户端确认已收到并落库
    Ping            ping          = 14;
    // 服务端 → 客户端
    AuthResponse    auth_resp     = 20;
    SendMessageAck  send_ack      = 21;  // 含 server_msg_id + conv_seq
    MessagePush     msg_push      = 22;  // 服务端下推新消息
    SyncResponse    sync_resp     = 23;
    Pong            pong          = 24;
    ErrorFrame      error         = 25;
  }
}

message ChatMessage {
  string client_msg_id = 1;   // UUIDv7, 幂等键
  int64  server_msg_id = 2;   // Snowflake, 服务端填
  string conv_id       = 3;
  int64  conv_seq      = 4;   // 服务端填, 排序唯一依据
  string sender_id     = 5;
  int64  server_ts_ms  = 6;   // 展示用时间, 以服务端为准
  MessageContent content = 7; // oneof: text / image / audio / system...
}
```

设计要点：

- **oneof 信封**而不是"type 字段 + bytes body"：编译期穷尽检查，Go 和 Swift
  两端都能 switch 全覆盖，加新消息类型时编译器逼你处理。
- 二进制 WebSocket frame，一帧一个 Envelope，不需要额外分帧
  （WebSocket 自带消息边界——这是相对裸 TCP 少学的一课，在 spec 里注明）。
- proto3 + buf 管理，破坏性变更由 `buf breaking` CI 挡住。字段只加不改不删。

### 3.2 消息状态机（客户端视角）

```
composing ──发送──► pending ──收到 SendMessageAck──► sent(有 conv_seq)
                     │  ▲                              │
              超时5s │  │ 重连后带原 client_msg_id 重发   ├─对端 AckDelivered─► delivered
                     ▼  │                              └─对端已读回执────► read (M2)
                    retrying ──超过重试预算──► failed(可手动重试)
```

## 4. 存储模型（PostgreSQL）

```sql
CREATE TABLE conversations (
  conv_id   TEXT PRIMARY KEY,          -- 1:1 会话: 两 uid 排序拼接, 天然幂等
  type      SMALLINT NOT NULL,         -- 1=direct, 2=group(M2)
  last_seq  BIGINT NOT NULL DEFAULT 0  -- 定序器
);

CREATE TABLE messages (
  server_msg_id BIGINT      NOT NULL,  -- Snowflake
  conv_id       TEXT        NOT NULL,
  conv_seq      BIGINT      NOT NULL,
  sender_id     TEXT        NOT NULL,
  client_msg_id UUID        NOT NULL,
  content       BYTEA       NOT NULL,  -- 序列化后的 MessageContent
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conv_id, conv_seq)
) PARTITION BY RANGE (created_at);     -- 月分区: 消息表是无限增长表, 第一天就分区

CREATE UNIQUE INDEX msg_idem ON messages (sender_id, client_msg_id); -- 幂等键
```

**规模化路径（文档承诺，D7）**：单 PG 到千万级消息/日没问题；再往上按
`conv_id` 哈希分库分表；亿级换 Cassandra/ScyllaDB 宽表
（partition key = `conv_id`，clustering key = `conv_seq` 倒序——Discord 公开
架构就是这个模型）。本项目的表结构刻意让这条迁移路是平滑的：
所有查询都以 `(conv_id, conv_seq)` 为轴，没有跨会话 JOIN。

## 5. 范围

**In**：proto 仓库与 buf 工程化；Go/Swift 双端代码生成；Snowflake 发号器；
UUIDv7 客户端库选型；上述 DDL；消息状态机文档。
**Out**：任何网络传输实现（002）、落库逻辑（003）、加密（011——但 content
用 bytes 而非结构化字段，就是为 E2EE 留的门）。

## 6. 验收标准

1. `buf lint && buf breaking` 通过并进 CI；`buf generate` 产出 Go 与 Swift 代码，两端编译通过。
2. 单测：Snowflake 并发 8 goroutine × 10w 次发号无重复；时钟回拨有保护（拒绝发号而非重复）。
3. 单测：同一 `(sender_id, client_msg_id)` 写两次 messages 表，第二次返回第一次的 `conv_seq`，表中只有一行。
4. 文档：消息状态机图 + 每条边的触发条件，评审通过。

## 7. 测试计划

纯单测 + 属性测试（property-based：任意交错的并发发号序列仍满足单调性），
无需集成环境。
