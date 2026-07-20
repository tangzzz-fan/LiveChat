# Domain Docs

本仓库按 single-context 仓库处理。工程技能在探索代码库或规格库时，优先按以下顺序读取文档。

## Before exploring, read these

- 根目录 `CONTEXT.md`
- `docs/adr/` 下与当前主题相关的 ADR
- `Specs/` 下与当前任务直接相关的规格文档

如果某个区域还没有 ADR，继续执行，不需要因为缺少 ADR 中断。

## File structure

```text
/
├── CONTEXT.md
├── Specs/
├── docs/
│   └── adr/
└── ...
```

## Vocabulary rules

- 输出中涉及领域概念时，优先使用 `CONTEXT.md` 中已经定义的术语。
- 规格、代码、issue、测试、ADR 应尽量保持同一套名词，不要在 `Conversation`、`Chat`、`Thread` 之间反复漂移。
- 如果发现现有术语不足以表达新概念，再通过 `domain-modeling` 补充。

## ADR rules

- 重要的、难以逆转的架构决策写入 `docs/adr/`
- 新 ADR 使用递增编号，例如 `0001-xxx.md`
- 新实现如果与现有 ADR 冲突，必须显式指出冲突，而不是静默覆盖
