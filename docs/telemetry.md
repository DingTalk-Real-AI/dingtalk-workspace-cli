# Operational Telemetry

dws can emit one **anonymous, dimensions-only** operational metric for **every
command invocation**, to monitor error rate, latency, command distribution and
version/platform health. It is the ops-monitoring counterpart to
[audit](./audit.md), but deliberately **much smaller**:

- It collects **coarse dimensions only** — never object names, free text, peer
  ids, device fingerprints, or natural-language intent. There is no "redaction
  tier" because nothing sensitive is ever collected.
- It is **independent of auditing**: unrelated to the `DWS_AUDIT_*` switches, so
  you can run telemetry without enabling compliance auditing.
- It is **off by default**. With `DWS_TELEMETRY_ENABLED` unset, dws emits
  nothing and the hot path is unaffected.

> This is an open-source CLI: any centralized reporting must be **opt-in and
> disclosed**. Nothing is reported by default.

## Enabling

| Environment variable | Description | Example |
|---|---|---|
| `DWS_TELEMETRY_ENABLED` | Enable telemetry (also needs URL to take effect) | `true` |
| `DWS_TELEMETRY_URL` | Ingest endpoint; one JSON event POSTed per command | `https://telemetry.example.com/dws` |
| `DWS_TELEMETRY_TOKEN` | Bearer token for the endpoint (optional) | `xxxxx` |
| `DWS_TELEMETRY_TIMEOUT_MS` | Per-POST timeout cap, ms (default 1500) | `1500` |

## Reported fields (all of them)

```json
{
  "schema_version": "1",
  "ts": "2026-06-04T11:38:24+08:00",
  "trace_id": "76a04f9eba0ad00c",   // == transport execution_id; join with server-side logs
  "corp_id": "ding...",              // tenant dimension, best-effort (from the login token)
  "cli_version": "1.0.34",           // version health: "did this release break a command"
  "channel": "openclaw",             // which agent/integration is driving dws (DWS_CHANNEL)
  "os": "darwin",                    // coarse platform, not PII
  "module": "doc",
  "command": "doc",
  "subcommand": "create_document",
  "outcome": "ok",                   // ok | error
  "err_class": "",                   // error category when outcome=error
  "exit_code": 0,
  "duration_ms": 73                  // wall-clock latency, for P99
}
```

**Deliberately NOT collected** (verify the privacy boundary by reading this
struct): user identity (user_id/name), object names/ids, free text, device
id/serial number, request/response bodies.

## Ingest contract

Any HTTP service can receive it:

```
POST /
Content-Type: application/json
Authorization: Bearer <token>        # matches DWS_TELEMETRY_TOKEN
X-Dws-Telemetry-Schema: 1
Body: one telemetry event as JSON
2xx means success
```

## Local testing (zero dependencies, no SLS)

Before touching SLS, validate the whole pipeline locally. Use
`fc-sls-ingest/localsink.py` (standard library only, no `pip install`) as the
receiver:

```bash
# 1. start the local sink (with a test token)
cd docs/telemetry/fc-sls-ingest
TOKEN=dev python3 localsink.py          # listens on 127.0.0.1:8799, writes /tmp/dws_telemetry.jsonl

# 2. in another terminal, point dws at it
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL=http://127.0.0.1:8799
export DWS_TELEMETRY_TOKEN=dev

# 3. run a few commands (--mock needs no network or real backend, still emits)
dws doc create --title test --mock
dws drive list --mock
```

The sink prints each event live and appends it to `/tmp/dws_telemetry.jsonl`.
Things to check:

- events carry `command/outcome/duration_ms/cli_version/channel/os`;
- compare a command argument (e.g. `--title test`) against the payload to
  confirm **content never appears** in it;
- a POST without the token is rejected (401).

You can also reproduce locally what the dashboard would compute:

