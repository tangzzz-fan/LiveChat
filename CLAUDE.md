# CLAUDE.md

本文件定义在本仓库内协作时应遵循的仓库级规则。

## 项目概览

LiveChat 是一个以学习为导向的 WhatsApp 类即时通信系统设计项目。当前阶段以规格文档为主，先完成领域建模、链路拆解和工程边界约束，再逐步落地服务端、客户端、协议与基础设施实现。

当前仓库的设计源位于 `Specs/`，不是旧的 `specs/SPEC-xxx` 目录结构。

## 当前仓库结构

```text
LiveChat/
├── Specs/                # 核心规格文档
├── .agents/skills/       # 本地 skills 仓库
├── CONTEXT.md            # 领域术语表
├── docs/
│   ├── adr/              # 架构决策记录
│   └── agents/           # skills 仓库级配置
├── README.md
└── CLAUDE.md
```

## 当前阶段约束

- 以 `Specs/` 为唯一设计源。
- 任何设计变更先改 spec，再改配置、脚本和代码。
- 文档编号和主题必须与 `Specs/` 保持一致。
- 旧的 `specs/SPEC-xxx` 路径视为历史路径，不应继续引用。

## 规格推进顺序

优先实现消息正确性骨架，再扩展用户可感知能力，最后补足规模化与工程化能力。

推荐主链路阅读顺序：

1. `Specs/00-规格总览与实施规划.md`
2. `Specs/02-领域模型与消息生命周期.md`
3. `Specs/04-消息发送主链路与Outbox模式.md`
4. `Specs/05-长连接网关与协议设计.md`
5. `Specs/06-离线同步与多端一致性.md`

## 核心架构原则

1. 先建立消息生命周期，再讨论接口和存储细节。
2. 接入层处理连接与协议，业务层处理语义与一致性。
3. 本地状态和服务端状态必须有单一可信源，不能让 UI、网关、消息服务各自发明状态定义。
4. 离线补拉、多端同步、投递 ACK、已读推进必须清晰分层，不能混用一个序号承担多种语义。
5. 监控和压测不是收尾工作，而是规格的一部分。

## 文档与目录约定

- 规格文档放在 `Specs/`
- 领域术语放在 `CONTEXT.md`
- 架构决策放在 `docs/adr/`
- Matt Pocock skills 的仓库级配置放在 `docs/agents/`

## Agent skills

### Issue tracker

Issues 使用 GitHub Issues，外部 PR 不作为 triage request surface。见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认标签映射：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。见 `docs/agents/triage-labels.md`。

### Domain docs

当前仓库按 single-context 处理，领域术语位于根目录 `CONTEXT.md`，架构决策位于 `docs/adr/`。见 `docs/agents/domain.md`。

## 协作要求

- 变更任何配置、脚本、说明文档前，先检查是否仍引用旧路径或旧命名。
- 创建新实现目录时，名称与职责必须能映射回对应 spec。
- 生成代码、测试、脚本或文档时，优先复用 `CONTEXT.md` 中的术语，避免同义词漂移。
- 评审或重构前，先读对应 spec，再读 `CONTEXT.md` 与相关 ADR。
- 命令行中需要通过代理访问外网时，使用 `proxy_on` 开启代理。
- 新实现的能力/模块如果涉及通用工程问题，应同步更新 `docs/engineering-problems/` 并更新 INDEX.md。问题库使用统一模板：问题是什么 → 通用分析思路 → 当前方案 → 替代方案及取舍 → 踩坑记录。
