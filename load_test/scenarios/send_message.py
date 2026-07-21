"""
文本消息发送压测场景
"""
import asyncio
from core.client import ChatClient


class SendMessageScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url
        self.ws_url = ws_url
        self.client = None
        self.users = []
        self.conversation_id = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        # Register two users and create a conversation
        user_a = await self.client.register_user(0)
        user_b = await self.client.register_user(1)
        self.users = [user_a, user_b]

        # Create a direct conversation via DB insert (simplified)
        import httpx
        async with httpx.AsyncClient() as c:
            # Register creates users, we need to create a conversation
            # For load testing, we just send messages between the two
            pass

        # For Phase 1 compat: use a known conversation ID
        self.conversation_id = "conv-load-test"

        # Create conversation via SQL (best effort)
        import subprocess
        try:
            subprocess.run([
                "psql", "-h", "localhost", "-U", "livechat", "-d", "livechat",
                "-c", f"INSERT INTO conversations (id, type) VALUES ('{self.conversation_id}', 'direct') ON CONFLICT DO NOTHING"
            ], capture_output=True)
            subprocess.run([
                "psql", "-h", "localhost", "-U", "livechat", "-d", "livechat",
                "-c", f"INSERT INTO conversation_members (conversation_id, user_id) VALUES ('{self.conversation_id}', {user_a['user_id']}), ('{self.conversation_id}', {user_b['user_id']}) ON CONFLICT DO NOTHING"
            ], capture_output=True)
        except Exception:
            pass

        # Round-robin through virtual users
        all_tokens = [user_a["token"]] * (count // 2) + [user_b["token"]] * (count - count // 2)
        for i, tok in enumerate(all_tokens):
            self.users.append({"idx": i, "token": tok})

    async def execute(self, idx: int):
        user = self.users[idx % len(self.users)]
        import time
        seq = int(time.time() * 1000)
        return await self.client.send_message(user["token"], self.conversation_id, seq)

    async def teardown(self):
        await self.client.stop()
