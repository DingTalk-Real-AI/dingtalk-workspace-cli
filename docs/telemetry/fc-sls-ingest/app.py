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
# Two modes (auto-detected):
#   - SLS mode:     all of SLS_ENDPOINT / SLS_PROJECT / SLS_LOGSTORE are set
#                   (and TELEMETRY_DRYRUN is not truthy) -> writes to SLS.
#   - dry-run mode: otherwise -> just logs the event to stdout and returns 204.
#                   Lets you deploy to Function Compute and verify the pipeline
#                   end-to-end BEFORE wiring up SLS. The aliyun-log SDK is only
#                   imported in SLS mode, so dry-run needs no extra dependency.
#
# Deploy as a Function Compute (FC) "Web Function":
#   startup command:  gunicorn -b 0.0.0.0:9000 app:app
#   listen port:      9000
# See README.md in this directory for the full walkthrough.

import json
import os
import sys
import time

from flask import Flask, request, abort

app = Flask(__name__)

# Shared secret the CLI sends as `Authorization: Bearer <token>`. This must
# match DWS_TELEMETRY_TOKEN on the dws side. Empty disables auth (NOT advised).
INGEST_TOKEN = os.environ.get("INGEST_TOKEN", "")

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


def _truthy(v):
    return str(v).strip().lower() in ("1", "true", "yes", "on")


def _sls_mode():
    """SLS mode requires the three SLS vars and no explicit dry-run override."""
    if _truthy(os.environ.get("TELEMETRY_DRYRUN", "")):
        return False
    return all(os.environ.get(k) for k in ("SLS_ENDPOINT", "SLS_PROJECT", "SLS_LOGSTORE"))


# Lazily-built SLS client (only constructed once, only in SLS mode). Kept module
# global so warm FC invocations reuse it.
_client = None


def _get_client():
    global _client
    if _client is None:
        # Imported here (not at module load) so dry-run works without the SDK.
        from aliyun.log import LogClient

        _client = LogClient(
            os.environ["SLS_ENDPOINT"],
            os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_ID", ""),
            os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_SECRET", ""),
            securityToken=os.environ.get("ALIBABA_CLOUD_SECURITY_TOKEN", "") or None,
        )
    return _client


def _write_sls(event):
    from aliyun.log import LogItem, PutLogsRequest

    item = LogItem()
    item.set_time(int(time.time()))
    contents = [("event", json.dumps(event, ensure_ascii=False))]
    for k in _INDEX_FIELDS:
        if k in event and event[k] is not None:
            contents.append((k, str(event[k])))
    item.set_contents(contents)

    req = PutLogsRequest(
        project=os.environ["SLS_PROJECT"],
        logstore=os.environ["SLS_LOGSTORE"],
        topic=event.get("schema_version", ""),
        source="dws-telemetry",
        logitems=[item],
    )
    _get_client().put_logs(req)


@app.get("/")
def health():
    mode = "sls" if _sls_mode() else "dry-run"
    return f"dws telemetry ingest ok (mode={mode})\n", 200


@app.post("/")
def ingest():
    # 1) Auth: bearer check.
    if INGEST_TOKEN:
        if request.headers.get("Authorization", "") != f"Bearer {INGEST_TOKEN}":
            abort(401)

    # 2) Parse one telemetry Event.
    try:
        event = request.get_json(force=True)
        if not isinstance(event, dict):
            raise ValueError("body is not a JSON object")
    except Exception as e:  # noqa: BLE001 - reject any malformed body
        abort(400, description=f"bad json: {e}")

    # 3) Dispatch by mode.
    if _sls_mode():
        try:
            _write_sls(event)
        except Exception as e:  # noqa: BLE001 - surface SLS errors, never crash
            abort(502, description=f"sls put_logs failed: {e}")
    else:
        # dry-run: emit to stdout (captured by FC function logs) so you can
        # confirm the pipeline before SLS is wired up.
        print("DRYRUN " + json.dumps(event, ensure_ascii=False), file=sys.stdout, flush=True)

    return "", 204


if __name__ == "__main__":
    # Local dev: python app.py, then POST to http://127.0.0.1:9000/
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", "9000")))
