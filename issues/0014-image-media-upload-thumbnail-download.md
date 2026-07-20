---
id: "0014"
title: "图片消息直传 + 缩略图 + 授权下载"
status: ready-for-agent
labels: ["ready-for-agent"]
parent: "0010"
blocked_by: []
created_at: 2026-07-20
---

## Parent

[0010 - 阶段二：用户可感知能力](0010-phase-2-user-visible-capabilities.md)

## What to build

落地 `Spec 08` 的 P0 图片消息闭环：上传凭证签发、客户端分片直传、服务端记录 `Attachment` 元数据、异步生成缩略图、接收方授权下载原图。

端到端行为：User A 请求上传凭证并分片上传图片 → 上传完成后发送一条带 `Attachment` 的 `Message` → User B 收到消息时先看到缩略图或缩略图元数据，再按需拿到授权下载链接获取原图；上传中断后，客户端可以从已完成分片继续。

具体交付：

- `POST /v1/media/upload/initiate`
- `GET /v1/media/upload/{upload_id}/status`
- 分片完成后的上传确认接口
- `Attachment` 元数据落库，并与消息关联
- 缩略图生成 worker 与媒体状态机
- `POST /v1/media/download/auth` 授权下载
- 未完成上传的 orphan 清理任务

## Acceptance criteria

- [ ] `POST /v1/media/upload/initiate` 返回 `upload_id`、`object_key`、分片大小和预签名地址
- [ ] 上传中断后，`GET /v1/media/upload/{upload_id}/status` 能返回已完成分片列表
- [ ] 上传完成后，发送图片消息时消息体能携带 `Attachment` 元数据，并保留 `mime_type`、尺寸、大小和 `object_key`
- [ ] 缩略图 worker 成功生成至少一种缩略图，并将状态推进到 `complete`
- [ ] 接收方查询消息时能看到图片消息及其缩略图元数据
- [ ] `POST /v1/media/download/auth` 仅对会话成员返回有效下载链接
- [ ] 24 小时未完成的上传会被清理为 orphan 或等价失效状态

## Blocked by

None - can start immediately.
