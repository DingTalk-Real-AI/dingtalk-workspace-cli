# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
#
# Reference telemetry ingest for dws (DingTalk Workspace CLI).
#
# Role: the "translator" between dws and SLS. dws POSTs one telemetry Event as
# JSON; SLS cannot accept that raw POST (its write API must be signed), so this
# tiny HTTP service verifies the bearer token, then writes the event into an SLS
# Logstore via PutLogs.
#
# Deploy as a Function Compute (FC) "Web Function":
#   startup command:  gunicorn -b 0.0.0.0:9000 app:app
#   listen port:      9000
# See README.md in this directory for the full walkthrough.

import json
import os
import time

from flask import Flask, request, abort

from aliyun.log import LogClient, LogItem, PutLogsRequest

app = Flask(__name__)

# --- configuration (set these as FC environment variables) -------------------
# SLS target. SLS_ENDPOINT looks like "cn-hangzhou.log.aliyuncs.com".
SLS_ENDPOINT = os.environ["SLS_ENDPOINT"]
SLS_PROJECT = os.environ["SLS_PROJECT"]
SLS_LOGSTORE = os.environ["SLS_LOGSTORE"]

# Shared secret the CLI sends as `Authorization: Bearer <token>`. This must
# match DWS_TELEMETRY_TOKEN on the dws side. Empty disables auth (NOT advised).
INGEST_TOKEN = os.environ.get("INGEST_TOKEN", "")

# Credentials: prefer the STS credentials FC injects when a service role is
# bound (recommended — no long-lived keys in env). Fall back to explicit keys.
AK_ID = os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
AK_SECRET = os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
STS_TOKEN = os.environ.get("ALIBABA_CLOUD_SECURITY_TOKEN", "")

# A new client per process is fine for FC; reuse across warm invocations.
_client = LogClient(SLS_ENDPOINT, AK_ID, AK_SECRET, securityToken=STS_TOKEN or None)

# Fields lifted out of the event into their own SLS columns so they are
# query/aggregation-friendly (the full event is also stored verbatim).
_INDEX_FIELDS = (
    "trace_id",
    "corp_id",
    "cli_version",
    "channel",
    "os",
    "module",
    "command",
    "subcommand",
    "outcome",
    "err_class",
    "exit_code",
    "duration_ms",
)


@app.get("/")
def health():
    # FC health checks and humans hitting the URL land here.
    return "dws telemetry ingest ok\n", 200


@app.post("/")
def ingest():
    # 1) Auth: constant-ish bearer check.
    if INGEST_TOKEN:
        auth = request.headers.get("Authorization", "")
        if auth != f"Bearer {INGEST_TOKEN}":
            abort(401)

    # 2) Parse one telemetry Event.
    try:
        event = request.get_json(force=True)
        if not isinstance(event, dict):
            raise ValueError("body is not a JSON object")
    except Exception as e:  # noqa: BLE001 - reject any malformed body
        abort(400, description=f"bad json: {e}")

    # 3) Build the SLS log item. Keep the full event verbatim in `event`,
    #    and promote the dimensions to their own columns for querying.
    item = LogItem()
    item.set_time(int(time.time()))
    contents = [("event", json.dumps(event, ensure_ascii=False))]
    for k in _INDEX_FIELDS:
        if k in event and event[k] is not None:
            contents.append((k, str(event[k])))
    item.set_contents(contents)

    # 4) Write to SLS. topic/source are coarse routing labels.
    req = PutLogsRequest(
        project=SLS_PROJECT,
        logstore=SLS_LOGSTORE,
        topic=event.get("schema_version", ""),
        source="dws-telemetry",
        logitems=[item],
    )
    try:
        _client.put_logs(req)
    except Exception as e:  # noqa: BLE001 - surface SLS errors as 502
        # Telemetry is best-effort on the client; returning non-2xx just makes
        # the CLI log a forward failure. Never crash the worker.
        abort(502, description=f"sls put_logs failed: {e}")

    return "", 204


if __name__ == "__main__":
    # Local dev: python app.py, then POST to http://127.0.0.1:9000/
    app.run(host="0.0.0.0", port=9000)
