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
PGPASSWORD=livechat psql -h localhost -U livechat -d livechat -c 'select 1;'
redis-cli -h localhost -p 6379 ping
make migrate-up
```

然后分别在 3 个独立终端中启动：

```bash
make run-message-service
make run-gateway
make run-outbox-consumer
```

本次实测结果：

- `PGPASSWORD=livechat psql -h localhost -U livechat -d livechat -c 'select 1;'`：通过
- `redis-cli -h localhost -p 6379 ping`：返回 `PONG`
- `make migrate-up`：通过，当前输出为 `all migrations complete`
- `make run-message-service`：通过，监听 `:8080` 和 gRPC `:9090`
- `make run-gateway`：通过，监听 `:8081` 和 gRPC `:9091`
- `make run-outbox-consumer`：通过，启动 worker pool，并暴露 metrics `:8082`

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
curl http://localhost:8082/metrics
```

预期：

- `message-service` 返回 PostgreSQL 和 Redis 状态
- `message-service` 暴露 Prometheus 格式指标
- `gateway` 返回 `active_sessions`
- `outbox-consumer` 暴露 `outbox_pending_count`、`outbox_processing_count`、`outbox_failed_count`

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
- `./scripts/phase1-realtime-delivery.sh`：通过
- `./scripts/phase1-read-receipt.sh`：通过
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
- `phase1-realtime-delivery.sh` 已把 `Outbox -> Fanout -> gRPC Gateway -> WebSocket` 固定为可重复 runbook，并校验 delivery trace 透传
- `phase1-read-receipt.sh` 已把 `WebSocket ACK(read) -> gRPC Message Service -> Outbox Consumer -> sync_events` 固定为可重复 runbook，并校验 A 的 `message_read`、B 其他设备的 `conversation_updated`、`MAX(last_read_seq)` 收敛示例和 `/metrics` 指标名
- Gateway 的旧连接替换和心跳续租已有固定自动化测试
- Outbox 的指数退避、stale processing 接管、原子领取已有固定自动化测试
- `TestProcessEventRetryThenRecoveryMarksDoneWithoutLoss` 已固定证明：下游短暂失败时事件回到 `pending`，恢复后会被再次领取并成功落为 `done`，不会静默丢失，也不会误入 `failed`
- `ReconnectBackoffWindowGrowthAndCap`、`ReconnectBackoffDelayStaysInsideWindow`、`FastReconnectEligible` 已固定验证 `Spec 05 §6.1` 的重连退避窗口、30s 封顶和“连接存活超过 5 分钟优先快速重连”的判定
- `TestGatewayWatchdogClosesStaleSessionWithReconnectHint` 已固定验证 watchdog 超时关闭时会发出 `should_reconnect=true` 的错误帧；旧连接替换测试也会断言重连提示存在
- 会话摘要的成员列表、未读累计、分页与空数组行为已有固定自动化测试
- Sync API 的分页读取、latest_event_seq 返回与 cursor 单调前进已有固定自动化测试
- HTTP、gRPC、outbox payload 与 WebSocket frame 已具备 `trace_id` 透传

### 当前结论

- 父票 `0001` 的 6 条硬性里程碑标准现在都已有固定 runbook 或自动化测试覆盖，可以关闭
- `make test` 已包含 Outbox 重试恢复与重连退避这两条新增自动化测试资产

因此，当前可以声明：

- “Phase 1 父票的消息正确性骨架已验收通过”
- “消息发送 -> 持久化 -> WebSocket 实时投递 -> ACK -> 已读 收敛闭环已具备固定验收证据”

## 后续增量

不再阻塞 Phase 1 父票、但仍值得继续补齐的自动化资产是：

1. 为 `POST /v1/messages/send`、`GET /v1/sync/events`、`GET /v1/conversations/{cid}/messages` 继续补齐 HTTP handler 级边界自动化测试
2. 为 Gateway 的 invalid JWT、主动 `DISCONNECT`、watchdog 断链后的 Redis 清理补齐更细粒度测试
3. 为 Outbox Consumer 的并发消费与优雅退出补齐固定自动化测试
