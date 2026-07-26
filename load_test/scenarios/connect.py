"""
登录 + WebSocket 连接建立压测（真 upgrade；握手帧最小可用）。
"""
import asyncio
from core.client import ChatClient


class ConnectScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url
        self.ws_url = ws_url
        self.client = None
        self._sockets = []

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

    async def execute(self, idx: int):
        user = await self.client.register_user(idx)
        ws = await self.client.connect_ws(user["token"], user["device_id"])
        self._sockets.append(ws)
        # Keep connection briefly so upgrade path is measurable; close to avoid FD leak under long runs
        await asyncio.sleep(0.05)
        await ws.close()
        self._sockets.remove(ws)
        return user

    async def teardown(self):
        for ws in list(self._sockets):
            try:
                await ws.close()
            except Exception:
                pass
        self._sockets.clear()
        if self.client:
            await self.client.stop()
