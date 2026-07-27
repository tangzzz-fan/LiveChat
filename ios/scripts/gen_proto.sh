#!/usr/bin/env bash
# Generate Swift protobuf bindings for the LiveChat WS frame protocol.
# Schema source of truth: livechat-server/proto/ws_frame.proto
# Requires: protoc + protoc-gen-swift
#   proxy_on && brew install protobuf swift-protobuf
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROTO_DIR="$ROOT/livechat-server/proto"
OUT_DIR="$ROOT/ios/Packages/ChatInfrastructure/Sources/ChatInfrastructure/Generated"

mkdir -p "$OUT_DIR"

if ! command -v protoc >/dev/null 2>&1; then
  echo "error: protoc not found. Run: brew install protobuf swift-protobuf" >&2
  exit 1
fi

if ! command -v protoc-gen-swift >/dev/null 2>&1; then
  echo "error: protoc-gen-swift not found. Run: brew install swift-protobuf" >&2
  exit 1
fi

protoc --proto_path="$PROTO_DIR" --swift_out="$OUT_DIR" --swift_opt=Visibility=Public "$PROTO_DIR/ws_frame.proto"

rm -f "$ROOT/ios/Generated/"*.pb.swift 2>/dev/null || true

echo "Generated into $OUT_DIR:"
ls -la "$OUT_DIR"
