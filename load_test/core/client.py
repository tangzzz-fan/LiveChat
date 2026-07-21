"""
HTTP + WebSocket 客户端封装
"""
import asyncio
import json
import httpx
import websockets


class ChatClient:
    def __init__(self, base_url: str, ws_url: str):
        self.base_url = base_url.rstrip("/")
        self.ws_url = ws_url.rstrip("/")
        self.http = None
        self._phone_counter = 0

    async def start(self):
        self.http = httpx.AsyncClient(timeout=httpx.Timeout(10.0))

    async def stop(self):
        await self.http.aclose()

    def unique_phone(self, idx: int) -> str:
        import time
        return f"+1555{int(time.time() * 1000) % 1_000_000_000 + idx:09d}"

    async def register_user(self, idx: int):
        """Register a new user via the new two-step auth flow."""
        phone = self.unique_phone(idx)
        device_id = f"load-test-dev-{idx}"

        # Request code
        resp = await self.http.post(
            f"{self.base_url}/v1/auth/request_code",
            json={"phone_e164": phone},
        )
        if resp.status_code != 200:
            raise Exception(f"request_code failed: {resp.status_code}")

        # Verify code
        resp = await self.http.post(
            f"{self.base_url}/v1/auth/verify_code",
            json={
                "phone_e164": phone,
                "verification_code": "123456",
                "device_id": device_id,
                "platform": "ios",
            },
        )
        if resp.status_code != 200:
            raise Exception(f"verify_code failed: {resp.status_code}")

        data = resp.json()
        return {
            "user_id": data["user_id"],
            "device_id": device_id,
            "token": data["access_token"],
            "phone": phone,
        }

    async def send_message(self, token: str, conversation_id: str, seq: int):
        resp = await self.http.post(
            f"{self.base_url}/v1/messages/send",
            headers={"Authorization": f"Bearer {token}"},
            json={
                "client_message_id": f"load-msg-{seq}",
                "conversation_id": conversation_id,
                "message_type": "text",
                "content": json.dumps({"text": f"load test message {seq}"}),
            },
        )
        if resp.status_code != 200:
            raise Exception(f"send failed: {resp.status_code}")
        return resp.json()

    async def connect_ws(self, token: str):
        ws = await websockets.connect(f"{self.ws_url}")
        # Send handshake (simplified — P0 load test skips protobuf handshake)
        # In a real implementation, we'd send a proper protobuf HandshakeRequest
        return ws


async def _test():
    c = ChatClient("http://localhost:8080", "ws://localhost:8081/ws")
    await c.start()
    user = await c.register_user(0)
    print(f"Registered: user_id={user['user_id']}")
    await c.stop()

if __name__ == "__main__":
    asyncio.run(_test())
