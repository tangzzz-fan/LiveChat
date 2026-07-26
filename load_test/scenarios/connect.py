"""
WebSocket 连接压测。

三个前提决定了本场景的设计：
1. OTP 频控（同 IP ~20 次/小时）→ setup 预建小用户池并复用，不每次注册。
2. Gateway 接入限流（Spec 05 §6.2：每 IP 5 conn/s、每用户 2 conn/s）→ 超限拒绝是**预期行为**，
   单独计入 throttled，不记为错误。
3. upgrade 成功 ≠ 连接可用。真正可被 Fanout 投递的连接必须完成应用层 protobuf 握手，
   所以本场景把 upgrade 与握手分开计数（issue 0034）。

产出：健康握手延迟 + 三态分布（upgrade 失败 / 握手失败 / 握手成功）。
超限风暴的对照实验见 reconnect_storm 与 docs/load-practice/03。
"""
from core.client import ChatClient
from core.ws_protocol import HandshakeRejected


def _is_throttled(exc: Exception) -> bool:
    """Gateway 在 upgrade 前用 HTTP 状态拒绝时，视为接入限流而非故障。"""
    for attr in ("status_code", "status"):
        code = getattr(exc, attr, None)
        code = getattr(code, "value", code)
        if isinstance(code, int) and code in (429, 403, 503):
            return True
    text = str(exc)
    return any(marker in text for marker in ("429", "HTTP 403", "HTTP 503"))


class ConnectScenario:
    MAX_USERS = 8

    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url
        self.ws_url = ws_url
        self.client = None
        self.users = []
        self._sessions = []
        self.handshake_ok = 0
        self.handshake_rejected = 0
        self.upgrade_throttled = 0
        self.reject_reasons = {}

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        pool_n = max(1, min(count, self.MAX_USERS))
        for i in range(pool_n):
            self.users.append(await self.client.register_user(i))
        print(f"  [connect] user pool={len(self.users)} (gateway admits ~5 conn/s per IP)")

    async def execute(self, idx: int):
        user = self.users[idx % len(self.users)]
        try:
            # Heartbeat off: connections here are short-lived by design.
            session = await self.client.connect_ws(user["token"], user["device_id"], heartbeat=False)
        except HandshakeRejected as exc:
            self.handshake_rejected += 1
            key = f"{exc.code}:{exc.message}"
            self.reject_reasons[key] = self.reject_reasons.get(key, 0) + 1
            return "handshake_rejected"
        except Exception as exc:
            if _is_throttled(exc):
                self.upgrade_throttled += 1
                return "upgrade_throttled"
            raise

        self.handshake_ok += 1
        self._sessions.append(session)
        await session.close()
        self._sessions.remove(session)
        return "handshake_ok"

    async def teardown(self):
        for session in list(self._sessions):
            try:
                await session.close()
            except Exception:
                pass
        self._sessions.clear()

        total = self.handshake_ok + self.handshake_rejected + self.upgrade_throttled
        if total:
            print(
                f"  [connect] handshake_ok={self.handshake_ok} "
                f"handshake_rejected={self.handshake_rejected} "
                f"upgrade_throttled={self.upgrade_throttled} "
                f"({(self.handshake_rejected + self.upgrade_throttled) / total * 100:.1f}% admission-limited)"
            )
            for reason, n in sorted(self.reject_reasons.items(), key=lambda kv: -kv[1])[:3]:
                print(f"    reject {reason} ×{n}")
        if self.client:
            await self.client.stop()
