# Swift Protobuf 生成入库与版本对齐

标签: `connection`, `observability`

## 问题是什么

网关帧的 schema 唯一来源是 `livechat-server/proto/ws_frame.proto`。若 iOS 手写字段、或生成物与链接的 `SwiftProtobuf` / `protoc-gen-swift` 版本漂移，会出现：**能编译但握手解失败**、或生成代码用了新插件语法而本地 SPM 插件过旧。

## 典型场景

- 改了 Go proto，忘了重跑 iOS `gen_proto.sh`，客户端仍按旧字段解码。  
- `brew install protobuf swift-protobuf` 得到的 `protoc-gen-swift`（如 1.38）生成带 `nonisolated` / `FoundationEssentials` 的代码，而 Package 链的是本地 `TG Libraries/swift-protobuf` 1.31 —— 多数情况仍可编，但升级时需回归握手。  
- 脚本续行符后有空格，`protoc \` 断行失败，表现为诡异的 `-proto_path: No such file`。  
- 生成到 `ios/Generated/` 却未编进 target，运行时仍在用 `ProtobufScaffold` 空壳。

## 通用分析思路

1. **单一 schema 源** → 生成物是衍生品，禁止手改 `.pb.swift` 业务含义。  
2. **生成工具版本**与 **运行时库版本**尽量同系列；至少保证 `ProtobufAPIVersion` check 通过。  
3. **入库策略**：小团队学习仓适合 **生成物 commit**，CI/他人无需装 protoc 也能编。  
4. 用**黄金路径对照**：压测 Python 客户端与 iOS 必须能跟同一 gateway 握手。

## 当前项目方案

| 项 | 位置 |
|----|------|
| Schema | `livechat-server/proto/ws_frame.proto` |
| iOS 脚本 | `ios/scripts/gen_proto.sh`（单行 `protoc` 调用，避免 `\` 续行坑） |
| 输出（入库） | `ios/Packages/ChatInfrastructure/Sources/ChatInfrastructure/Generated/ws_frame.pb.swift` |
| 运行时 | SPM path：`TG Libraries/swift-protobuf` |
| 工具（仅生成时） | `brew install protobuf swift-protobuf`；需要外网时 `proxy_on` |
| 压测对照 | `load_test/gen_proto.sh` → Python；`load_test/core/ws_protocol.py` |

生成命令：

```bash
proxy_on   # 若 brew/下载需要代理
brew install protobuf swift-protobuf   # 首次
./ios/scripts/gen_proto.sh
```

编码入口：`WsCodec`（opcode 常量与 Go `internal/gateway/frame.go` 对齐）。

冒烟：`ProtobufScaffold.libraryLinked` 触碰 `Livechat_Ws_WsFrame`。

## 替代方案及取舍

| 方案 | 优点 | 代价 |
|------|------|------|
| 生成物入库（当前） | 克隆即编；审查可见协议快照 | proto 变更要记得跑脚本并提交 |
| SPM plugin 构建时生成 | 无生成物 diff | 每人/CI 都要 protoc；Xcode 插件配置脆 |
| 手写 Codable 镜像 | 无 protoc | 易与 Go 漂移；0041 明确禁止 |

## 踩坑记录

- 0041：`gen_proto.sh` 多行 `protoc \` 曾因续行问题失败；改为**一行参数**。  
- 输出目录必须落在 `ChatInfrastructure` target 源码树下，SPM 才会编译；旧注释里的 `ios/Generated/` 仅作历史路径，脚本会尝试清理。  
- Empty `Heartbeat` 序列化可能是 **0 字节** —— 测试不要断言 `!data.isEmpty`；用 round-trip handshake 测编解码。  
- 协议变更后：iOS 与 `load_test` **都要**重新 gen，否则一边握手成功一边「静默解不出」难查。
