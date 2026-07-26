"""
WebSocket 协议层：真实 protobuf 握手、后台读循环、心跳保活。

帧格式以服务端实现为准（`internal/gateway`）：
- WebSocket **binary** message 内是裸的 `WsFrame` protobuf，**没有**长度前缀
  （4 字节长度前缀只用于 `ReadFrame`/`WriteFrame` 的 io.Reader 路径，不走 WS）
- opcode 取值见 `internal/gateway/frame.go`

协议若变更需重跑 `./gen_proto.sh`，否则本客户端会静默失效（issue 0034）。
"""
import asyncio
import time
from typing import Optional

import websockets

from core.gen import ws_frame_pb2 as pb

PROTOCOL_VERSION = 0x01

OP_HANDSHAKE_REQ = 0x0001
OP_HANDSHAKE_RESP = 0x0002
OP_HEARTBEAT = 0x0003
OP_HEARTBEAT_ACK = 0x0004
OP_ACK = 0x0005
OP_ERROR = 0x0006
OP_DISCONNECT = 0x0007
OP_MESSAGE_DELIVERY = 0x1001
OP_SYNC_EVENT = 0x2001
OP_CONVERSATION_UPDATE = 0x2002

OPCODE_NAMES = {
    OP_HANDSHAKE_RESP: "HANDSHAKE_RESP",
    OP_HEARTBEAT_ACK: "HEARTBEAT_ACK",
    OP_ERROR: "ERROR",
    OP_DISCONNECT: "DISCONNECT",
    OP_MESSAGE_DELIVERY: "MESSAGE_DELIVERY",
    OP_SYNC_EVENT: "SYNC_EVENT",
    OP_CONVERSATION_UPDATE: "CONVERSATION_UPDATE",
}


class HandshakeRejected(Exception):
    """网关明确拒绝了应用层握手（限流、token 失效、协议不符等）。

    与「upgrade 失败」区分开：upgrade 成功但握手被拒说明连接根本不可路由。
    """

    def __init__(self, code: int, message: str, should_reconnect: bool = False):
        super().__init__(f"handshake rejected: code={code} message={message!r}")
        self.code = code
        self.message = message
        self.should_reconnect = should_reconnect


def encode_frame(opcode: int, payload_msg=None, seq_id: int = 0) -> bytes:
    frame = pb.WsFrame(
        version=PROTOCOL_VERSION,
        opcode=opcode,
        seq_id=seq_id,
        timestamp_ms=int(time.time() * 1000),
    )
    if payload_msg is not None:
        frame.payload = payload_msg.SerializeToString()
    return frame.SerializeToString()


def decode_frame(raw: bytes) -> pb.WsFrame:
    frame = pb.WsFrame()
    frame.ParseFromString(raw)
    return frame


