#!/usr/bin/env python3
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0 (the "License").
#
# Local telemetry sink for testing dws telemetry WITHOUT SLS or Function Compute.
#
# Zero dependencies (Python standard library only). It mimics the ingest
# contract: accepts `POST /` with a JSON body, optionally checks the bearer
# token, pretty-prints each event and appends the raw line to a JSONL file.
# Use this to validate the whole pipeline (dws -> HTTP) before touching any
# cloud infrastructure.
#
# Usage:
#   python3 localsink.py                 # listen on :8799, no auth
#   PORT=9000 TOKEN=dev python3 localsink.py
#
# Then point dws at it:
#   export DWS_TELEMETRY_ENABLED=true
#   export DWS_TELEMETRY_URL=http://127.0.0.1:8799
#   # export DWS_TELEMETRY_TOKEN=dev   # only if you set TOKEN above

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ.get("PORT", "8799"))
# HOST: 127.0.0.1 (default, local-only/safe). Set HOST=0.0.0.0 to accept POSTs
# from other machines on your LAN — e.g. to use THIS computer as a small central
# collector that teammates' dws report into (token auth strongly recommended then).
HOST = os.environ.get("HOST", "127.0.0.1")
TOKEN = os.environ.get("TOKEN", "")
OUTFILE = os.environ.get("OUTFILE", "/tmp/dws_telemetry.jsonl")
# APPEND=1 keeps history across restarts (central collector); default truncates.
APPEND = os.environ.get("APPEND", "") not in ("", "0", "false", "no")

_count = 0


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):  # health check
        self._send(200, "dws local telemetry sink ok\n")

    def do_POST(self):
        global _count
        if TOKEN and self.headers.get("Authorization", "") != f"Bearer {TOKEN}":
            self._send(401, "unauthorized\n")
            return
        n = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(n)
        try:
            event = json.loads(raw)
        except Exception as e:  # noqa: BLE001
            self._send(400, f"bad json: {e}\n")
            return

        _count += 1
        with open(OUTFILE, "ab") as f:
            f.write(raw + b"\n")
        print(f"\n#{_count}  ({len(raw)} bytes)  ->  {OUTFILE}")
        print(json.dumps(event, indent=2, ensure_ascii=False), flush=True)
        self._send(204, "")

    def _send(self, code, body):
        data = body.encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if data:
            self.wfile.write(data)

    def log_message(self, *args):  # silence default access logging
        pass


if __name__ == "__main__":
    if not APPEND:
        open(OUTFILE, "w").close()  # truncate previous run (test default)
    auth = f"(bearer required: {TOKEN!r})" if TOKEN else "(no auth — set TOKEN!)"
    print(f"dws local telemetry sink listening on http://{HOST}:{PORT}  {auth}")
    print(f"capturing to {OUTFILE}  (append={APPEND})\n")
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()
