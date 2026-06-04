# Ops Telemetry

dws can emit one **anonymous, dimensions-only** ops metric per **command
invocation**, used to monitor error rate, latency, command distribution, and
version/platform health. It is the ops-side counterpart of [audit](./audit.md),
but deliberately **far smaller**:

- Collects **coarse dimensions only** — never object names, free text, peer ids,
  device fingerprints, or natural-language input. There is no "redaction profile"
  because there are no sensitive fields to redact in the first place.
- **Independent of audit**: unrelated to `DWS_AUDIT_*`; you can enable telemetry
  without enabling compliance audit.
- **Default posture depends on the build** (see [Default posture](#default-posture)):
  the open-source build is **off** (pure opt-in); a downstream distribution may
  bake in a default endpoint and ship **on by default**, with a one-time
  disclosure and an opt-out.

> This is an open-source CLI: the **public build never reports a byte and never
> hardcodes an endpoint**. Any on-by-default behavior lives only in a downstream
> build that injects its own endpoint — and even then it is disclosed once and
> can be opted out of.

## Enabling

| Environment variable | Description | Example |
|---|---|---|
| `DWS_TELEMETRY_ENABLED` | Explicitly enable/disable; overrides the build default either way | `true` / `false` |
| `DWS_TELEMETRY_DISABLED` | Hard opt-out; wins over everything (the off switch for on-by-default builds) | `true` |
| `DWS_TELEMETRY_FILE` | **Local file sink** — append each event as one JSON line here instead of POSTing (no server, no network). Takes precedence over URL | `~/.dws/telemetry.jsonl` |
| `DWS_TELEMETRY_URL` | Ingest endpoint; overrides the build-time default; one JSON event POSTed per invocation | `https://telemetry.example.com/dws` |
| `DWS_TELEMETRY_TOKEN` | Bearer auth for the endpoint (optional) | `xxxxx` |
| `DWS_TELEMETRY_TIMEOUT_MS` | Per-report timeout cap, in ms (default 1500) | `1500` |

## Default posture

`Enabled()` resolves like this:

1. `DWS_TELEMETRY_DISABLED=true` → **off** (always wins).
2. No destination (no `DWS_TELEMETRY_URL` and no baked-in default) → **off**.
3. `DWS_TELEMETRY_ENABLED` set → its value wins (`true`/`false`).
4. Otherwise → **on only if the build baked in a default endpoint**; a bare env
   URL in the open-source build stays opt-in (off until `DWS_TELEMETRY_ENABLED=true`).

**Open-source build** → off; an operator opts in with `DWS_TELEMETRY_ENABLED=true`
plus a `DWS_TELEMETRY_URL`.

**Downstream "fleet" build (on by default)** → inject a default endpoint at build
time via `-ldflags`, so every install of that distribution reports to the
operator's own ingest out of the box (users opt out with
`DWS_TELEMETRY_DISABLED=true`):

```bash
go build -ldflags "\
  -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry.defaultURL=https://<your-fc-host>/dws \
  -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry.defaultToken=<token>" ./cmd
```

The public repo never hardcodes a real endpoint — only your build does. This
keeps "code is open source" and "data lands in the operator's own sink"
decoupled.

### One-time disclosure

The first time telemetry is active on a machine, dws prints a one-time notice to
stderr and writes a marker (`~/.dws/.telemetry_notice_shown`) so it never repeats:

```
ℹ️  dws reports anonymous operational telemetry (command, outcome, latency, version
   — no content, no identity) to help monitor stability. Opt out anytime with
   DWS_TELEMETRY_DISABLED=true. Details: docs/telemetry.md
```

## Local monitoring (lightest — no server, no SLS)

The smallest possible setup: point telemetry at a **local file**. No receiver, no
FC, no SLS — each machine appends its own events; you aggregate the file whenever.

```bash
# turn it on (file sink alone enables telemetry)
export DWS_TELEMETRY_FILE=~/.dws/telemetry.jsonl

# ... use dws normally ...

# one-line stability view (per command: calls / errors / avg latency)
python3 - <<'PY'
import json, collections, os
rows=[json.loads(l) for l in open(os.path.expanduser('~/.dws/telemetry.jsonl')) if l.strip()]
by=collections.defaultdict(lambda:{'n':0,'err':0,'dur':[]})
for r in rows:
    k=f"{r.get('command')}.{r.get('subcommand')}"; b=by[k]
    b['n']+=1; b['err']+=(r.get('outcome')!='ok'); b['dur'].append(r.get('duration_ms',0))
print(f"{'command':<28}{'calls':>6}{'err':>5}{'avg_ms':>8}")
for k,v in sorted(by.items(),key=lambda x:-x[1]['n']):
    d=v['dur'] or [0]; print(f"{k:<28}{v['n']:>6}{v['err']:>5}{sum(d)//len(d):>8}")
PY
```

For a small fleet, collect each machine's `telemetry.jsonl` (rsync/scp) and run
the same aggregation over the combined files. Scale to the URL→ingest path only
when you outgrow this.

## Reported fields (complete)

```json
{
  "schema_version": "1",
  "ts": "2026-06-04T11:38:24+08:00",
  "trace_id": "76a04f9eba0ad00c",   // == transport execution_id, joinable with server-side logs
  "corp_id": "ding...",              // tenant dimension, best-effort (from the login token)
  "cli_version": "1.0.34",           // version health: "did this release break a command"
  "channel": "openclaw",             // which agent/integration drove the call (DWS_CHANNEL)
  "os": "darwin",                    // coarse platform, not PII
  "module": "doc",
  "command": "doc",
  "subcommand": "create_document",
  "outcome": "ok",                   // ok | error
  "err_class": "",                   // error category when outcome=error
  "exit_code": 0,
  "duration_ms": 73                  // wall-clock latency of the call, used for P99
}
```

**Deliberately not collected** (verify the privacy boundary by reading the
struct): user identity (user_id / name), object names/ids, free text, device
id/serial, request/response body.

## Receiver contract

Any HTTP service can receive it:

```
POST /
Content-Type: application/json
Authorization: Bearer <token>        # matches DWS_TELEMETRY_TOKEN
X-Dws-Telemetry-Schema: 1
Body: one telemetry event JSON
Return 2xx for success
```

## Local testing (zero dependencies, no SLS)

Before going to SLS, run the whole pipeline locally. Use
`fc-sls-ingest/localsink.py` (pure Python standard library, no `pip install`
needed) as the receiver:

```bash
# 1. Start the local receiver (with a test token)
cd docs/telemetry/fc-sls-ingest
TOKEN=dev python3 localsink.py          # listens on 127.0.0.1:8799, writes /tmp/dws_telemetry.jsonl

# 2. In another terminal, point dws at it
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL=http://127.0.0.1:8799
export DWS_TELEMETRY_TOKEN=dev

# 3. Run a few commands (--mock needs no network or real backend, still emits telemetry)
dws doc create --title test --mock
dws drive list --mock
```

The receiver prints each event in real time and appends to
`/tmp/dws_telemetry.jsonl`. Things to verify:

- Events carry dimensions such as `command/outcome/duration_ms/cli_version/channel/os`;
- Compare command arguments (e.g. `--title test`) against the payload and confirm
  the **content does not appear in the payload**;
- A POST without the token must be rejected (401).

Once written to disk, you can locally simulate the kind of metrics a dashboard
would compute:

```bash
python3 - <<'PY'
import json, collections
rows=[json.loads(l) for l in open('/tmp/dws_telemetry.jsonl') if l.strip()]
by=collections.defaultdict(lambda:{'n':0,'err':0,'dur':[]})
for r in rows:
    k=f"{r['command']} {r['subcommand']}"; b=by[k]
    b['n']+=1; b['err']+=(r['outcome']!='ok'); b['dur'].append(r.get('duration_ms',0))
for k,v in sorted(by.items(), key=lambda x:-x[1]['n']):
    d=v['dur']; print(f"{k:<26}calls{v['n']:>4} err{v['err']:>3} avg{sum(d)//len(d):>5}ms max{max(d):>5}ms")
PY
```

> Note: telemetry is only emitted once a command actually reaches the MCP-call
> stage. If a command fails at argument parsing (before the call), no telemetry is
> produced — this is expected behavior.

## Boundary between open-source code and internal resources (public/private split)

dws is an open-source repository, but **which SLS the telemetry lands in and which
internal app it binds to is the deployer's own concern and never goes into the
repo**. This boundary is by design, not accident:

| | Where | Contains | In repo? |
|---|---|---|---|
| dws binary + the FC/local reference code in this dir | Public repo | Only POSTs to `DWS_TELEMETRY_URL`; **no endpoint, no secret, no app name** | ✅ |
| SLS Project / FC instance / real URL+token | Deployer's internal infra | Real address, auth, logstore; inside Alibaba it also binds to an internal app | ❌ Never in the repo; injected via env vars |

The code **never hardcodes any vendor reporting address**; the URL is always read
from an environment variable at runtime. So "code is public" and "data lands in
the deployer's internal SLS" are naturally decoupled: switching deployers is just
a different set of env vars, the repo needs no change, and no party's real config
is visible.

> Inside Alibaba: the SLS Project must hang under an AONE app (resource-governance
> requirement). Bind it to the app that owns the dws backend (e.g. the DingTalk
> MCP gateway app); that binding, the real URL, and the token all stay internal —
> the public repo is unaware of them.

## Wiring up Alibaba Cloud SLS (recommended for production)

SLS (Log Service) ships with ingest / storage / search / dashboards / alerting —
a standard choice for ops monitoring:

1. **Create the store**: in the SLS console create a Project + Logstore (e.g.
   `dws-telemetry`), set retention days; index the fields `command` /
   `subcommand` / `outcome` / `cli_version` / `corp_id` / `channel`, and set
   `duration_ms` as a long-typed index (needed for P99).
2. **Create the receiver endpoint**: a **Function Compute (FC)** HTTP trigger is
   the lowest-ops option — after validating the Bearer, write the body as a single
   log via `PutLogs` into the Logstore (put the whole JSON in an `event` field and
   also extract `command`/`outcome`/`duration_ms`/`cli_version` as indexed
   columns).
3. **Roll out**: set the FC address as `DWS_TELEMETRY_URL` on each dws endpoint.

### Four ready-to-use alerts (SLS alert rules)

| Alert | SLS query (illustrative) | Trigger |
|---|---|---|
| Error-rate spike | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 5% |
| P99 latency over budget | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| One command failing broadly | `* \| select command, count_if(outcome='error') c group by command order by c desc` | c spikes for a single command |
| Call volume drops to zero | `* \| select count(*)` | == 0 within 5 minutes |

The alert notification channel can be a DingTalk bot directly.

## Where the data lands / two flows

- **Off = never leaves the machine.** dws ships no default vendor reporting address.
- **Enterprise self-hosted monitoring**: point `DWS_TELEMETRY_URL` at the
  enterprise's own SLS ingest.
- **Platform-side unified monitoring**: point the URL at DingTalk's telemetry
  ingest — technically possible, but must be opt-in + disclosed. Because this
  telemetry **contains only anonymous dimensions**, the privacy boundary is clean
  by construction, suitable for a platform ops dashboard.
- Full compliance trails are a separate track — use the enterprise's own sink via
  [audit](./audit.md); don't mix it with telemetry.