class WsSession:
    """一条完成握手的连接。

    读循环独立于发送路径：投递帧要在一个专门的 task 里收，否则端到端延迟统计
    会被自己的发送调用阻塞污染。
    """

    def __init__(self, ws, device_id: str):
        self.ws = ws
        self.device_id = device_id
        self.session_id: Optional[str] = None
        self.heartbeat_interval_s: int = 30
        self.latest_event_seq: int = 0

        # 收到的投递帧：(WsMessageDelivery, 本地单调时钟接收时刻)
        self.deliveries: "asyncio.Queue" = asyncio.Queue()
        self.frame_counts: dict = {}
        self.closed_reason: Optional[str] = None

        self._seq = 0
        self._reader_task: Optional[asyncio.Task] = None
        self._heartbeat_task: Optional[asyncio.Task] = None

    # ── lifecycle ──────────────────────────────────

    def start_background_tasks(self, heartbeat: bool = True):
        self._reader_task = asyncio.create_task(self._read_loop())
        if heartbeat:
            self._heartbeat_task = asyncio.create_task(self._heartbeat_loop())

    async def close(self):
        for task in (self._heartbeat_task, self._reader_task):
            if task is not None:
                task.cancel()
        try:
            await self.ws.close()
        except Exception:
            pass

    # ── frames ─────────────────────────────────────

    def _next_seq(self) -> int:
        self._seq += 1
        return self._seq

    async def send_heartbeat(self):
        await self.ws.send(encode_frame(OP_HEARTBEAT, pb.Heartbeat(), self._next_seq()))

    async def _heartbeat_loop(self):
        # 略快于协商周期，避免边界抖动被判定超时
        interval = max(1.0, self.heartbeat_interval_s * 0.5)
        try:
            while True:
                await asyncio.sleep(interval)
                await self.send_heartbeat()
        except (asyncio.CancelledError, Exception):
            return

    async def _read_loop(self):
        try:
            async for raw in self.ws:
                received_at = time.monotonic()
                frame = decode_frame(raw)
                name = OPCODE_NAMES.get(frame.opcode, f"0x{frame.opcode:04x}")
                self.frame_counts[name] = self.frame_counts.get(name, 0) + 1

                if frame.opcode == OP_MESSAGE_DELIVERY:
                    delivery = pb.WsMessageDelivery()
                    delivery.ParseFromString(frame.payload)
                    await self.deliveries.put((delivery, received_at))
                elif frame.opcode in (OP_ERROR, OP_DISCONNECT):
                    msg = pb.ErrorFrame() if frame.opcode == OP_ERROR else pb.DisconnectFrame()
                    msg.ParseFromString(frame.payload)
                    self.closed_reason = f"{name}: {msg}"
        except asyncio.CancelledError:
            return
        except Exception as exc:
            self.closed_reason = f"read loop ended: {type(exc).__name__}: {exc}"

    async def wait_delivery(self, timeout: float):
        return await asyncio.wait_for(self.deliveries.get(), timeout=timeout)


async def connect_and_handshake(
    ws_url: str,
    token: str,
    device_id: str,
    last_event_seq: int = 0,
    timeout: float = 5.0,
    heartbeat: bool = True,
) -> WsSession:
    """完成 upgrade + HANDSHAKE_REQ/RESP，返回可用会话。

    upgrade 失败会抛 websockets 的异常；握手被拒抛 HandshakeRejected。
    调用方应区分这两类，它们说明的问题完全不同。
    """
    ws = await asyncio.wait_for(websockets.connect(ws_url), timeout=timeout)

    req = pb.HandshakeRequest(
        access_token=token,
        device_id=device_id,
        platform="load-test",
        app_version="0.0.0",
        protocol_ver=PROTOCOL_VERSION,
        last_event_seq=last_event_seq,
    )
    try:
        await ws.send(encode_frame(OP_HANDSHAKE_REQ, req, seq_id=1))
        raw = await asyncio.wait_for(ws.recv(), timeout=timeout)
    except Exception:
        await ws.close()
        raise

    frame = decode_frame(raw)
    if frame.opcode == OP_ERROR:
        err = pb.ErrorFrame()
        err.ParseFromString(frame.payload)
        await ws.close()
        raise HandshakeRejected(err.error_code, err.message, err.should_reconnect)
    if frame.opcode != OP_HANDSHAKE_RESP:
        await ws.close()
        raise HandshakeRejected(0, f"unexpected opcode 0x{frame.opcode:04x}")

    resp = pb.HandshakeResponse()
    resp.ParseFromString(frame.payload)
    if not resp.success:
        await ws.close()
        raise HandshakeRejected(0, resp.error_message or "handshake unsuccessful")

    session = WsSession(ws, device_id)
    session.session_id = resp.session_id
    session.latest_event_seq = resp.latest_event_seq
    if resp.heartbeat_interval_s:
        session.heartbeat_interval_s = resp.heartbeat_interval_s
    session.start_background_tasks(heartbeat=heartbeat)
    return session
