#!/usr/bin/env bash
# gen_proto.sh — 从服务端 proto 生成压测客户端用的 Python 绑定。
#
# schema 的唯一来源是 livechat-server/proto/。协议一旦变更，重跑本脚本；
# 不要手写 wire format——压测客户端会静默失效而没人发现（见 issue 0034）。
set -euo pipefail

cd "$(dirname "$0")"

PROTO_DIR="../livechat-server/proto"
OUT_DIR="core/gen"

if [ ! -x .venv/bin/python ]; then
  echo "ERROR: .venv not found. Run: python3 -m venv .venv && .venv/bin/pip install -r requirements.txt"
  exit 1
fi

mkdir -p "$OUT_DIR"
touch "$OUT_DIR/__init__.py"

.venv/bin/python -m grpc_tools.protoc \
  --proto_path="$PROTO_DIR" \
  --python_out="$OUT_DIR" \
  "$PROTO_DIR/ws_frame.proto"

echo "generated $OUT_DIR/ws_frame_pb2.py from $PROTO_DIR/ws_frame.proto"
