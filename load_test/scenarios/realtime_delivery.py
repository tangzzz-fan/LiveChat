"""
实时投递压测：A 发消息 → B 的 WebSocket 收到 MESSAGE_DELIVERY，量端到端延迟。

这是 issue 0034 补上的核心能力。此前所有场景只能证明「写入成功」，
证明不了「对端收到」——两者之间隔着 Outbox 消费、Redis 路由查找、Gateway 推帧。

延迟口径：发送 HTTP 请求发出前打点 → 读循环收到投递帧时打点，取同一进程内的
单调时钟差。因此它包含 send 往返 + outbox 消费 + 扇出 + 推帧，**不是**纯网络延迟。
"""
import asyncio
import time

import httpx

from core.client import ChatClient


class RealtimeDeliveryScenario:
    # 接入限流（每用户 2 conn/s）下，长连接必须复用而不是反复建连
    RECEIVER_TIMEOUT_S = 10.0

    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url.rstrip("/")
        self.ws_url = ws_url
        self.client = None
        self.sender = None
        self.receiver = None
        self.receiver_session = None
        self.conversation_id = None
        self.delivered = 0
        self.timed_out = 0
        self.latencies_ms = []
        self._lock = asyncio.Lock()

    async def setup(self, count: int):
        self.client = ChatClient(self.base_url, self.ws_url)
        await self.client.start()

        self.sender = await self.client.register_user(0)
        self.receiver = await self.client.register_user(1)

        async with httpx.AsyncClient(timeout=15.0) as http:
            resp = await http.post(
                f"{self.base_url}/v1/conversations/direct",
                headers={"Authorization": f"Bearer {self.sender['token']}"},
                json={"peer_user_id": self.receiver["user_id"]},
            )
            if resp.status_code != 200:
                raise RuntimeError(f"create direct conversation failed: {resp.status_code} {resp.text}")
            self.conversation_id = resp.json()["conversation_id"]

        self.receiver_session = await self.client.connect_ws(
            self.receiver["token"], self.receiver["device_id"]
        )
        print(
            f"  [realtime_delivery] conv={self.conversation_id} "
            f"receiver_session={self.receiver_session.session_id}"
        )

    async def execute(self, idx: int):
        seq = int(time.time() * 1000000) + idx
        sent_at = time.monotonic()
        await self.client.send_message(self.sender["token"], self.conversation_id, seq)

        try:
            _, received_at = await self.receiver_session.wait_delivery(self.RECEIVER_TIMEOUT_S)
        except asyncio.TimeoutError:
            async with self._lock:
                self.timed_out += 1
            raise RuntimeError("no MESSAGE_DELIVERY within timeout")

        # 队列是共享的，收到的帧不保证对应本次发送；统计的是投递流的滞后量级，
        # 不是单条消息的精确往返。并发下这个近似是有意的取舍。
        async with self._lock:
            self.delivered += 1
            self.latencies_ms.append((received_at - sent_at) * 1000)
        return True

    async def teardown(self):
        if self.receiver_session:
            reason = self.receiver_session.closed_reason
            counts = self.receiver_session.frame_counts
            await self.receiver_session.close()
            print(f"  [realtime_delivery] frames={counts}")
            if reason:
                print(f"  [realtime_delivery] connection ended: {reason}")

        total = self.delivered + self.timed_out
        if total:
            rate = self.delivered / total * 100
            print(
                f"  [realtime_delivery] delivered={self.delivered} timed_out={self.timed_out} "
                f"({rate:.1f}% delivery rate)"
            )
        if self.latencies_ms:
            ordered = sorted(self.latencies_ms)
            p50 = ordered[len(ordered) // 2]
            p95 = ordered[min(len(ordered) - 1, int(len(ordered) * 0.95))]
            print(f"  [realtime_delivery] end-to-end P50={p50:.1f}ms P95={p95:.1f}ms")

        if self.client:
            await self.client.stop()
