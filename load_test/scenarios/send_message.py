"""
文本消息发送压测：用群会话 API 建会话，避免 psql 直写。
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

        headers = {"Authorization": f"Bearer {user_a['token']}"}
        async with httpx.AsyncClient(timeout=15.0) as http:
            resp = await http.post(
                f"{self.base_url}/v1/groups",
                headers=headers,
                json={"name": f"load-send-{int(time.time())}", "description": "send_message load"},
            )
            if resp.status_code not in (200, 201):
                raise RuntimeError(f"create group failed: {resp.status_code} {resp.text}")
            data = resp.json()
            group = data.get("group") or {}
            group_id = group.get("id") or data.get("group_id")
            self.conversation_id = data.get("conversation_id") or f"conv_grp_{group_id}"

            add = await http.post(
                f"{self.base_url}/v1/groups/{group_id}/members",
                headers=headers,
                json={"user_ids": [user_b["user_id"]]},
            )
            if add.status_code not in (200, 201, 204):
                raise RuntimeError(f"add members failed: {add.status_code} {add.text}")

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
