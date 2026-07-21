# 两步认证的状态管理：验证码不能放在客户端回传

标签: `security`, `idempotency`

## 问题是什么

手机号验证的流程是：`request_code`（请求验证码）→ `verify_code`（提交验证码）。这两个请求是独立的 HTTP 调用，服务端需要在两个请求之间记住"这个手机号请求过验证码、验证码是什么、还剩几次尝试机会"。

**关键安全约束：验证码本身绝对不能放在 JWT 或其他客户端可解码的 token 里让客户端回传。** 如果客户端可以读到验证码，那么任何知道 JWT 结构的人都可以伪造验证。

## 典型场景

```
客户端: POST /auth/request_code {"phone": "+8613800138000"}
服务端: 生成 code = rand(100000, 999999)，发送短信
        返回 200 {retry_after: 30s}

客户端: POST /auth/verify_code {"phone": "...", "code": "123456"}
服务端: 比对存储的 code，如果匹配 → 签发 JWT
```

如果服务端把 `code=123456` 放在 JWT 里返回给客户端：
```
❌ 错误方案: JWT {phone, code, exp} → 客户端可以解码看到 code
                                  → 攻击者也可以解码看到 code
```

## 通用分析思路

1. **验证码的机密性要求**：只有服务端（和用户手机短信）应该知道正确答案
2. **服务端状态 vs 客户端状态**：验证码必须存在服务端，用 Redis/DB 存储
3. **TTL 管理**：验证码有时效性（通常 5 分钟），存储时自动过期
4. **尝试次数限制**：防止暴力破解（3 次失败 → 需重新请求）

## 当前项目方案

LiveChat 使用 Redis 存储验证码（0011 实现）：

```
Redis Key:     code:{phone_e164}
Value:         "123456"  (mock OTP — P0 开发阶段)
TTL:           5 minutes
尝试限制:      code 用后立即 DEL（一次性使用）
```

**Mock OTP 策略：**
- P0 开发阶段：固定验证码 `"123456"`，不实际发送短信
- `verify_code` handler 只检查 Redis 中有没有这个 key（表示用户确实先请求过 code）
- 无论客户端传什么 code，只要 Redis key 存在就通过
- 这避免了开发时需要真实短信服务，但保持了正确的两步认证流程

**频控：**
```
phone 维度: 每个号码 3 次/小时 (Redis: rate:phone:{phone}, TTL 1h)
IP 维度:    每个 IP 20 次/小时  (Redis: rate:ip:{ip}, TTL 1h)
```

## 替代方案及取舍

| 方案 | 安全性 | 复杂度 | 外部依赖 | LiveChat 选择 |
|------|--------|--------|---------|--------------|
| JWT 携带 code（错误） | ❌ 客户端可读 | 低 | 无 | 不采用 |
| Redis 存储 | ✅ | 低 | Redis | **✅ P0 采用** |
| DB 存储 | ✅ | 中 | DB | 备选（无 Redis 时） |
| 无状态 TOTP | ✅ | 高 | 客户端+服务端时钟同步 | 未采用 |

## 踩坑记录

1. **验证码不删除导致无限重试**：0011 实现初期忘记 `redis.DEL(codeKey)`，导致同一个 code 可以反复使用。修复：`verify_code` handler 在读取 code 后立即 `rdb.Del(ctx, codeKey)`。
2. **phone_e164 作为 Redis key 的编码问题**：E.164 号码包含 `+` 号，如 `+8613800138000`。Redis key 可以包含 `+`，但要注意 URL 编码和日志脱敏。
3. **P0 的 mock code 不等于生产方案**：当前 mock 接受任意 6 位数字，没有真正生成随机 code。这是一个**明确的 P0 简化边界**——真实 code 生成和短信发送需要接入 SMS 服务商，属于 P1 扩展。

### 代码位置

- `internal/api/router.go` → handleRequestCode（Redis SET code + 频控）
- `internal/api/router.go` → handleVerifyCode（Redis GET/DEL code）
- `internal/api/router.go` → e164RE（E.164 格式校验正则）
