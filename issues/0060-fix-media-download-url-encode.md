---
id: "0060"
title: "修复图片下载 URL 编码：接收方无法展示对端图片"
status: complete
labels: ["done", "p0", "bug"]
blocked_by: []
created_at: 2026-07-27
---

# 0060 — 图片下载 URL 编码导致接收方无法展示

## Parent

回归自 [0049](0049-ios-image-message.md) / [0014](0014-image-media-upload-thumbnail-download.md)。0049 验收时本端靠上传缓存可见，掩盖了接收方 download 路径。

## Notes

**现象**：A 发送图片后，A 能看到（本地 `ImageMediaCache`），B 气泡显示加载失败 / 无法展示。

**根因**：`LocalObjectStore` 用 `/`↔`_` 伪编码 object_key。真实 key 形如  
`media/u_1201/2026/07/27/img_1785163851196381000_photo.jpg`，文件名自带 `_`，解码后变成  
`media/u/1201/.../img/1785.../photo.jpg`，HMAC 用错 key → `GET /media/download/...` 返回 `403 invalid signature`。

**复现（红灯命令）**：

```bash
# auth → download/auth → GET download_url
# 修复前：HTTP 403 {"error":"invalid signature"}
# 修复后：HTTP 200 + image/jpeg bytes
```

**后端：阻塞接收展示。** 客户端无需改协议（仍消费服务端返回的 `download_url`）。

## What to build

- [x] `urlEncode` / `urlDecode` 改为可逆编码（base64url）
- [x] 回归测试：含 `_` 的 object_key 经 Presign → ServeDownload 必须 200
- [x] 更新 `docs/engineering-problems/13-download-url-hmac-signing.md` 踩坑
- [x] 重启本机 message-service；HTTP 端到端 download 已绿

## Acceptance criteria

- [x] `go test ./internal/media/ -count=1` 覆盖 underscore-rich key roundtrip
- [x] 本机：用户 B 对 A 已发图片执行 download/auth + GET → 200（1741446 bytes JPEG）
- [ ] iOS HITL：B 打开会话可看到 A 发的图片气泡（客户端无改动，依赖服务端 URL；请确认一次）

## Blocked by

None
