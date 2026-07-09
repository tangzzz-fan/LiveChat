# LiveChat

WhatsApp 级架构的即时通讯系统——iOS 客户端 + Go 后端。目的不是做产品，而是**通过亲手实现来学习大规模 IM 的经典问题**。

## 学习目标

| 课题 | 具体问题 |
|------|---------|
| 长连接 | 单机 10w+ WebSocket 连接、心跳风暴、重连雪崩 |
| 消息可靠性 | 不丢、不重、不乱序——at-least-once + 幂等 = exactly-once |
| 离线同步 | 收件箱模型 + 游标增量拉取 + APNs 推拉结合 |
| 群聊 | 扇出放大、写扩散 vs 读扩散、异步 worker |
| 状态广播 | N×M presence 风暴治理、订阅模型、数据分级 |
| 移动端 | local-first 架构、iOS 后台保活、乐观发送 |
| 端到端加密 | Signal Protocol：X3DH + Double Ratchet + Sender Keys |
| 流式渲染 | 200 tok/s 打字机效果、TextKit 2 增量排版、帧级合批 |
| 端侧 AI | Foundation Models、语义搜索、资源纪律 |

## 架构全景

```
┌─────────────┐   WebSocket+Protobuf   ┌──────────────────────────────────────┐
│  iOS App    │◄──────────────────────►│  Gateway (Go, 无状态, 可横向扩展 xN)   │
│ SwiftUI     │                        │  连接管理/心跳/鉴权/编解码              │
│ GRDB 本地库 │      HTTPS (REST)      └──────┬───────────────────────▲───────┘
│ 同步引擎    │◄──────────┐                   │ gRPC                  │ push RPC
└─────────────┘           │                   ▼                       │
                   ┌──────┴──────┐    ┌──────────────────┐    ┌───────┴────────┐
                   │  API 服务    │    │  Message Service │───►│  Redis         │
                   │ 注册/登录     │    │  收发/去重/定序    │    │  路由表(user→gw)│
                   │ 会话/联系人   │    │  收件箱写扩散     │    │  在线状态/未读   │
                   └──────┬──────┘    └────────┬─────────┘    └────────────────┘
                          │                    │
                          ▼                    ▼
                   ┌────────────────────────────────┐   ┌───────────────┐
                   │  PostgreSQL（messages 按会话+   │   │  Push Worker  │
                   │  时间分区; inbox; users; convs） │   │  → APNs       │
                   └────────────────────────────────┘   └───────────────┘
```

## Milestones

| Milestone | 范围 | 核心验收 |
|-----------|------|---------|
| M1 | 核心链路：1:1 聊天 | 5w 连接 30min、1,000 msg/s p99<500ms、kill-9 零丢失、飞行模式实验 |
| M2 | 多人实时：群聊 / presence / typing / APNs | 500 人群送达 p99<1s、锁屏推送 <5s |
| M3 | 媒体 / 多设备 / E2EE | MinIO 旁路、双设备同步、服务器盲测零明文 |
| M4 | iOS 深度 + 端侧 AI | 流式 200 tok/s 不掉帧、飞行模式全功能可用 |

## 技术栈

| 层 | 选型 |
|----|------|
| iOS | SwiftUI, GRDB (SQLite), Swift Concurrency (actors), iOS 17+ |
| 后端 | Go, goroutine-per-connection, gRPC, Protobuf (buf) |
| 存储 | PostgreSQL (分区表), Redis |
| 对象存储 | MinIO (S3 协议, M3) |
| E2EE | libsignal (Signal Protocol, M3) |
| 端侧 AI | Apple Foundation Models (iOS 26+, M4), Core ML 降级 |
| 可观测 | Prometheus + Grafana |
| 部署 | docker-compose |

## 仓库结构

```
LiveChat/
├── specs/          # 15 份架构规格文档 + 评审报告
├── proto/          # .proto 定义（Go/Swift 共享）
├── server/         # Go 后端（gateway / msgsvc / api / pushworker / aisvc）
├── ios/            # SwiftUI App
├── loadtest/       # Go 压测客户端
└── deploy/         # docker-compose + Grafana 面板
```

## 快速开始

```bash
# 启动全栈
cd deploy && docker-compose up -d

# 运行压测
cd loadtest
go run ./cmd/loadtest --scenario scenarios/chat_1kmsg.yaml --mode correctness

# iOS
open ios/LiveChat.xcodeproj
```

## 文档

- [SPEC-000 项目总览与 Spec 索引](specs/SPEC-000-overview.md)
- [SPEC-001 消息协议与数据模型](specs/SPEC-001-protocol.md)
- [全部 15 份 Spec](specs/)
- [第一轮评审报告](specs/REVIEW-SPECS.md)
- [第二轮评审报告](specs/REVIEW-SPECS-2.md)
- [CLAUDE.md](CLAUDE.md)——AI 工作指引

## 哲学

- **验证驱动**：每个 spec 以可量化实验为验收标准，不以"代码写完了"为准
- **先跑通再加密**：E2EE 排在 M3 最后——加密会冻结架构，先在明文下把系统调稳
- **连接可以死，数据不能丢**：可靠性在同步层不在连接层
- **推拉结合**：推送求快（best-effort），拉取求全（收件箱兜底）
