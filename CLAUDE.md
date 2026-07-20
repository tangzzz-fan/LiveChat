# CLAUDE.md

本文件定义在本仓库内协作时应遵循的仓库级规则。

## 项目概览

LiveChat 是一个以学习为导向的 WhatsApp 类即时通信系统设计项目。当前阶段以规格文档为主，先完成领域建模、链路拆解和工程边界约束，再逐步落地服务端、客户端、协议与基础设施实现。

**Phase 1 的“消息正确性骨架”父级目标已完成，代码位于 `livechat-server/`；但 Phase 1 仍有若干子票处于 `in_progress` 收尾状态，因此不能表述为“Phase 1 全部完成”。**

当前仓库的设计源位于 `Specs/`，不是旧的 `specs/SPEC-xxx` 目录结构。

## 当前仓库结构

```text
LiveChat/
├── Specs/                # 核心规格文档
├── livechat-server/      # Go 服务端实现（Phase 1）
│   ├── cmd/              # 三个入口（message-service, gateway, outbox-consumer）
│   ├── internal/         # 8 个领域包
│   ├── proto/            # Protobuf 定义
│   ├── migrations/       # PostgreSQL DDL
│   ├── docs/             # 技术决策文档
│   └── README.md         # 操作说明 + API 文档
├── docs/
│   ├── adr/              # 架构决策记录
│   ├── agents/           # skills 仓库级配置
│   └── engineering-problems/  # 工程问题库（问题→分析→方案）
├── issues/               # 本地 issue 跟踪
├── .agents/skills/       # 本地 skills 仓库
├── CONTEXT.md            # 领域术语表
├── README.md
└── CLAUDE.md
```

## 当前阶段约束

- 以 `Specs/` 为唯一设计源。
- 任何设计变更先改 spec，再改配置、脚本和代码。
- 文档编号和主题必须与 `Specs/` 保持一致。
- 旧的 `specs/SPEC-xxx` 路径视为历史路径，不应继续引用。
- `0001`（消息正确性骨架）当前为 `complete`，说明 1:1 消息发送、实时投递、离线同步、已读收敛的父级目标已经具备固定 runbook 或自动化测试证据。
- Phase 1 当前仍有子票 `0002`、`0004`、`0005`、`0007` 处于 `in_progress`，因此不能表述为“Phase 1 全部 ticket 已完成”。
- Phase 2 票据已拆分，但整体尚未开始执行。

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

Issues 使用本地 `issues/` 目录（不依赖 GitHub Issues）。见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认标签映射：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。见 `docs/agents/triage-labels.md`。

### Domain docs

当前仓库按 single-context 处理，领域术语位于根目录 `CONTEXT.md`，架构决策位于 `docs/adr/`。见 `docs/agents/domain.md`。

### 开发环境

- Go 1.22+, PostgreSQL 16+, Redis 7+
- 使用 `brew` 管理本地服务
- 通过 `proxy_on` 开启终端代理访问外网
- `livechat-server/Makefile` 提供常用命令
- 当前已验证的默认开发路径是**本机服务模式**：本地 PostgreSQL + 本地 Redis
- 本地 Redis 视为当前仓库的默认运行时依赖，默认地址为 `localhost:6379`
- 本地 PostgreSQL 视为当前仓库的默认运行时依赖，默认地址为 `localhost:5432`
- 若 `make dev` 因缺少 `docker` 失败，不视为仓库故障，应直接切换到本机服务模式
- 当前仓库**不存在**可作为默认入口的 `./scripts/setup.sh` 或 `./scripts/stop.sh`，不要再引用它们作为标准启动方式
- 推荐启动顺序：
  1. 确认本地 PostgreSQL 与 Redis 已启动
  2. 执行 `make migrate-up`
  3. 分别执行 `make run-message-service`、`make run-gateway`、`make run-outbox-consumer`

### Issue 实现约定

- 按 ticket 编号提交（一个 commit 对应一个或多个连续的 ticket）
- 提交信息格式：`feat(XXXX): description` 或 `docs:` / `fix:`
- 每完成一个 ticket，更新 `issues/INDEX.md` 中的状态
- 实现完成后打 annotated tag

### Phase 推进流程

每个新 Phase 开始前：

1. **读 spec**：重读该 Phase 对应的 spec 文档，确认 P0 范围和已实现的基础。
2. **拆 ticket**：按 spec 的 P0 交付物拆成独立可验证的垂直切片, 使用 /to-ticket 命令。
   - 每个 ticket 有明确的 Acceptance Criteria（可演示的端到端行为）。
   - ticket 之间标注依赖关系（blocked_by）。
   - ticket 编号从上一个 Phase 的最大编号 +1 开始递增。
3. **审阅拆分**：向用户展示拆分方案，确认粒度、依赖和 HITL/AFK 标记后执行。
4. **发布 issues**：在 `issues/` 目录下创建对应文件，更新 `INDEX.md`。
5. **按序实现**：按依赖顺序逐个实现 ticket，每个 ticket 一个 commit。
6. **Phase 验收**：全部 ticket 完成后，打 annotated tag（格式：`v0.<n>.0-p0`）。

## 协作要求

- 变更任何配置、脚本、说明文档前，先检查是否仍引用旧路径或旧命名。
- 创建新实现目录时，名称与职责必须能映射回对应 spec。
- 生成代码、测试、脚本或文档时，优先复用 `CONTEXT.md` 中的术语，避免同义词漂移。
- 评审或重构前，先读对应 spec，再读 `CONTEXT.md` 与相关 ADR。
- 命令行中需要通过代理访问外网时，使用 `proxy_on` 开启代理。
- 新实现的能力/模块如果涉及通用工程问题，应同步更新 `docs/engineering-problems/` 并更新 INDEX.md。问题库使用统一模板：问题是什么 → 通用分析思路 → 当前方案 → 替代方案及取舍 → 踩坑记录。
