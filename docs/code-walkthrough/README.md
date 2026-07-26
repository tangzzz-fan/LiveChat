# 源码导读（code-walkthrough）

对照 `livechat-server/internal/` 的包级阅读笔记，面向 Go 基础偏弱的复盘学习。  
总入口仍见：[Go 后端技术栈与学习导读](../Go后端技术栈与学习导读.md)。

| # | 包 | 文档 | 状态 |
|---|-----|------|------|
| 01 | `internal/messages` | [01-messages.md](01-messages.md) | 已写 |
| 02 | `internal/outbox` | [02-outbox.md](02-outbox.md) | 已写 |
| — | `gateway` / `fanout` / `api` … | 后续按需追加 | 待写 |

阅读约定：

1. 先扫「一句话职责」与流程图  
2. 打开对应 `.go` 文件，按「建议断点」跟一遍  
3. 回到「Go 语言点」巩固语法在本仓的用法  
4. 相关 Spec / 工程问题用来回答「为什么」  
