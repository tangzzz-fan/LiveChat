"""
文本消息发送压测：用 1:1 建会话 API 拿会话，避免 psql 直写与建群 workaround。
"""
import time
import httpx
from core.client import ChatClient


class SendMessageScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url.rstrip("/")
        self.ws_url = ws_url
        self.client = None
        self.senders = []
        self.conversation_id = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        user_a = await self.client.register_user(0)
        user_b = await self.client.register_user(1)

        async with httpx.AsyncClient(timeout=15.0) as http:
            resp = await http.post(
                f"{self.base_url}/v1/conversations/direct",
                headers={"Authorization": f"Bearer {user_a['token']}"},
                json={"peer_user_id": user_b["user_id"]},
            )
            if resp.status_code != 200:
                raise RuntimeError(f"create direct conversation failed: {resp.status_code} {resp.text}")
            self.conversation_id = resp.json()["conversation_id"]

        # Round-robin virtual users over the two tokens
        half = max(1, count // 2)
        self.senders = [{"token": user_a["token"]}] * half + [{"token": user_b["token"]}] * (count - half)
        print(f"  [send_message] conv={self.conversation_id} virtual_users={len(self.senders)}")

    async def execute(self, idx: int):
        user = self.senders[idx % len(self.senders)]
        seq = int(time.time() * 1000) + idx
        return await self.client.send_message(user["token"], self.conversation_id, seq)

    async def teardown(self):
        if self.client:
            await self.client.stop()
