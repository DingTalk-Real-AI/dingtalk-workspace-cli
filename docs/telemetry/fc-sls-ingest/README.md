# dws telemetry ingest (Function Compute → SLS)

This is the **reference receiver** for [operational telemetry](../../telemetry.md):
dws POSTs one telemetry JSON, and SLS cannot accept that raw POST (its write API
must be signed), so this minimal HTTP service verifies the token and writes to
SLS via `PutLogs`. Deploy it as a Function Compute (FC) **Web Function** — no
need to worry about FC handler signatures.

```
dws  ──POST one JSON──▶  this service (FC Web Function)  ──PutLogs──▶  SLS Logstore  ──▶ dashboard/alerts
```

## Files

- `app.py` — Flask service: `POST /` verifies the bearer → parses JSON → writes to SLS; `GET /` is a health check
- `localsink.py` — zero-dependency local sink for testing without SLS/FC
- `requirements.txt` — dependencies (flask / gunicorn / aliyun-log-python-sdk)

## 1. Create the store in SLS (a few clicks in the console)

1. Create a **Project** (e.g. `dws-ops`) and a **Logstore** (e.g. `dws-telemetry`) with a retention period.
2. Add indexes: `command` / `subcommand` / `outcome` / `cli_version` / `corp_id` / `channel`
   as **text**; `duration_ms` / `exit_code` as **long** (for P99 and aggregation).

## Two run modes (auto-detected)

`app.py` switches automatically by environment variable — **no code change**:

| Mode | Trigger | Behavior |
|---|---|---|
| **dry-run** | any SLS var missing, or `TELEMETRY_DRYRUN=true` | logs each event to stdout (captured by FC function logs) and returns 204. **No aliyun-log SDK needed**; good for validating the pipeline first |
| **SLS** | `SLS_ENDPOINT` + `SLS_PROJECT` + `SLS_LOGSTORE` all set | verifies, then `PutLogs` into the Logstore |

`GET /` reports the active mode (`mode=dry-run` / `mode=sls`) — obvious at a glance after deploy.

## 2. Deploy as an FC Web Function

1. FC console → create function → **Web Function** → Python runtime.
2. Upload this directory (including `requirements.txt`; FC installs deps automatically).
3. **Startup command**: `gunicorn -b 0.0.0.0:9000 app:app`, **listen port** `9000`.
4. **Dry-run first (strongly recommended)**: on the first deploy set only
   `INGEST_TOKEN` and **no SLS vars** (or add `TELEMETRY_DRYRUN=true`). After
   deploy, `GET /` should show `mode=dry-run`; point dws at it, run a few
   commands, and look for `DRYRUN {...}` lines in the **FC function logs** — that
   proves the "client → FC" leg works. This step needs **no SLS, no store, no SDK**.
5. **Then wire SLS**: bind a **service role** to the function granting
   `AliyunLogFullAccess` (or a narrower PutLogs permission) — that way no
   AccessKey goes into env, FC injects STS temporary credentials, and `app.py`
   reads them preferentially. Then add the SLS environment variables and `GET /`
   flips to `mode=sls`:

   | Variable | Value | Notes |
   |---|---|---|
   | `SLS_ENDPOINT` | `cn-hangzhou.log.aliyuncs.com` | change for your region |
   | `SLS_PROJECT` | `dws-ops` | the Project from step 1 |
   | `SLS_LOGSTORE` | `dws-telemetry` | the Logstore from step 1 |
   | `INGEST_TOKEN` | a random string you generate | must match `DWS_TELEMETRY_TOKEN` on the dws side |

6. After deploy, take the function's HTTP trigger URL (e.g. `https://xxx.cn-hangzhou.fcapp.run`).

## 3. Wire up dws

In the environment that runs dws (or injected by the orchestrating agent):

```bash
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL="https://xxx.cn-hangzhou.fcapp.run"   # the function URL from above
export DWS_TELEMETRY_TOKEN="<same random string as INGEST_TOKEN>"
```

Run a few commands and you'll see records appear in the SLS Logstore query page.

## 4. Local validation first (optional, no FC / no SLS)

The simplest local check uses `localsink.py` (standard library, zero deps); see
[the "Local testing" section in telemetry.md](../../telemetry.md#local-testing-zero-dependencies-no-sls).

You can also run this service's **dry-run mode** locally (no SLS, no aliyun-log):

```bash
cd docs/telemetry/fc-sls-ingest
pip install flask                       # dry-run needs only flask; aliyun-log is for SLS mode
INGEST_TOKEN=dev python3 app.py         # no SLS_* -> auto dry-run, listens on :9000
# in another terminal:
curl -s localhost:9000/                 # should report mode=dry-run
curl -XPOST localhost:9000/ -H 'Authorization: Bearer dev' \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":"1","command":"doc","outcome":"ok","duration_ms":42}'
# returns 204; the event prints as DRYRUN {...} in the app.py terminal.
```

To validate against real SLS locally, add `SLS_ENDPOINT/SLS_PROJECT/SLS_LOGSTORE`
and an AccessKey pair (`pip install -r requirements.txt` for aliyun-log); `GET /`
becomes `mode=sls`.

## 5. Configure alerts (SLS console → Alerts)

| Alert | Query (illustrative) | Trigger |
|---|---|---|
| Error-rate spike | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 0.05 |
| P99 latency breach | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| One command failing at scale | `* \| select command, count_if(outcome='error') c group by command order by c desc` | c spikes for one command |
| Traffic dropped to zero | `* \| select count(*) as n` | n == 0 (5-minute window) |

Route notifications straight to a DingTalk bot webhook.

## Security notes

- Use a strong random `INGEST_TOKEN`, keep it in sync with the dws side, and never leave it empty.
- Prefer an FC service role (STS); do not put long-lived AccessKeys in env vars.
- This service only receives **anonymous dimensions** — no user content/identity; the privacy boundary is enforced by the dws client.
