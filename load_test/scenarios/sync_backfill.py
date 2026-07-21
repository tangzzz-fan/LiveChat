"""
离线同步回补压测（stub）
"""
from core.client import ChatClient


class SyncBackfillScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url
        self.ws_url = ws_url
        self.client = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

    async def execute(self, idx: int):
        user = await self.client.register_user(idx)
        return user

    async def teardown(self):
        await self.client.stop()
