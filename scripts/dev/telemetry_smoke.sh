#!/usr/bin/env bash
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0 (the "License").
#
# One-shot local smoke test for dws telemetry — NO SLS, NO cloud, NO login.
# Builds dws, starts the zero-dependency local sink, fires a few --mock
# commands, then asserts the pipeline end-to-end:
#   - events are received with the expected dimensions
#   - command argument content never leaks into the payload (privacy boundary)
#   - the bearer token is enforced (401 without it)
# Exits non-zero on any failure, so it is safe to wire into CI / pre-push.
#
# Usage:  bash scripts/dev/telemetry_smoke.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PORT="${PORT:-8799}"
TOKEN="dev"
SENTINEL="PRIVATE_SENTINEL_$$"   # unique per run; must NOT appear in any event
BIN="$(mktemp -t dws-smoke.XXXXXX)"
OUT="$(mktemp -t dws-tel.XXXXXX.jsonl)"
SINK_PID=""

cleanup() {
  if [ -n "$SINK_PID" ]; then
    kill "$SINK_PID" 2>/dev/null || true
    wait "$SINK_PID" 2>/dev/null || true   # absorb the job-control "Terminated" notice
  fi
  rm -f "$BIN" "$OUT"
}
trap cleanup EXIT

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() { printf '\033[31mFAIL: %s\033[0m\n' "$*" >&2; exit 1; }

say "[1/5] build dws"
( cd "$ROOT" && go build -o "$BIN" ./cmd )
echo "  -> $BIN"

say "[2/5] start local sink on :$PORT"
TOKEN="$TOKEN" PORT="$PORT" OUTFILE="$OUT" \
  python3 "$ROOT/docs/telemetry/fc-sls-ingest/localsink.py" >/dev/null 2>&1 &
SINK_PID=$!
sleep 1.2
curl -fsS "http://127.0.0.1:$PORT/" >/dev/null || fail "sink not responding on :$PORT"
echo "  -> sink up (pid $SINK_PID)"

say "[3/5] auth enforced (POST without token must be 401)"
code="$(curl -s -o /dev/null -w '%{http_code}' -XPOST "http://127.0.0.1:$PORT/" -d '{}')"
[ "$code" = "401" ] || fail "expected 401 without token, got $code"
echo "  -> 401 OK"

say "[4/5] run --mock commands (telemetry on)"
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL="http://127.0.0.1:$PORT"
export DWS_TELEMETRY_TOKEN="$TOKEN"
export DWS_CHANNEL="smoke-test"
"$BIN" doc create --title "$SENTINEL" --mock >/dev/null 2>&1 || true
"$BIN" doc create --title other --mock        >/dev/null 2>&1 || true
"$BIN" drive list --mock                       >/dev/null 2>&1 || true
sleep 1

say "[5/5] assert captured events"
python3 - "$OUT" "$SENTINEL" <<'PY'
import json, sys, collections
path, sentinel = sys.argv[1], sys.argv[2]
rows = [json.loads(l) for l in open(path) if l.strip()]
if len(rows) < 3:
    print(f"FAIL: expected >=3 events, got {len(rows)}", file=sys.stderr); sys.exit(1)

required = ("schema_version","trace_id","cli_version","os","command","subcommand","outcome","duration_ms")
for r in rows:
    miss = [k for k in required if k not in r]
    if miss:
        print(f"FAIL: event missing fields {miss}: {r}", file=sys.stderr); sys.exit(1)
    if r["channel"] != "smoke-test":
        print(f"FAIL: channel not propagated: {r.get('channel')!r}", file=sys.stderr); sys.exit(1)

# privacy boundary: the sentinel title must never appear anywhere in the payload
raw = open(path, encoding="utf-8").read()
if sentinel in raw:
    print("FAIL: command content LEAKED into telemetry payload", file=sys.stderr); sys.exit(1)

by = collections.defaultdict(lambda: {"n":0,"err":0,"d":[]})
for r in rows:
    k=f"{r['command']}/{r['subcommand']}"; b=by[k]
    b["n"]+=1; b["err"]+=(r["outcome"]!="ok"); b["d"].append(r["duration_ms"])
print(f"  {len(rows)} events, all dimensions present, no content leak")
for k,v in sorted(by.items(), key=lambda x:-x[1]['n']):
    d=v["d"]; print(f"  {k:<26} calls {v['n']} err {v['err']} avg {sum(d)//len(d)}ms max {max(d)}ms")
PY

say "PASS — telemetry pipeline healthy"
