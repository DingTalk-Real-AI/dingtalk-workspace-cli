# dws Telemetry Receiver (Function Compute FC → SLS)

This is the **reference receiver** for [Ops Telemetry](../../telemetry.md): dws
POSTs one telemetry JSON over, but SLS cannot accept a raw POST directly (writes
must be signed), so this minimal HTTP service sits in between — it validates the
token and writes to SLS via `PutLogs`. Deploy it as a Function Compute (FC) **web
function**; you don't have to deal with the FC handler signature.

```
dws  ──POST one JSON──▶  this service (FC web fn)  ──PutLogs──▶  SLS Logstore  ──▶ dashboard/alerts
```

## Files

- `app.py` — Flask service: `POST /` validates Bearer → parses JSON → writes SLS; `GET /` health check
- `requirements.txt` — dependencies (flask / gunicorn / aliyun-log-python-sdk)

## 1. Create the store in SLS first (a few clicks in the console)

1. Create a **Project** (e.g. `dws-ops`) and a **Logstore** (e.g.
   `dws-telemetry`), set retention days.
2. Enable indexes: set `command` / `subcommand` / `outcome` / `cli_version` /
   `corp_id` / `channel` as **text**; set `duration_ms` / `exit_code` as **long**
   (needed for P99 and aggregation).

## Two run modes (auto-detected)

`app.py` switches automatically by environment variables — **no code change**:

| Mode | Trigger | Behavior |
|---|---|---|
| **dry-run** | Any SLS variable missing, or `TELEMETRY_DRYRUN=true` | Received events are printed to stdout (FC captures this in function logs) and return 204. **Does not require the aliyun-log SDK** — good for validating the pipeline first |
| **SLS** | `SLS_ENDPOINT`+`SLS_PROJECT`+`SLS_LOGSTORE` all set | After validation, `PutLogs` writes into the Logstore |

The `GET /` health check echoes the current mode (`mode=dry-run` / `mode=sls`),
so it's obvious right after deploy.

## 2. Deploy this service as an FC web function

1. Function Compute console → Create function → **Web function** → Python runtime.
2. Upload this directory's code (incl. `requirements.txt`; FC installs deps
   automatically).
3. **Startup command**: `gunicorn -b 0.0.0.0:9000 app:app`, **listen port** `9000`.
4. **Dry-run validation first (strongly recommended)**: on the first deploy set
   only `INGEST_TOKEN` and **leave the SLS variables unset** (or add
   `TELEMETRY_DRYRUN=true`). After deploy, `GET /` should show `mode=dry-run`;
   point dws at it, run a few commands, and you'll see `DRYRUN {...}` lines in FC's
   **function logs** — proving the "client → FC" leg works. This step **needs no
   SLS, no store, no SDK**.
5. **Then wire up SLS**: **bind a service role** to the function and grant
   `AliyunLogFullAccess` (or a narrower PutLogs permission) — this way you don't
   put an AccessKey in env vars; FC injects STS temporary credentials and `app.py`
   reads them first. Then add the SLS env vars; once `GET /` becomes `mode=sls`
   it's live:

   | Variable | Value | Note |
   |---|---|---|
   | `SLS_ENDPOINT` | `cn-hangzhou.log.aliyuncs.com` | change to your region |
   | `SLS_PROJECT` | `dws-ops` | the Project from step 1 |
   | `SLS_LOGSTORE` | `dws-telemetry` | the Logstore from step 1 |
   | `INGEST_TOKEN` | a random string you generate | must match dws-side `DWS_TELEMETRY_TOKEN` |

6. After deploy, grab the function's HTTP trigger address (like
   `https://xxx.cn-hangzhou.fcapp.run`).

## 3. Wire dws up

In the environment where dws runs (or injected by the host agent):

```bash
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL="https://xxx.cn-hangzhou.fcapp.run"   # the function address from above
export DWS_TELEMETRY_TOKEN="<same random string as INGEST_TOKEN>"
```

Run a few commands and you'll see records appear in the SLS Logstore query page.

## 4. Validate locally first (optional, no FC / SLS needed)

The simplest local validation uses `localsink.py` (pure standard library, zero
deps), see [the "Local testing" section in telemetry.md](../../telemetry.md#local-testing-zero-dependencies-no-sls).

You can also run this service's **dry-run mode** locally (no SLS, no aliyun-log):

```bash
cd docs/telemetry/fc-sls-ingest
pip install flask                       # dry-run only needs flask; aliyun-log is only for SLS mode
INGEST_TOKEN=dev python3 app.py         # no SLS_* -> auto dry-run, listens on :9000
# in another terminal:
curl -s localhost:9000/                 # should echo mode=dry-run
curl -XPOST localhost:9000/ -H 'Authorization: Bearer dev' \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":"1","command":"doc","outcome":"ok","duration_ms":42}'
# returns 204; the event prints as DRYRUN {...} in the app.py terminal.
```

To validate against real SLS locally, add `SLS_ENDPOINT/SLS_PROJECT/SLS_LOGSTORE`
and an AccessKey (`pip install -r requirements.txt` to install aliyun-log), and
`GET /` will become `mode=sls`.

## 5. Configure alerts (SLS console → Alerts)

| Alert | Query (illustrative) | Trigger |
|---|---|---|
| Error-rate spike | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 0.05 |
| P99 latency over budget | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| One command failing broadly | `* \| select command, count_if(outcome='error') c group by command order by c desc` | c spikes for a single command |
| Call volume drops to zero | `* \| select count(*) as n` | n == 0 (5-minute window) |

The notification channel can be a DingTalk bot webhook directly.

## Security notes

- Use a strong random string for `INGEST_TOKEN`, keep it in sync with the dws
  side, and never leave it empty.
- Prefer the FC service role (STS); do not put a long-lived AccessKey in env vars.
- This service only accepts **anonymous dimension** data — no user content or
  identity; the privacy boundary is guaranteed by the dws client.
