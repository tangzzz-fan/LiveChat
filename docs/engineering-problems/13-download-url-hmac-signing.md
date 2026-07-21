# 下载 URL 的签名安全：为什么不能直接暴露对象存储路径

标签: `security`, `durability`

## 问题是什么

媒体下载 URL 如果直接指向对象存储路径（如 `/media/u_123/2026/07/img.jpg`），任何知道 URL 的人都能访问文件——即使他们已经离开会话、已被移出群、或者从未参加过该会话。

**核心矛盾：** 对象存储的访问控制通常是 bucket 级别的，但聊天系统的访问控制是 conversation 级别的（只有会话成员可以查看该会话的媒体）。

## 典型场景

```
1. User A 在群聊中发送一张图片 → 图片存储在 S3: media/u_A/img001.jpg
2. User B 通过 API 获取了下载 URL: /media/u_A/img001.jpg
3. User B 被移出群 → B 不能再访问群的任何内容
4. 但 B 已经记住了 /media/u_A/img001.jpg → 仍然可以访问这张图片
5. URL 被分享到群外 → 任何有链接的人都能看到
```

## 通用分析思路

1. **访问控制必须在每次请求时校验**。不能依赖"URL 没人知道"的安全假设。
2. **签名 URL 是标准方案**。URL 中嵌入 HMAC 签名 + 过期时间，服务端在每次请求时校验。
3. **签名密钥不暴露给客户端**。签名在服务端生成，客户端只拿到结果 URL。

## 当前项目方案

LiveChat 采用 HMAC 签名 URL（0014 + 0016）：

### 下载授权流程

```
POST /v1/media/download/auth {object_key, conversation_id}
  → 校验: 请求者 ∈ conversation_members
  → 生成签名: sig = HMAC-SHA256(object_key|user_id|expires_at, secret)
  → 返回: /media/download/{object_key}?user={user_id}&exp={exp}&sig={sig}
  → 客户端用此 URL 下载

GET /media/download/{object_key}?user={}&exp={}&sig={}
  → 校验: exp > now
  → 重新计算 HMAC，常量时间比较
  → 读取文件并返回
```

### 签名生成（服务端）

```go
payload := fmt.Sprintf("%s|%d|%d", objectKey, userID, expiresAt)
mac := hmac.New(sha256.New, []byte(downloadSignSecret))
mac.Write([]byte(payload))
sig := hex.EncodeToString(mac.Sum(nil))
```

### 签名校验（下载端点）

```go
expected := computeHmac(objectKey, userID, expiresAt, secret)
if !hmac.Equal([]byte(expected), []byte(providedSig)) {
    return 403
}
if time.Now().Unix() > expiresAt {
    return 410  // Gone
}
```

**为什么用 `hmac.Equal` 而不是 `==`？**
- `==` 会在第一个不同字符处短路返回，攻击者可以通过计时推断签名的前缀
- `hmac.Equal` 是常量时间比较，无论是否匹配都执行相同数量的操作

## 替代方案及取舍

| 方案 | 安全性 | 复杂度 | LiveChat 选择 |
|------|--------|--------|--------------|
| 直接暴露路径（无签名） | ❌ | 低 | 不采用 |
| HMAC 签名 + 过期时间 | ✅ | 中 | **✅ P0 采用** |
| S3 Presigned URL（委托 S3） | ✅ | 低（依赖 S3） | P1（需 S3/MinIO） |
| OAuth2 Bearer Token | ✅ | 高 | 未采用 |

**P0 为什么不用 S3 Presigned URL？**
- P0 开发环境没有 S3/MinIO——使用本地文件系统
- `LocalObjectStore` 的 "Presigned URL" 实际上是带 HMAC 签名的 HTTP 端点
- P1 切换到 MinIO/S3 时，可以直接用 S3 SDK 的 presigned URL，`ObjectStore` 接口不变

## 踩坑记录

1. **签名密钥必须从 config 读取，不能硬编码**：`media.download_sign_secret` 在 `configs/message-service.yaml` 中配置。生产环境必须更换默认值。
2. **expires_at 用 unix 秒，不是毫秒**：HMAC 签名对输入敏感，`exp=1752681600` 和 `exp=1752681600000` 是不同的签名。
3. **不要对签名参数做 URL 编码**：签名是 hex 字符串（`[0-9a-f]`），天然 URL-safe。URL 编码只会让校验复杂化。

### 代码位置

- `internal/media/service.go` → AuthorizeDownload（权限校验 + 签名生成）
- `internal/media/service.go` → ServeDownload（签名校验 + 文件流）
- `configs/message-service.yaml` → `media.download_sign_secret`
