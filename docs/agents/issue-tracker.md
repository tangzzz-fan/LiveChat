# Issue tracker: Local

本仓库的 issue、任务拆解和需求跟踪使用本地 `issues/` 目录。说明本仓库不依赖 GitHub Issues 作为主跟踪系统。

## Conventions

- **Issue 文件格式**: `issues/XXXX-name.md`，XXXX 为递增编号
- **Issue index**: `issues/INDEX.md` 列出所有 issue 及其状态和标签
- **状态流转**: `draft` → `ready-for-agent` → `in-progress` → `done`

## Issue 文件模板

每个 issue 文件使用 markdown，文件头为 YAML frontmatter:

```yaml
---
id: XXXX
title: "..."
status: draft
labels: ["needs-triage"]
created_at: YYYY-MM-DD
---
```

## When a skill says "publish to the issue tracker"

在 `issues/` 目录下创建新 issue 文件，更新 `INDEX.md`，应用 `ready-for-agent` 标签。

## When a skill says "fetch the relevant ticket"

读取 `issues/` 目录下对应的 issue 文件。
