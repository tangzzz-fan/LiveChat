---
id: "0016"
title: "安全基线加固与审计收敛"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: ["0011"]
created_at: 2026-07-21
---

# 0016 — 安全基线加固与审计收敛

## Parent

Phase 3: 规模化与工程质量（P0/P1 交界），对应 Spec 10。

## What to build

对 Phase 1 + Phase 2 的服务端代码执行一次系统性的安全基线加固：强制 TLS 配置、补齐安全响应头、日志脱敏覆盖所有敏感字段、上传/下载授权签名升级、审计事件收敛到统一模块、最小安全基线 checklist 全部具备可自动化验证的检查手段。

端到端行为：部署后的服务端在公网环境下通过 TLS 1.2+ 提供 HTTPS/WSS → 所有错误响应和日志输出中不包含完整 Token、手机号明文、IP 地址 → 上传接口校验 MIME 白名单和大小上限、下载链接必须携带有效 HMAC 签名 → 关键安全事件（登录、登出、设备增删、Token 刷新）自动写入审计表 → 运维可通过脚本或 CI 检查安全基线 checklist 是否全部通过。

## Acceptance criteria

- [ ] `livechat-server` 支持 TLS 配置：`configs/` 增加 `tls.cert_file`、`tls.key_file` 配置项；开发环境可使用自签名证书（提供生成脚本 `scripts/gen-dev-cert.sh`）
- [ ] Gateway 和 Message Service 在 TLS 模式下强制最低 TLS 1.2，禁用 TLS 1.0/1.1
- [ ] HTTP 安全头中间件：所有响应携带 `Strict-Transport-Security`、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`
- [ ] 日志脱敏中间件/hook：slog handler 自动将 `phone_e164` 脱敏为 `+86138****0001`，将 Token 脱敏为 `eyJh***.eyJh***.****` 仅保留前 8 字符 + 后缀标识
- [ ] 审计事件收敛为统一模块 `internal/audit/`：
  - `audit.Record(ctx, db, event)` 统一接口
  - 事件类型常量对齐 Spec 10 §8（login_success / login_failed / code_request / device_added / device_revoked / token_refreshed / token_replay_detected / security_alert）
  - 替代 Phase 1 中分散在各 handler 中的审计写入
- [ ] 上传接口补齐服务端侧 MIME 校验：白名单 `image/jpeg, image/png, image/webp`，拒绝其他类型返回 415
- [ ] 上传文件大小在服务端侧二次校验：超过 50MB 返回 413
- [ ] 下载授权 HMAC 签名：`download_url` 格式改为 `?key={}&user={}&expires={}&sig={}`，`sig = HMAC-SHA256(key + user + expires, secret)`；download handler 校验签名是否有效且未过期
- [ ] 最小安全基线 checklist（Spec 10 §5）的 10 项均有对应的验证方式：部分通过自动化检查脚本 `scripts/security-baseline-check.sh`，部分通过配置审查
- [ ] `POST /v1/auth/request_code` 增加单 IP 频控：>20 次/小时返回 429（复用 Phase 2 0015 将要建立的限流基础设施，或使用 Redis 滑动窗口）

> **注意：** `login_audit_events` 表预期由 0011（认证收敛）创建。如果 0011 实施时未创建此表，本票需自行补齐 DDL。

## Blocked by

None — can start immediately.

## 技术难点与注意事项

### 1. TLS 证书的开发与部署

**问题：** 本地开发不能依赖公开 CA 签发的证书，自签名证书会导致 curl/浏览器信任问题。

**方案：** 
- 提供 `scripts/gen-dev-cert.sh`：使用 `openssl` 生成自签名 CA + 服务器证书
- 仓库不提交 `.pem`/`.key` 文件到 git（`.gitignore` 排除）
- `configs/config.dev.yaml` 中 `tls.enabled: false` 默认关闭 TLS，生产 `configs/config.prod.yaml` 强制 `tls.enabled: true`
- 集成测试不验证 TLS 证书链（`InsecureSkipVerify` 仅测试环境）

**坑点：** iOS App 的 ATS（App Transport Security）要求 HTTPS 证书必须是受信任 CA 签发或配置 exception domains。测试阶段需要在 Info.plist 中配置 `NSAppTransportSecurity` 例外。

### 2. 日志脱敏的实现位置

**问题：** 脱敏需要在日志输出的最后一道关卡统一做，而不是每个 `slog.Info("...", "phone", phone)` 调用点手动脱敏。

**方案：** 实现自定义 `slog.Handler`，在 `Handle()` 方法中检查每条 log record 的 attrs：
- 如果 key 包含 `phone`、`token`、`ip_address` → 自动脱敏 value
- 如果 value 是结构体（如 `Claims`）→ 通过反射检查字段 tag 是否需要脱敏

**坑点：** 
- 不要用正则扫描日志文本做脱敏——应该在结构化 attrs 层面处理。日志文本匹配不可靠（Token 可能出现在 message 字符串中而未被捕获）。
- 已经有 `traceutil` 的习惯做法，新的 `slog.Handler` wrapper 应该保持兼容。

### 3. 审计模块的 DB 写入不应阻塞业务请求

**问题：** 每个安全事件都同步写 `login_audit_events` 表会拖慢登录/刷新等接口。

**方案：**
- `audit.Record()` 内部用 buffered channel + 后台 goroutine 批量写入
- channel 容量 1024，满则丢弃并记录 `audit_queue_full` 错误（审计可以丢少量，但不能丢业务）
- 可选：P0 也可以同步写（审计量不大），P1 改异步

### 4. HMAC 签名 vs 简单的 token 校验

**问题：** Phase 1 的下载没有签名机制（因为还没有媒体模块）。Phase 2 0014 实现下载端点时需要签名，本票统一规范。

**方案：**
- 签名密钥从 config 读取：`media.download_sign_secret`
- `sig = HMAC-SHA256(key|user_id|expires_at, secret)`（注意：expires_at 用 unix 秒，不是毫秒）
- 服务端收到下载请求时，从 query params 取 `key, user, expires, sig`，重新计算 HMAC 并做常量时间比较

**坑点：** HMAC 比较必须用 `crypto/subtle.ConstantTimeCompare`，不能用 `==`——否则有时序攻击风险。

### 5. 现有 JWT 签名的 RS256 升级（P0 可选/P1）

**问题：** Phase 1 使用 HMAC-SHA256（对称签名），Spec 10 要求 RS256（非对称签名）。升级会影响所有已有 JWT。

**方案（P0 简化）：** 本票保持 HMAC-SHA256，但在 `configs/` 中预留 `auth.jwt_public_key` 和 `auth.jwt_private_key` 配置项，`auth.Service` 的 `SignAccessToken` 和 `VerifyAccessToken` 改为通过接口抽象，支持未来切换算法。RS256 升级作为 P1 独立 ticket。

### 6. 涉及的关键文件

- `internal/api/middleware.go` — 新增安全头中间件 + 日志脱敏中间件
- `internal/audit/audit.go` — 新包：统一审计接口
- `internal/media/` — 补齐 MIME/大小校验（在 upload handler 中）
- `configs/` — 新增 TLS 配置项 + 签名密钥配置项
- `scripts/gen-dev-cert.sh` — 新脚本
- `scripts/security-baseline-check.sh` — 新脚本
- `migrations/` — login_audit_events 表（如果 0011 尚未创建）
