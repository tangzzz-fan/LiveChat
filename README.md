# LiveChat

LiveChat 是一个面向学习的大规模即时通信系统设计项目，目标是通过规格先行的方式，系统化拆解 WhatsApp 类聊天系统的核心问题：消息正确性、长连接、离线同步、多端一致性、群聊扇出、媒体消息、推送唤醒、安全边界与可观测性。

**快速理解整站架构与高并发 / IM 痛点解法**：先读 [`docs/架构设计总览.md`](docs/架构设计总览.md)。  
**客户端 API**：[`docs/API参考.md`](docs/API参考.md)。  
**多台 iOS 能否接入、怎么按 Spec 13 做**：[`docs/iOS多端接入评估与实现.md`](docs/iOS多端接入评估与实现.md)。

当前仓库已完成 Phase 1–3 的 P0/P1 学习闭环实现（服务端在 `livechat-server/`），规格仍以 `Specs/` 为设计源。

## 当前仓库结构

```text
LiveChat/
├── Specs/                      # 核心规格文档（设计源）
├── livechat-server/            # Go 服务端（message-service / gateway / outbox-consumer）
├── load_test/                  # 压测框架与场景
├── docs/
│   ├── 架构设计总览.md         # ← 新人 / 复盘首选
│   ├── adr/                    # 架构决策记录
│   ├── chaos/                  # 故障演练手册
│   ├── engineering-problems/   # 工程问题库（痛点→方案）
│   └── agents/                 # skills 仓库级配置
├── issues/                     # 本地 ticket 跟踪
├── CONTEXT.md                  # 领域术语表
└── CLAUDE.md                   # 仓库级协作约束
```

## 规格目录

- `00-规格总览与实施规划.md`
- `01-产品边界与SLO.md`
- `02-领域模型与消息生命周期.md`
- `03-账号认证、设备管理与联系人发现.md`
- `04-消息发送主链路与Outbox模式.md`
- `05-长连接网关与协议设计.md`
- `06-离线同步与多端一致性.md`
- `07-群聊模型与扇出策略.md`
- `08-媒体消息与对象存储.md`
- `09-推送通知与后台唤醒.md`
- `10-安全体系与密钥管理.md`
- `11-存储分层与分片设计.md`
- `12-监控告警与压测方案.md`
- `13-iOS客户端架构设计.md`

## 项目原则

- 先定义消息生命周期，再设计接口、存储和同步链路。
- 先保证正确性，再追求吞吐、成本和复杂度优化。
- 先稳定模块边界，再考虑服务拆分和基础设施扩展。
- 所有实现修改都应先更新对应规格，再更新代码和配置。

## 推荐阅读顺序

1. [`docs/架构设计总览.md`](docs/架构设计总览.md) — 全局拓扑 + IM 痛点对照表
2. `Specs/00-规格总览与实施规划.md`
3. `Specs/02-领域模型与消息生命周期.md`
4. `Specs/04-消息发送主链路与Outbox模式.md`
5. `Specs/05-长连接网关与协议设计.md`
6. `Specs/06-离线同步与多端一致性.md`
7. Phase 实现细节：`livechat-server/docs/Phase1-架构设计说明.md`、`Phase2-架构设计说明.md`

## Agent 协作入口

- 仓库级协作规则：`CLAUDE.md`
- 领域术语：`CONTEXT.md`
- Skill 配置：`docs/agents/`
- 架构决策：`docs/adr/`
- 工程问题库：`docs/engineering-problems/`

## 当前状态

- Phase 1–3 学习闭环已在 `livechat-server/`、`load_test/`、`docs/chaos/` 落地
- 规格以 `Specs/` 为唯一设计源；领域术语见 `CONTEXT.md`
- 架构快照入口：`docs/架构设计总览.md`
