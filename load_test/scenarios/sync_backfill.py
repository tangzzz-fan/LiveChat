"""
离线同步回补：灌消息后以落后 cursor 拉 /v1/sync/events。
"""
import time
import httpx
from core.client import ChatClient


class SyncBackfillScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url.rstrip("/")
        self.ws_url = ws_url
        self.client = None
        self.reader = None
        self.conversation_id = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        writer = await self.client.register_user(0)
        self.reader = await self.client.register_user(1)

        headers = {"Authorization": f"Bearer {writer['token']}"}
        async with httpx.AsyncClient(timeout=15.0) as http:
            resp = await http.post(
                f"{self.base_url}/v1/groups",
                headers=headers,
                json={"name": f"load-sync-{int(time.time())}", "description": "sync backfill"},
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
                json={"user_ids": [self.reader["user_id"]]},
            )
            if add.status_code not in (200, 201, 204):
                raise RuntimeError(f"add members failed: {add.status_code} {add.text}")

            # Seed backlog so cursor=0 has work to do
            seed_n = max(20, min(count * 5, 200))
            for i in range(seed_n):
                await self.client.send_message(writer["token"], self.conversation_id, int(time.time() * 1000) + i)

        print(f"  [sync_backfill] conv={self.conversation_id} seeded>={seed_n}")

    async def execute(self, idx: int):
        # Always start from 0 to stress backfill path; limit keeps pages bounded
        return await self.client.sync_events(self.reader["token"], cursor=0, limit=50)

    async def teardown(self):
        if self.client:
            await self.client.stop()
