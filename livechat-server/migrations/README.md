# LiveChat Server — Migration Guide

## Overview

LiveChat Server 使用基于文件的迁移工具（`cmd/migrate`），在 `migrations/` 目录中管理所有 DDL 变更。

## Adding a Migration

1. Create numbered up/down files in `migrations/`:

```bash
touch migrations/009_descriptive_name.up.sql
touch migrations/009_descriptive_name.down.sql
```

2. Write the up migration (DDL to apply):

```sql
-- migrations/009_example.up.sql
CREATE TABLE IF NOT EXISTS example (
    id BIGSERIAL PRIMARY KEY,
    ...
);
```

3. Write the down migration (rollback):

```sql
-- migrations/009_example.down.sql
DROP TABLE IF EXISTS example;
```

4. Run:

```bash
go run ./cmd/migrate up
```

## Conventions

- Migration numbers are sequential and NEVER reused
- Once a migration has been applied to any environment, its SQL MUST NOT be modified; create a new migration instead
- Each `.up.sql` file must have a corresponding `.down.sql`
- Use `IF EXISTS` / `IF NOT EXISTS` to make migrations idempotent
- The `schema_migrations` table tracks which migrations have been applied
- Phase 1 applied 001-008. P0 auth service without rate limiting is already in place
