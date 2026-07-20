# LiveChat Server 构建与测试

本文档集中维护 `livechat-server/` 的构建、启动、测试与阶段验证信息。`README.md` 只保留入口说明和 API 文档，避免把构建命令、测试状态、阶段验收结论混在一起。

## 环境要求

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | 编译语言 |
| PostgreSQL | 16+ | 主数据库 |
| Redis | 7+ | 路由缓存与运行时依赖 |
| protoc | 任意 | Protobuf 代码生成，可选 |

## 启动模式

当前支持两种本地开发模式。

### 模式 A：本机服务模式

适用于本机已经启动 PostgreSQL 和 Redis 的情况。

```bash
psql -h localhost -U livechat -d livechat -c 'select 1;'
redis-cli -h localhost -p 6379 ping
make migrate-up
make run-message-service
make run-gateway
make run-outbox-consumer
```

### 模式 B：Docker 模式

适用于本机存在 `docker` 命令时。

```bash
make dev
make migrate-up
make run-message-service
make run-gateway
make run-outbox-consumer
```

注意：当前执行环境不一定安装 `docker`。如果 `make dev` 报 `docker: No such file or directory`，不要继续把 Docker 路径当成默认通过条件，应切换到“本机服务模式”。

## Build 命令

```bash
make build
```

该命令应完成以下二进制编译：

- `./cmd/message-service`
- `./cmd/gateway`
- `./cmd/outbox-consumer`

## Test 命令

```bash
make test
```

当前仓库没有任何 `*_test.go` 文件，因此：

- `make test` 只能证明 `go test ./...` 可执行
- 它本质上仍是“编译级校验”
- 它不能单独证明 Phase 1 已测试通过

如果要把 `make test` 升级为真正的回归保护，需要补齐：

- 消息发送幂等与 Outbox 事件生成测试
- 增量同步 API 测试
- 会话消息补拉测试
- WebSocket 握手、心跳和实时投递测试

## 运行时健康检查

```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
curl http://localhost:8081/health
```

预期：

- `message-service` 返回 PostgreSQL 和 Redis 状态
- `message-service` 暴露 Prometheus 格式指标
- `gateway` 返回 `active_sessions`

## Phase 1 验证回路

当前 Phase 1 的最小可执行验证命令为：

```bash
./scripts/phase1-smoke.sh
```

这个脚本会验证：

1. PostgreSQL 和 Redis 可连通
2. `message-service` 健康检查可用
3. `message-service` metrics 端点可用
4. 两个用户可以注册并拿到 token
5. 可以手工创建 direct conversation 并加入成员
6. 发送消息后返回 `server_message_id` 与 `conversation_seq`
7. 相同 `client_message_id` 重发时命中幂等
8. 接收方可看到会话摘要
9. 接收方可看到同步事件
10. 接收方可补拉会话消息

## Phase 1 当前结论

截至当前仓库状态，Phase 1 只能判定为：

- **部分能力已验证**
- **整体验收未通过**

## 当前实测结果

本次在当前仓库中实际得到的结果如下：

- `make build`：通过
- `make test`：通过，且仓库已具备 `gateway`、`outbox`、`conversations`、`sync`、`receipts` 的聚焦自动化测试
- `go test ./internal/gateway -run TestGatewayDeliversPublishedMessageToConnectedDevice -count=1`：通过
- `go test ./internal/gateway -run TestGatewayForwardsReadAckToMessageService -count=1`：通过
- `go test ./internal/gateway -run 'TestGateway(ReplacesOldSessionWithoutDroppingNewRoute|HeartbeatRefreshesUserAndNodeRouteTTL)' -count=1`：通过
- `go test ./internal/outbox -count=1`：通过
- `go test ./internal/conversations -count=1`：通过
- `go test ./internal/sync -count=1`：通过
- `go test ./internal/receipts -run TestProcessReadAckCreatesOutboxAndProjectsReadState -count=1`：通过
- `make dev`：在当前环境失败，原因是缺少 `docker` 命令
- 本机服务模式：通过
- `./scripts/phase1-smoke.sh`：通过
- `make cleanup-sync-events`：可手动触发 `sync_events` 保留期清理

这组结果说明：

- HTTP 主链路已经具备可重复的 smoke 级验证能力
- Gateway 已具备基于 `GatewayDeliveryService.DeliverMessage` gRPC 的实时投递能力，并有一条聚焦 `MESSAGE_DELIVERY` 的自动化测试
- ACK(read) 已具备最小闭环：Gateway 会通过 `MessageAckService.ProcessAck` gRPC 上送到 Message Service，`read_receipt` 会生成 outbox 事件并投影为 sync events
- Docker 不是当前环境下稳定可用的默认路径
- 自动化测试资产仍然很薄，尚未覆盖 ACK / read receipt / 多端一致性

原因分为两层。

### 已验证部分

- 基础依赖可启动
- 迁移可执行
- 二进制可编译
- 注册 / 登录态建立可用
- direct conversation 下的消息发送、幂等重发、会话摘要、同步事件、消息补拉这条 HTTP 主链路可验证
- Gateway 节点能够通过 gRPC `DeliverMessage` 把 Delivery 下发为 WebSocket `MESSAGE_DELIVERY` 帧
- Gateway 能把 `ACK(read)` 转发到 Message Service，且 `read_receipt` 消费后会把阅读者 `unread_count` 置 0，并写出 `message_read` / `conversation_updated` sync events
- Gateway 的旧连接替换和心跳续租已有固定自动化测试
- Outbox 的指数退避、stale processing 接管、原子领取已有固定自动化测试
- 会话摘要的成员列表、未读累计、分页与空数组行为已有固定自动化测试
- Sync API 的分页读取、latest_event_seq 返回与 cursor 单调前进已有固定自动化测试
- HTTP 请求日志已生成或透传 `trace_id`，并可通过响应头观察

### 未通过部分

- Outbox -> Fanout -> Gateway -> WebSocket 的进程级完整验收还未作为固定 runbook 关闭
- ACK / read receipt 的进程级端到端 runbook 尚未跑通，当前通过证据仍以自动化测试为主
- 多端已读收敛仍未验证“设备 2 从 50 收敛到 100”这类 MAX 规则端到端行为
- `make test` 仍然缺少覆盖主要 Phase 1 接缝的自动化测试资产

因此，当前不能声明：

- “Phase 1 已完整测试通过”
- “消息发送 -> 持久化 -> WebSocket 实时投递 -> ACK -> 已读 收敛闭环已验收完成”

## 下一步最小补齐项

若要把 Phase 1 从“部分交付已验证”推进到“整体验收通过”，最小增量应是：

1. 补一条覆盖 `Outbox -> Gateway -> WebSocket` 的进程级集成验证
2. 补齐 ACK -> Message Service -> receipt / read path
3. 为 `POST /v1/messages/send`、`GET /v1/sync/events`、`GET /v1/conversations/{cid}/messages` 增加自动化测试
4. 为 read receipt、多端已读收敛和 WebSocket 心跳补集成测试
