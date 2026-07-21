"""
群消息扇出压测：创建群 → 批量加人 → 并发发消息，观察热点保护与扇出放大。
"""
import time
import httpx
from core.client import ChatClient


class GroupFanoutScenario:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url.rstrip("/")
        self.ws_url = ws_url
        self.client = None
        self.owner = None
        self.members = []
        self.conversation_id = None
        self.group_id = None

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        # Owner + up to min(count, 20) members (local DB friendly; still exercises fanout)
        member_n = max(2, min(count, 20))
        self.owner = await self.client.register_user(0)
        self.members = [self.owner]
        for i in range(1, member_n):
            self.members.append(await self.client.register_user(i))

        headers = {"Authorization": f"Bearer {self.owner['token']}"}
        async with httpx.AsyncClient(timeout=15.0) as http:
            resp = await http.post(
                f"{self.base_url}/v1/groups",
                headers=headers,
                json={"name": f"load-fanout-{int(time.time())}", "description": "load test"},
            )
            if resp.status_code not in (200, 201):
                raise RuntimeError(f"create group failed: {resp.status_code} {resp.text}")
            data = resp.json()
            group = data.get("group") or {}
            self.group_id = group.get("id") or data.get("group_id")
            self.conversation_id = data.get("conversation_id") or f"conv_grp_{self.group_id}"

            other_ids = [m["user_id"] for m in self.members[1:]]
            if other_ids:
                add = await http.post(
                    f"{self.base_url}/v1/groups/{self.group_id}/members",
                    headers=headers,
                    json={"user_ids": other_ids},
                )
                if add.status_code not in (200, 201, 204):
                    raise RuntimeError(f"add members failed: {add.status_code} {add.text}")

        print(f"  [group_fanout] group={self.group_id} conv={self.conversation_id} members={len(self.members)}")

    async def execute(self, idx: int):
        sender = self.members[idx % len(self.members)]
        seq = int(time.time() * 1000) + idx
        return await self.client.send_message(sender["token"], self.conversation_id, seq)

    async def teardown(self):
        if self.client:
            await self.client.stop()
