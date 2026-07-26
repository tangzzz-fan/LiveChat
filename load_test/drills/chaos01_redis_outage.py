"""
chaos 01 复跑脚本：用真 protobuf 握手回答「Redis 中断时连接还能不能用」。

背景：上一轮只用 HTTP upgrade 观察，得出「upgrade 仍成功」，但握手之后的行为
（路由注册、实时投递）无法覆盖。本脚本补上这段（issue 0034）。

用法（服务已启动、Redis 在线时执行）：
    cd load_test && .venv/bin/python drills/chaos01_redis_outage.py
"""
import asyncio
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import httpx

from core.client import ChatClient
from core.ws_protocol import HandshakeRejected

BASE_URL = "http://localhost:8080"
WS_URL = "ws://localhost:8081/ws"
REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


def chaos(script: str):
    path = os.path.join(REPO_ROOT, "livechat-server", "scripts", "chaos", script)
    subprocess.run(["bash", path], check=True, env={**os.environ, "CHAT_ENV": "dev"})


def step(title: str):
    print(f"\n=== {title} ===")


async def try_send(client, token, conv_id, seq):
    try:
        await client.send_message(token, conv_id, seq)
        return "200"
    except Exception as exc:
        return f"FAILED: {exc}"


async def main():
    client = ChatClient(BASE_URL, WS_URL)
    await client.start()

    step("准备：注册双方并建 1:1 会话（必须在注入前完成，中断期间登录不可用）")
    sender = await client.register_user(0)
    receiver = await client.register_user(1)
    async with httpx.AsyncClient(timeout=15.0) as http:
        resp = await http.post(
            f"{BASE_URL}/v1/conversations/direct",
            headers={"Authorization": f"Bearer {sender['token']}"},
            json={"peer_user_id": receiver["user_id"]},
        )
        resp.raise_for_status()
        conv_id = resp.json()["conversation_id"]
    print(f"conv={conv_id} sender={sender['user_id']} receiver={receiver['user_id']}")

    step("基线：Redis 在线，握手 + 实时投递")
    session = await client.connect_ws(receiver["token"], receiver["device_id"])
    print(f"handshake ok, session_id={session.session_id}")
    print("send:", await try_send(client, sender["token"], conv_id, 1))
    try:
        delivery, _ = await session.wait_delivery(10.0)
        print(f"realtime delivery: RECEIVED seq={delivery.conversation_seq}")
    except asyncio.TimeoutError:
        print("realtime delivery: TIMEOUT (unexpected while healthy)")

    step("注入：停止 Redis")
    chaos("redis-down.sh")
    await asyncio.sleep(3)

    step("中断期间：既有连接是否仍能收到实时投递？")
    print("send:", await try_send(client, sender["token"], conv_id, 2))
    try:
        delivery, _ = await session.wait_delivery(8.0)
        print(f"existing connection: STILL RECEIVING seq={delivery.conversation_seq}")
    except asyncio.TimeoutError:
        print("existing connection: NO DELIVERY within 8s (fanout cannot resolve route)")

    step("中断期间：新连接能否完成应用层握手？")
    try:
        new_session = await client.connect_ws(receiver["token"], "chaos-new-device", heartbeat=False)
        print(f"new connection: HANDSHAKE SUCCEEDED session_id={new_session.session_id}")
        print("  → upgrade 与握手都不依赖 Redis；连接看起来正常，但路由注册已失败")
        await new_session.close()
    except HandshakeRejected as exc:
        print(f"new connection: HANDSHAKE REJECTED -> {exc}")
    except Exception as exc:
        print(f"new connection: FAILED -> {type(exc).__name__}: {exc}")

    step("恢复：启动 Redis")
    chaos("redis-up.sh")
    await asyncio.sleep(5)

    step("恢复后：重连并确认投递恢复")
    await session.close()
    session = await client.connect_ws(receiver["token"], receiver["device_id"])
    print(f"reconnect handshake ok, session_id={session.session_id}")
    print("send:", await try_send(client, sender["token"], conv_id, 3))
    try:
        delivery, _ = await session.wait_delivery(10.0)
        print(f"realtime delivery: RECOVERED seq={delivery.conversation_seq}")
    except asyncio.TimeoutError:
        print("realtime delivery: STILL TIMING OUT (investigate)")

    step("恢复后：中断期间的消息是否可补拉")
    async with httpx.AsyncClient(timeout=15.0) as http:
        resp = await http.get(
            f"{BASE_URL}/v1/conversations/{conv_id}/messages",
            headers={"Authorization": f"Bearer {receiver['token']}"},
            params={"from_seq": 0, "limit": 20},
        )
        msgs = resp.json().get("messages", [])
        print(f"backfill: {len(msgs)} messages, seqs={[m.get('conversation_seq') for m in msgs]}")

    await session.close()
    await client.stop()


if __name__ == "__main__":
    asyncio.run(main())
