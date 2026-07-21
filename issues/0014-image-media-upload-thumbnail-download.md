---
id: "0014"
title: "图片消息直传 + 缩略图 + 授权下载"
status: complete
labels: ["done"]
parent: "0010"
blocked_by: []
created_at: 2026-07-20
updated_at: 2026-07-21
---

# 0014 — 图片消息直传 + 缩略图 + 授权下载

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

落地 Spec 08 的 P0 图片消息闭环：上传凭证签发、客户端直传对象存储、服务端记录 Attachment 元数据并与 Message 关联、异步缩略图生成、接收方授权下载原图。P0 使用本地 MinIO（开发环境）或本地文件系统作为对象存储，不引入真实 S3/CDN。

端到端行为：User A 请求上传凭证 → 服务端签发 `upload_id` + 分片预签名 URL → A 分片上传图片到对象存储 → 上传完成后 A 发送一条 `message_type: "image"` 的 Message，content 中含 `attachment_id` 和元数据 → 缩略图 worker 异步从对象存储读取原图生成缩略图 → 缩略图生成完成后更新 `attachments.upload_status = 'complete'` → User B 收到消息时看到缩略图元数据（含 BlurHash 占位） → B 调用下载授权接口获得带签名的临时下载 URL → 24 小时未完成的上传被定时清理。

## Acceptance criteria

- [ ] `POST /v1/media/upload/initiate` 接受 `{mime_type, size_bytes, file_name, width, height}`，返回 `{upload_id, object_key, chunk_size, presigned_urls[], expires_at_ms}`；大小上限 50MB
- [ ] 上传中断后，`GET /v1/media/upload/{upload_id}/status` 返回 `{status, total_parts, completed_parts}`；这部分通过对象存储的 multipart upload API 查询
- [ ] 上传完成后，客户端发送 `message_type: "image"` 消息，消息 content 中携带 attachment 元数据（`{attachment_id, object_key, mime_type, size_bytes, width, height, thumbnail_key, blur_hash}`）
- [ ] `POST /v1/messages/send` 对 image 类型消息校验 attachment 字段完整性
- [ ] 缩略图 worker：从对象存储读取原图 → 生成 320px 宽度缩略图（保持比例，JPEG quality 80%）→ 写回对象存储 → 更新 `attachments.upload_status = 'complete'` 和 `thumbnail_key`
- [ ] 缩略图生成失败时，`upload_status` 更新为 `failed`，不影响消息本身可见（接收方看到原始元数据 + 占位图）
- [ ] `POST /v1/media/download/auth` 接受 `{object_key, conversation_id}`，校验请求者是否为会话成员，返回 `{download_url, expires_in_sec, content_type, content_length}`
- [ ] 授权下载 URL 包含 HMAC 签名 + 过期时间，对象存储侧验证签名后放行
- [ ] orphan 清理：定时任务（每 10 分钟）扫描 `attachments WHERE upload_status = 'pending' AND created_at < NOW() - INTERVAL '24 hours'`，标记为 `orphan`
- [ ] 消息的 `message_type` 从 Phase 1 的仅 `text` 扩展到 `text` 和 `image`（在 domain/types.go 中新增常量）

## Blocked by

None — can start immediately（技术上不依赖 0011，但联调验证建议在 0011 之后进行）。

## 技术难点与注意事项

### 1. 对象存储选择与抽象

**问题：** P0 开发环境没有 S3。需要既能本地运行又不绑死具体实现。

**方案：** 抽象 `ObjectStore` 接口：
```
type ObjectStore interface {
    InitiateMultipartUpload(ctx context.Context, key string, contentType string) (uploadID string, err error)
    PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int, expires time.Duration) (url string, err error)
    CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []Part) error
    ListCompletedParts(ctx context.Context, key, uploadID string) ([]Part, error)
    PresignDownload(ctx context.Context, key string, expires time.Duration) (url string, err error)
    GetObject(ctx context.Context, key string) ([]byte, error)
    PutObject(ctx context.Context, key string, data []byte, contentType string) error
}
```

**P0 实现：** 本地文件系统实现 `LocalObjectStore`，对象存储路径为 `data/storage/`。`Presign*` 方法生成带 HMAC 签名的特殊 URL，由 `media_handler` 中的 download handler 校验签名后直接读文件。

**坑点：** 本地文件系统的 "presigned URL" 只是带签名的 HTTP URL 指向 message-service 的下载端点，不是真正的 S3 presigned URL。这是 P0 简化，P1 切换到 MinIO/S3 时替换 `ObjectStore` 实现即可，接口不变。

### 2. 缩略图生成的异步模型

**问题：** Phase 1 没有异步任务系统（Outbox consumer 是轮询 DB，但不适合处理大文件）。缩略图生成是 CPU/IO 密集操作，不能阻塞 HTTP 请求。

**方案：** 新增一个轻量级 `thumbnail-worker` goroutine，在主进程中启动（不独立为进程）。通过 channel 接收任务：
```
type ThumbnailJob struct {
    AttachmentID int64
    ObjectKey    string
    MimeType     string
}
```
worker 从 channel 读取 → 从 ObjectStore 下载原图 → `imaging` 库生成缩略图 → 写回 ObjectStore → 更新 DB。

**坑点：** goroutine panic 会导致 worker 挂掉。需要 recover + 重试 + 失败回写 DB。如果进程重启，pending 状态的 attachment 会被 orphan 清理任务处理，不会永久卡住。

### 3. BlurHash 生成

**问题：** 消息元数据中的 `blur_hash` 用于客户端即时展示模糊占位图。服务端生成 BlurHash 需要在缩略图生成时完成。

**方案：** 缩略图 worker 在生成缩略图后计算 BlurHash。Go 生态没有原生 BlurHash 库，P0 简化：使用纯色占位（`blur_hash` 字段暂存空字符串），客户端用灰色占位。P1 再引入 BlurHash。

**替代方案：** 引入 `github.com/bbrks/go-blurhash` 第三方库，但增加了依赖。P0 接受空 blur_hash。

### 4. 文件上传大小限制

**问题：** 用户可能上传超大文件。P0 只支持图片（JPEG/PNG/WebP），上限 50MB。

**方案：**
- `upload/initiate` handler 校验 `size_bytes ≤ 50 * 1024 * 1024`
- `mime_type` 必须匹配 `image/jpeg`、`image/png`、`image/webp`
- 分片大小固定 5MB（与 Spec 08 一致）

### 5. 分片上传的事务性

**问题：** 用户可能在上传分片中途放弃。对象存储侧有 orphan 分片。

**方案：**
- 对象存储（本地文件系统）：每个 upload 用一个目录 `data/storage/uploads/{upload_id}/`，分片写入 `part_0001`、`part_0002`...
- orphan 清理任务同时清理：1) DB 中 24 小时未完成的 attachment 记录，2) 文件系统中超过 24 小时的 upload 目录

### 6. 涉及的关键文件

- `internal/media/` — 新包：objectstore.go（接口+本地实现）、thumbnail.go（worker）、upload.go（handler 逻辑）
- `internal/api/router.go` — 新端点：upload/initiate、upload/status、download/auth
- `internal/api/media_handler.go` — 新媒体 handler
- `internal/messages/service.go` — 扩展 SendMessage 支持 image 类型 + attachment 校验
- `internal/domain/types.go` — 补 Attachment 结构体、MessageTypeImage 常量
- `migrations/` — attachments 表已存在，可能需要加 blur_hash 列
- `cmd/message-service/main.go` — 启动 thumbnail worker goroutine
