# 0002: 安全基线策略 — JWT HMAC vs RS256 签名方案选择

## 状态

已采用（2026-07-21）

## 背景

Phase 1 使用 HMAC-SHA256（对称签名）签发 JWT。Spec 10 要求 RS256（非对称签名）。Phase 3 P0 需要决定是否升级签名算法。

## 决策

**P0 保持 HMAC-SHA256，预留 RS256 接口。**

## 理由

1. **单服务架构不需要非对称签名**：当前 message-service 和 gateway 共享同一个 `jwt_secret`，HMAC 已经足够。只有引入第三方服务验证 JWT 时（如独立的 auth service），非对称签名才有价值。
2. **升级成本不低**：RS256 需要生成密钥对、管理公钥分发、处理密钥轮换。这些是"真实生产需求"，但当前项目的学习目标在其他模块。
3. **接口预留**：`auth.Service` 的 `SignAccessToken` 和 `VerifyAccessToken` 签名保持简单，未来切换到 RS256 时只需换 `SigningMethodRS256`。

## 影响

- JWT 格式保持不变（HMAC-SHA256）
- 生产环境部署时，`jwt_secret` 必须从环境变量读取，不硬编码在配置文件中
- P1 升级到 RS256 时，所有已有 JWT 失效（签名算法不同），需停机切换

## 相关

- Spec 10 §4.2
- Ticket 0016
