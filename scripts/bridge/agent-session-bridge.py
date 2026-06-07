#!/usr/bin/env python3
"""
Agent Session Bridge — link a dws channel to the CURRENT local agent session
============================================================================
Relays between dws and an interactive desktop agent session (WorkBuddy,
QoderWork, ...). dws Stream POSTs the DingTalk message over HTTP; the bridge
enqueues it and waits for the agent session to write a reply, then returns it to
dws to send back to DingTalk.

Why a bridge: these assistants are live sessions you are chatting with and
expose no OpenAI-compatible endpoint of their own. Running a fresh one-shot CLI
(`qodercli -p` / `claude -p`) would spawn a DISCONNECTED instance, not your
session — so the bot must be relayed into the running session via this queue.

Usage (one instance per channel, distinct port + dir so they don't collide):
  # WorkBuddy
  python3 scripts/bridge/agent-session-bridge.py
  WB_GATEWAY=http://localhost:18790 dws connect start --channel workbuddy ...
  # QoderWork
  BRIDGE_PORT=18791 BRIDGE_DIR=~/.dingtalk-bridge-qoderwork python3 scripts/bridge/agent-session-bridge.py
  QW_GATEWAY=http://localhost:18791 dws connect start --channel qoderwork ...

Messages land as JSON in <BRIDGE_DIR>/queue/; the session writes the reply to
<BRIDGE_DIR>/responses/<msg_id>.json, after which the bridge returns immediately.

Environment variables:
  BRIDGE_PORT          listen port (default 18790)
  BRIDGE_DIR           queue/response/log root (default ~/.dingtalk-bridge)
  BRIDGE_TIMEOUT_SEC   max wait per message in seconds (default 120)
"""

import json
import os
import time
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path

BRIDGE_DIR   = Path(os.getenv("BRIDGE_DIR", str(Path.home() / ".dingtalk-bridge")))
QUEUE_DIR    = BRIDGE_DIR / "queue"
RESP_DIR     = BRIDGE_DIR / "responses"
LOG_FILE     = BRIDGE_DIR / "log.txt"
PORT         = int(os.getenv("BRIDGE_PORT", "18790"))
TIMEOUT      = int(os.getenv("BRIDGE_TIMEOUT_SEC", "120"))

for d in (QUEUE_DIR, RESP_DIR):
    d.mkdir(parents=True, exist_ok=True)


def log(msg: str) -> None:
    ts = datetime.now().strftime("%H:%M:%S")
    line = f"[{ts}] {msg}"
    with open(LOG_FILE, "a", encoding="utf-8") as f:
        f.write(line + "\n")
    print(line, flush=True)


class Bridge(BaseHTTPRequestHandler):

    def log_message(self, fmt: str, *args) -> None:
        pass  # suppress default stderr logging

    def do_GET(self) -> None:
        if self.path == "/health":
            self._json(200, {"ok": True, "status": "live"})
        elif self.path.startswith("/queue"):
            items = sorted(QUEUE_DIR.glob("*.json"))
            self._json(200, {
                "pending": len(items),
                "items": [json.loads(p.read_text()) for p in items],
            })
        else:
            self._json(404, {})

    def do_POST(self) -> None:
        if self.path != "/v1/chat/completions":
            self._json(404, {})
            return

        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length))
        messages = body.get("messages", [])
        user_text = messages[-1]["content"] if messages else ""

        msg_id = f"{int(time.time() * 1000)}"
        (QUEUE_DIR / f"{msg_id}.json").write_text(json.dumps({
            "id": msg_id,
            "text": user_text,
            "timestamp": time.time(),
        }, ensure_ascii=False))

        log(f"📩 RECV → {user_text[:80]}")

        reply = self._wait(msg_id)

        log(f"📤 SEND ← {reply[:80]}")

        self._json(200, {
            "id": f"wb-{msg_id}",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": "workbuddy-assistant",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": reply},
                "finish_reason": "stop",
            }],
        })

    def _wait(self, msg_id: str) -> str:
        resp_file = RESP_DIR / f"{msg_id}.json"
        deadline = time.time() + TIMEOUT
        while time.time() < deadline:
            if resp_file.exists():
                data = json.loads(resp_file.read_text())
                resp_file.unlink()
                return data.get("text", "")
            time.sleep(1)
        return "⏰ 超时未收到回复，请稍后重试"

    def _json(self, code: int, data: dict) -> None:
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(data, ensure_ascii=False).encode())


def main() -> None:
    server = HTTPServer(("127.0.0.1", PORT), Bridge)
    log(f"🚀 Bridge started on http://127.0.0.1:{PORT}")
    log(f"   Queue dir:  {QUEUE_DIR}")
    log(f"   Response dir: {RESP_DIR}")
    server.serve_forever()


if __name__ == "__main__":
    main()
