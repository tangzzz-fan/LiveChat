---
id: "0031"
title: "压测与混沌演练硬化：stub、基线数字、chaos 04 注入"
status: complete
labels: ["complete", "p0"]
parent: "0029"
blocked_by: []
created_at: 2026-07-26
---

# 0031 — 压测与混沌演练硬化

## Parent

[0029 - 高负载 IM 验证](0029-high-load-im-validation.md)  
承接 [0019](0019-load-test-framework-baseline.md) / [0020](0020-chaos-engineering-runbooks.md) 已交付骨架。

## What to build

让 `load_test/` 五场景与 chaos 手册真正「可对照跑」：补齐 stub、加固脆弱场景、写入本机实测基线，并为 chaos 04（推送延迟）补最小注入手段。

端到端：`python load_test/run.py --quick`（或等价）五场景绿 → `load_test/baselines/` 有填数报告 → 执行 chaos 04 注入能观察到预期降级/延迟行为 → 恢复后 health-check 通过。

## Acceptance criteria

- [x] `load_test/scenarios/connect.py`：真 WebSocket upgrade（protobuf 握手按最小可用补齐）
- [x] `load_test/scenarios/sync_backfill.py`：造 cursor 落后 + 拉取 `/v1/sync/events`，输出延迟/吞吐/错误率
- [x] `load_test/scenarios/send_message.py`：去掉脆弱 `psql` 依赖；用已登录用户 + 建群或 1:1 API（若 [0026](0026-server-direct-conversation-api.md) 未就绪则用群会话 workaround，并在 README 注明）
- [x] `group_fanout`：支持可配置成员数；默认可抬到能触发热点阈值的量级；能对照 `ErrGroupBusy` / hot-group 日志
- [x] 跑一轮本机基线，把 P50/P95/错误率写入 [`local-measured-baseline.md`](../load_test/baselines/local-measured-baseline.md)（五场景实测 + 关键观察）
- [x] chaos 04：补最小 push 延迟/失败注入（env 开关或 mock provider sleep），与 [docs/chaos/04-push-delay.md](../docs/chaos/04-push-delay.md) 对齐
- [x] README / 手册命令与实际参数一致（`--max-members`、push 脚本）

## Blocked by

None（与 0030 并行）。若 0026 未完成，send 场景允许群会话 workaround。

## 技术难点与注意事项

- 共享本机 Postgres/Redis：压测与集成测试注意污染；文档说明清理方式。
- 基线是学习型本机数字，注明机器规格与 git SHA，不宣称生产容量。
- reconnect_storm 已有 jitter 参数时保持兼容，勿回退。
- 热点判定按 **60s 内消息数 > 50**（非成员数）；成员数抬高主要放大写扩散。

## 实施记录（2026-07-26）

- OTP 频控（同 IP ~20 次/小时）导致「每次迭代注册新用户」的旧实现产生 87.7% 假错误；`connect` 改为 setup 预建小用户池复用。
- Gateway 接入限流（每 IP 5 conn/s、每用户 2 conn/s）会拒绝绝大多数并发建连。这是预期行为，`connect` 场景把它计入 `throttled` 而非 `Errors`（实测 accepted=54 / throttled=4943）。
- `go run` 启动的服务会被终端会话回收；文档改为先 `make build` 再跑二进制。
- 压测依赖装在 `load_test/.venv`（本机 Python 3.9 无 httpx）。
- reconnect_storm 逐请求指标无业务意义（风暴在 setup 内测量），已在基线与手册中标注。
