# Operation Audit (local diagnostic trail)

dws can record a structured line for **every command invocation** as a local
**diagnostic / troubleshooting trail** — so an operator can reconstruct what a
machine did through dws, which step failed, and where data went.

> **Scope and limits — read this first.**
> This is a **best-effort, local** trail. It runs on the user's own machine and
> is opt-in, so the user can disable or bypass it; it is **not** a tamper-proof,
> mandatory compliance record. Authoritative, non-bypassable audit belongs on
> the **MCP gateway** (server side), through which every dws call must pass —
> that is the system of record. The client-side trail here is a *complement*: it
> captures local detail the gateway cannot see (e.g. local export paths and the
> agent-injected natural-language intent), not a replacement.

The design separates "producing an event" from "delivering an event":

- **Channel A — local file**: the primary use; the trail an operator owns and
  can `grep`/`jq` at any time.
- **Channel B — forward to a collector**: optional; the endpoint is configured
  by the **deployer** and is **never hardcoded to a vendor**. Useful for pulling
  several machines' trails into one place for investigation. Content can be
  downgraded by redaction tier before forwarding.

> Off by default. With `DWS_AUDIT_ENABLED` unset, dws produces nothing and the
> hot path is unaffected.

## Enabling

| Environment variable | Description | Example |
|---|---|---|
| `DWS_AUDIT_ENABLED` | Enable the local audit file | `true` |
| `DWS_AUDIT_FORWARD_URL` | Forward target (enterprise's own sink, not a vendor default) | `https://audit.internal.example.com/dws` |
| `DWS_AUDIT_FORWARD_TOKEN` | Bearer token for the enterprise sink | `xxxxx` |
| `DWS_AUDIT_FORWARD_REDACT` | Forward redaction tier: `none` / `hashed` / `minimal` | `none` |
| `DWS_AUDIT_REDACT_SALT` | Salt for the `hashed` tier | `tenant-salt` |
| `DWS_AUDIT_DEVICE_FINGERPRINT` | Collect `device_id` / `sn_no` (PIPL personal information; off by default) | `true` |
| `DWS_AUDIT_NL_INTENT` | Natural-language input injected by the orchestrating agent | `export last week's minutes` |
| `DWS_AUDIT_MAX_AGE_DAYS` | Days of dated audit files to keep (0 = keep all) | `30` |

## Where the file lives

The trail is written to `<config-dir>/logs/`, one file **per calendar day** named
`audit-YYYY-MM-DD.jsonl`. `<config-dir>` defaults to `~/.dws`:

| OS | Default path (today's file) |
|---|---|
| macOS | `/Users/<you>/.dws/logs/audit-2026-06-04.jsonl` |
| Linux | `/home/<you>/.dws/logs/audit-2026-06-04.jsonl` |
| Windows | `C:\Users\<you>\.dws\logs\audit-2026-06-04.jsonl` |

Override the base directory with `DWS_CONFIG_DIR` (files then live under
`$DWS_CONFIG_DIR/logs/`). A packaged edition may relocate `<config-dir>`; if home
cannot be resolved, dws falls back to a `.dws` directory next to the executable.

### Rotation & retention

The file **rotates by date** so it never grows unbounded: each day's events go to
that day's `audit-YYYY-MM-DD.jsonl`, and files older than the retention window are
pruned automatically. The window is `DWS_AUDIT_MAX_AGE_DAYS` (default **30**; set
`0` to keep everything). Long-running modes roll at midnight; the short-lived CLI
simply writes today's file each invocation.

### Format: JSONL (one JSON object per line)

The file is **JSONL** — one event per line. This is the mainstream format for
structured, append-only logs: it never rewrites existing lines, stays
human-inspectable, and is ingested natively by every log pipeline (`jq`,
fluentd/Vector, Loki, Splunk, Alibaba Cloud SLS…). Read it directly (glob across
days):

```bash
tail -n 1 ~/.dws/logs/audit-*.jsonl | jq .                              # latest events
jq 'select(.flow.direction=="local-export")' ~/.dws/logs/audit-*.jsonl  # everything exported to local disk
jq 'select(.outcome=="error") | {ts,command,subcommand,err_class}' ~/.dws/logs/audit-*.jsonl
```

## Fields

Fields are split by **trustworthiness**; only trustworthy fields are recorded:

**① Trustworthy fields (recorded)** — token-verified / dws-managed / dws-measured,
not forgeable by the caller per call:

| Field | Meaning | Source | Why trustworthy |
|---|---|---|---|
| `ts` / `trace_id` | time / unique trace | CLI (`trace_id` == transport execution_id) | dws-measured |
| `actor` | user id / name | login token | gateway-verified; `user_id` present only when the login flow captured it |
| `org` | org corp_id / name | login token | gateway-verified, unforgeable |
| `client` | `agent_id` (install id) / `source` / `cli_version` | identity.json + compiled-in version | dws-managed / compiled-in, not caller-asserted |
| `client.channel` | channel / which agent is calling (OpenClaw / Qoder…) | `DWS_CHANNEL` | **semi-trusted**: the gateway validates membership against the `allowedChannels` allowlist (a bogus value is rejected), but there is no cryptographic binding yet, so one registered channel could still impersonate another. Usable for grouping by channel; upgrade to fully trusted once the gateway signs it |
| `device` | os / hostname / device_id / sn_no | local machine; `device_id`/`sn_no` require opt-in | reads real hardware |
| `intent` | natural-language input + `provenance` | injected at the agent layer only | **flagged `provenance=agent`, explicitly unverifiable** |
| `module` / `command` / `subcommand` | operated module / skill command / subcommand | the command the CLI actually parsed and ran | dws-measured |
| `subcommand_desc` | subcommand description | command catalog | online catalog |
| `target` | operated object id / name / summary / sensitivity | call params + catalog (`sensitive` → `confidential`) | dws-measured |
| `flow` | data direction + api + local path / endpoint / peer ids | inferred from call params | dws-measured |
| `outcome` / `err_class` / `exit_code` | success/failure and error class | CLI | dws-measured |

**② Fields deliberately NOT recorded yet (fully forgeable, pending gateway
signing)** — see the TODO below: `host_agent` (which agent it is installed in,
`DINGTALK_AGENT`) and `agent_code` (`DINGTALK_DWS_AGENTCODE`). These are
plain caller-supplied environment-variable labels — an `export` is enough to
spoof them and the gateway does not validate them, so they are **fully
untrusted and therefore not recorded**.

> Difference vs `channel`: the gateway validates `channel` membership against
> the `allowedChannels` allowlist (semi-trusted, recorded); `host_agent` /
> `agent_code` have no validation at all (fully forgeable, not recorded).

### `flow.direction` values

- `local-export`: params carry a local path (e.g. `--output`); data lands on the local disk.
- `read`: read-only command (list/get/query/search…), no data movement.
- `intra-tenant`: data moves between objects inside the tenant; `peer_ids` collects the person/group/doc ids involved.
- `external-api`: flows to an endpoint outside the tenant (reserved).

## Redaction tiers (applied to forwarding only; the local file is always full)

| Tier | Behavior | When to use |
|---|---|---|
| `none` | forward verbatim | sink is inside the enterprise's own trust boundary (its internal audit store) |
| `hashed` | natural language, object names, serial numbers, peer ids replaced by salted hashes — correlatable but not reversible | crosses a trust boundary but still needs correlation |
| `minimal` | keep dimensions only (command × version × outcome × direction), drop all content/identity | pure ops monitoring |

## Enterprise integration example

Data goes into the enterprise's own audit store, all fields, including device fingerprint:

```bash
export DWS_AUDIT_ENABLED=true
export DWS_AUDIT_FORWARD_URL="https://audit.internal.example.com/dws"
export DWS_AUDIT_FORWARD_TOKEN="<enterprise-issued-token>"
export DWS_AUDIT_FORWARD_REDACT=none
export DWS_AUDIT_DEVICE_FINGERPRINT=true
# Injected by the orchestrating agent/skill before each call:
# export DWS_AUDIT_NL_INTENT="<the user's natural-language request this time>"
```

Verify:

```bash
dws minutes export --minute-id m-77 --output ~/Desktop/q2.md --format json
tail -n1 ~/.dws/logs/audit-*.jsonl | jq .   # path varies with DWS_CONFIG_DIR / edition
```

## Where the log lives / can it be centrally collected

> Reminder: central collection here is still **best-effort** — the user controls
> the client and can disable or bypass it. For an **authoritative, mandatory**
> record, audit on the **MCP gateway** (every call passes through it); this
> client-side forwarding is for convenience of investigation, not enforcement.

- **Default: on each user's own machine**, `<config-dir>/logs/audit-YYYY-MM-DD.jsonl`;
  with forwarding off, nothing leaves the machine.
- **For central collection**: set `DWS_AUDIT_FORWARD_URL` to a collection
  endpoint, and each user POSTs one record per invocation.
  - **Enterprise compliance**: point the endpoint at the **enterprise's own
    audit store**; DingTalk/the vendor holds no data (recommended, clean for compliance).
  - **Platform-side collection (DingTalk receives it)**: technically possible —
    point the endpoint at DingTalk's audit ingest service; but that means the
    vendor centrally holds user operation data, so it must be **opt-in and
    clearly disclosed**, otherwise it is the "silent reporting" that open-source
    CLIs most want to avoid. The recommendation is to split into two streams:
    **full compliance audit → enterprise's own sink**; **anonymous minimal
    telemetry (`minimal` tier) → DingTalk platform** for ops monitoring, so the
    privacy boundary is clear.
- Either way, the local file is always the source of truth; forwarding is
  best-effort and a loss can be backfilled from the local file.

### Ingest contract

The collection endpoint (`DWS_AUDIT_FORWARD_URL`) only needs to implement:

```
POST /
Content-Type: application/json
Authorization: Bearer <token>     # matches DWS_AUDIT_FORWARD_TOKEN
X-Dws-Audit-Schema: 2
Body: one audit event as JSON
2xx means success
```

Any HTTP service can receive it; no special component required.

### Wiring up Alibaba Cloud SLS (recommended for production)

SLS (Log Service) provides ingestion / storage / search / dashboards / retention
out of the box, and is the standard choice for landing audit data:

1. In the SLS console create a **Project** + **Logstore** with a retention
   period (180/365 days are common for compliance), and index
   `trace_id` / `command` / `subcommand` / `outcome` / `corp_id` / `agent_id`.
2. Stand up an endpoint that receives the POST (a **Function Compute (FC)** HTTP
   trigger is the lowest-ops option, or ECS/K8s): verify the bearer token, then
   write the body as one log via `PutLogs` (store the full JSON in an `event`
   field, and promote `trace_id`/`command`/`outcome`/`corp_id` to indexed columns).
3. Roll out the FC address as `DWS_AUDIT_FORWARD_URL` to each dws install.

Then "who / when / did what / succeeded or not / data direction" can be queried
and dashboarded directly in the SLS console.

## TODO

- **Gateway-signed agent identity (so `channel` becomes fully trusted and
  `host_agent`/`agent_code` can be recorded)**: see "Gateway-side support
  requirements" below.
- **Stabilize `actor.user_id`**: have the login flow persist `user_id` into the
  token so it is always non-empty (currently captured by only some login flows).

## Gateway-side support requirements (for the gateway team)

**Goal**: make "which agent / channel is calling" in the audit unforgeable.

**Status and gap**: dws already records `client.channel` (from `DWS_CHANNEL`).
The gateway does validate membership against the `allowedChannels` allowlist (a
bogus channel is rejected), but the channel code is just a **plaintext string,
not cryptographically bound to the caller's identity**, so **one registered
channel can impersonate another**; `DINGTALK_AGENT` / `DINGTALK_DWS_AGENTCODE`
are plain labels with no validation at all. To make it "unforgeable", the
gateway needs to support three things:

1. **Issue a signed agent credential bound to the token**
   - When dws completes OAuth/PAT authentication, the gateway — based on the
     **verified token + registered channel** — issues a signed credential (a JWT
     or HMAC string) containing: `channel_code`, `agent_code`, issue time,
     expiry, and a token-bound fingerprint (e.g. `hash(corp_id+user_id)`).
   - New auth-response fields: `agentCredential`, `agentCredentialExpiry`.

2. **Verify the signature on every call and return the "gateway-authenticated identity"**
   - dws sends back an `x-dws-agent-credential` header on every subsequent call.
   - After the gateway verifies it (signature + expiry + token-binding
     consistency), it returns `x-dws-verified-channel` / `x-dws-verified-agent`
     in the response.
   - dws audit **records the verified values the gateway returns**, not the
     locally self-asserted env values → impersonation would require forging the
     gateway's signature, which is infeasible.

3. **Channel registry + integrator identity check**
   - Maintain a `channel_code → integrator (OpenClaw / Qoder / …)` registry;
     when issuing the credential, verify the integrator's identity (its AppKey /
     certificate) so a `channel_code` can only be used by its true owner.

**Draft interface contract**

| Location | Added | Description |
|---|---|---|
| auth response | `agentCredential` / `agentCredentialExpiry` | signed credential bound to the token |
| call request header | `x-dws-agent-credential` | dws returns the credential |
| call response header | `x-dws-verified-channel` / `x-dws-verified-agent` | returned after the gateway verifies; dws writes these into the audit |

**dws-side follow-up (once the gateway is ready)**: switch `client.channel` from
the self-asserted env value to "the verified value the gateway returns", and
unlock `host_agent` / `agent_code` into the audit, flagged as fully trusted.

## Privacy and compliance

- `device_id` / `sn_no` are personal information under PIPL, **not collected by
  default**; the enterprise must explicitly enable them and inform users.
- Natural-language input can only be provided by the orchestrating agent and is
  flagged `provenance=agent` in the audit record, indicating it is not
  CLI-measured and cannot be verified.
- If forwarded, this trail can carry sensitive operational detail and should go
  to a collector the **deployer** owns; dws provides no vendor-default endpoint.
  (Authoritative compliance audit is a gateway-side concern — see "Scope and
  limits" at the top.)
- Caller-self-asserted fields such as `host_agent` / `channel` / `agent_code`
  are **not recorded** before gateway signing, to keep forgeable data out of the
  audit.