```bash
python3 - <<'PY'
import json, collections
rows=[json.loads(l) for l in open('/tmp/dws_telemetry.jsonl') if l.strip()]
by=collections.defaultdict(lambda:{'n':0,'err':0,'dur':[]})
for r in rows:
    k=f"{r['command']} {r['subcommand']}"; b=by[k]
    b['n']+=1; b['err']+=(r['outcome']!='ok'); b['dur'].append(r.get('duration_ms',0))
for k,v in sorted(by.items(), key=lambda x:-x[1]['n']):
    d=v['dur']; print(f"{k:<26}calls {v['n']:>4} err {v['err']:>3} avg {sum(d)//len(d):>5}ms max {max(d):>5}ms")
PY
```

> Note: telemetry only emits once a command actually reaches the MCP call stage.
> If a command fails at argument parsing (before the call), no telemetry is
> produced — this is expected.

## Open-source code vs internal resources (the public/private boundary)

dws is an open-source repo, but **which SLS the telemetry lands in, and which
internal application it binds to, is the deployer's own concern and never enters
the repo**. This boundary is by design, not by accident:

| | Where | Contains | In the repo? |
|---|---|---|---|
| dws binary + the FC/local reference code in this dir | public repo | only POSTs to `DWS_TELEMETRY_URL`; **no endpoint, no secret, no app name** | yes |
| SLS project / FC instance / real URL+token | the deployer's own infrastructure | real address, auth, log store; inside Alibaba it also binds to an internal application | no — never committed, injected via env |

The code **never hardcodes any vendor reporting endpoint**; the URL is always
read from the environment at runtime. So "the code is open" and "the data lands
in the deployer's own SLS" are naturally decoupled: switching deployers is just
a different set of environment variables, with no repo change and no party's
real config visible.

> Alibaba-internal note: an SLS project must belong to an AONE application
> (resource governance). Bind it to the application that owns the dws backend
> (e.g. the DingTalk MCP gateway app); the binding, real URL and token all stay
> internal and the public repo never knows about them.

## Wiring up Alibaba Cloud SLS (recommended for production)

SLS (Log Service) provides ingestion / storage / search / dashboards / alerting
out of the box, and is the standard choice for ops monitoring:

1. **Create the store**: in the SLS console create a Project + Logstore (e.g.
   `dws-telemetry`) with a retention period; index
   `command` / `subcommand` / `outcome` / `cli_version` / `corp_id` / `channel`
   as text and `duration_ms` as a long field (needed for P99).
2. **Create the ingest endpoint**: a **Function Compute (FC)** HTTP trigger is
   the lowest-ops option — it verifies the bearer token and writes the body as
   one log via `PutLogs` (store the full JSON in an `event` field, and promote
   `command`/`outcome`/`duration_ms`/`cli_version` to indexed columns).
3. **Roll out**: set the FC address as `DWS_TELEMETRY_URL` on each dws install.

### Four ready-to-use alerts (SLS alert rules)

| Alert | SLS query (illustrative) | Trigger |
|---|---|---|
| Error-rate spike | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 5% |
| P99 latency breach | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| One command failing at scale | `* \| select command, count_if(outcome='error') c group by command order by c desc` | c spikes for one command |
| Traffic dropped to zero | `* \| select count(*)` | == 0 within 5 minutes |

Route the alert notifications straight to a DingTalk bot.

## Where data lands / the two streams

- **Off = nothing leaves the machine.** dws ships no built-in vendor endpoint.
- **Enterprise's own monitoring**: point `DWS_TELEMETRY_URL` at the
  enterprise's own SLS ingest.
- **Platform-side monitoring**: pointing the URL at DingTalk's telemetry ingest
  is technically fine, but must be opt-in and disclosed. Because this telemetry
  carries **anonymous dimensions only**, the privacy boundary is clean and it
  suits a platform-wide ops dashboard.
- Full compliance trails are a separate stream — use [audit](./audit.md)'s
  enterprise-owned sink; do not mix it with telemetry.
