"""
登录 + WebSocket 连接建立压测
"""
from core.client import ChatClient


class ConnectScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url
        self.ws_url = ws_url
        self.client = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

    async def execute(self, idx: int):
        # Just register a new user (which exercises auth + token flow)
        await self.client.register_user(idx)

    async def teardown(self):
        await self.client.stop()
