"""
WebSocket 连接建立压测。

两个前提决定了本场景的设计：
1. OTP 频控（同 IP ~20 次/小时）→ setup 预建小用户池并复用，不每次注册。
2. Gateway 接入限流（Spec 05 §6.2：每 IP 5 conn/s、每用户 2 conn/s）→ 超限拒绝是**预期行为**，
   单独计入 throttled，不记为错误；只有真实故障（拒连、超时、协议错）才算错误。

因此本场景的产出是两个数字：健康握手延迟 + 被限流比例（说明接入限流生效）。
超限风暴的对照实验见 reconnect_storm 与 docs/load-practice/03。
"""
import asyncio
from core.client import ChatClient


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
        self._sockets = []
        self.accepted = 0
        self.throttled = 0

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
            ws = await self.client.connect_ws(user["token"], user["device_id"])
        except Exception as exc:
            if _is_throttled(exc):
                self.throttled += 1
                return "throttled"
            raise

        self.accepted += 1
        self._sockets.append(ws)
        # Hold briefly so the upgrade path is measurable, then close to avoid FD growth
        await asyncio.sleep(0.05)
        await ws.close()
        self._sockets.remove(ws)
        return "accepted"

    async def teardown(self):
        for ws in list(self._sockets):
            try:
                await ws.close()
            except Exception:
                pass
        self._sockets.clear()
        total = self.accepted + self.throttled
        if total:
            print(
                f"  [connect] accepted={self.accepted} throttled={self.throttled} "
                f"({self.throttled / total * 100:.1f}% admission-limited, expected)"
            )
        if self.client:
            await self.client.stop()
