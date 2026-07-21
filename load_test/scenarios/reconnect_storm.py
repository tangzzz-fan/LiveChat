"""
重连风暴压测：先建立 N 条 WS，同时断开，再带 jitter 重连，统计成功率与耗时。
"""
import asyncio
import random
import time
from core.client import ChatClient


class ReconnectStormScenario:
    def __init__(self, base_url: str, ws_url: str, jitter_ms: int = 500, no_jitter: bool = False):
        self.base_url = base_url
        self.ws_url = ws_url
        self.jitter_ms = jitter_ms
        self.no_jitter = no_jitter
        self.client = None
        self.users = []
        self.connections = []
        self._storm_done = False
        self.storm_stats = {"success": 0, "fail": 0, "elapsed_s": 0.0}

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        self.users = []
        for i in range(count):
            self.users.append(await self.client.register_user(i))

        self.connections = []
        for u in self.users:
            try:
                ws = await self.client.connect_ws(u["token"])
                self.connections.append(ws)
            except Exception:
                self.connections.append(None)

        alive = sum(1 for c in self.connections if c is not None)
        print(f"  [reconnect_storm] established {alive}/{count} connections")

        # Drop all
        await asyncio.gather(
            *[c.close() for c in self.connections if c is not None],
            return_exceptions=True,
        )
        self.connections = []

        # Storm reconnect once during setup (measured), then execute() is a light no-op loop
        start = time.time()
        results = await asyncio.gather(
            *[self._reconnect_one(u) for u in self.users],
            return_exceptions=True,
        )
        self.storm_stats["elapsed_s"] = time.time() - start
        self.storm_stats["success"] = sum(1 for r in results if r is True)
        self.storm_stats["fail"] = len(results) - self.storm_stats["success"]
        self._storm_done = True
        print(
            f"  [reconnect_storm] reconnect {self.storm_stats['success']}/{len(results)} "
            f"in {self.storm_stats['elapsed_s']:.2f}s "
            f"(jitter_ms={0 if self.no_jitter else self.jitter_ms})"
        )

    async def _reconnect_one(self, user: dict) -> bool:
        if not self.no_jitter and self.jitter_ms > 0:
            await asyncio.sleep(random.uniform(0, self.jitter_ms / 1000.0))
        try:
            ws = await self.client.connect_ws(user["token"])
            self.connections.append(ws)
            return True
        except Exception:
            return False

    async def execute(self, idx: int):
        # Storm already measured in setup; keep workers busy with cheap success markers
        if not self._storm_done:
            await asyncio.sleep(0.01)
        return self.storm_stats

    async def teardown(self):
        await asyncio.gather(
            *[c.close() for c in self.connections if c is not None],
            return_exceptions=True,
        )
        if self.client:
            await self.client.stop()
