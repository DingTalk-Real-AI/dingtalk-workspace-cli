# PAT Frontend Integration Guide

This guide is for desktop and agent hosts that intercept `dws` PAT authorization failures and render a first-class approval flow.

## Audience map

- Frontend/Desktop team: classify PAT stderr JSON, render the approval UI, and retry the original task.
- CLI/Backend team: keep the PAT payload and callback command surface stable.
- QA team: validate exit code, stderr JSON shape, host retry behavior, and legacy compatibility.
- Integration owner: choose which `CLAW_TYPE` values should enable host-owned PAT in your build.

## Primary host-control selector

Host-owned PAT is selected by `CLAW_TYPE`.

Supported values:

- `host-control`
- `rewind-desktop`
- `dws-wukong`
- `wukong`

`DWS_CHANNEL` is no longer the host-control switch. It stays as the upstream `channelCode` only.

Legacy compatibility path:

```bash
DWS_CHANNEL='Qoderwork;host-control'
```

Treat that suffix form as a migration-only fallback for older agents. New integrations should prefer `CLAW_TYPE`.

## Reference model from existing hosts

Use the existing split of responsibilities as the reference implementation:

- `dws-wukong` keeps the CLI side shallow: classify PAT, normalize raw JSON, exit with code `4`, and pass stderr through unchanged.
- `RewindDesktop` acts as the real host: parse PAT JSON from `stderr`, decide whether to render a card, bind host state to optional identifiers such as `authRequestId`, and trigger follow-up actions from the host side.

That split is the model this open-source contract now follows as well:

- CLI owns normalization and callback command execution.
- Host owns rendering, user interaction, callback invocation cadence, and retry of the original command.

## What the CLI emits

When `dws` hits a PAT authorization failure, it writes raw JSON to `stderr` and exits with code `4`.

Typical envelope:

```json
{
  "success": false,
  "code": "PAT_NO_PERMISSION",
  "data": {
    "requiredScopes": ["..."],
    "grantOptions": ["..."],
    "authRequestId": "...",
    "hostControl": {
      "clawType": "rewind-desktop",
      "mode": "host",
      "pollingOwner": "host",
      "retryOwner": "host",
      "callbackOwner": "host"
    },
    "callbacks": [
      {
        "name": "list_super_admins",
        "invoke": {
          "type": "cli",
          "argv": ["dws", "pat", "callback", "list-super-admins"]
        }
      }
    ],
    "flowId": "..."
  }
}
```

The backend may also surface the selector under `error_code`. Hosts should accept both `code` and `error_code`.

## PAT selectors to handle

Known host-relevant PAT selectors:

- `PAT_NO_PERMISSION`
- `PAT_LOW_RISK_NO_PERMISSION`
- `PAT_MEDIUM_RISK_NO_PERMISSION`
- `PAT_HIGH_RISK_NO_PERMISSION`
- `PAT_SCOPE_AUTH_REQUIRED`
- `AGENT_CODE_NOT_EXISTS`

`PAT_SCOPE_AUTH_REQUIRED` is the scope-auth contract. Do not assume it always contains `data.flowId`.

## Contract fields

- `code` / `error_code`: primary machine-readable selector.
- `success`: `false` for PAT interception.
- `data.requiredScopes`: scopes missing for the original operation.
- `data.grantOptions`: approval choices the host can present.
- `data.authRequestId`: correlation id for host state, logs, and callback requests.
- `data.hostControl`: indicates the host owns UI, polling, and retry.
- `data.callbacks`: callback descriptors for `dws pat callback ...` follow-up actions.
- `data.flowId`: optional approval-flow id. It may be absent, especially for `PAT_SCOPE_AUTH_REQUIRED`.

Preserve unknown `data.*` fields for diagnostics and forward compatibility.

## Exit behavior

- PAT authorization failures exit with code `4`.
- PAT JSON is written to `stderr` without the normal CLI formatter.
- If `flowId` is absent, the host must not assume polling is possible.
- In host-owned PAT mode, the host decides whether to show approval UI, invoke callbacks, and retry the original DWS command.

## Recommended host behavior

1. Detect PAT JSON as soon as `stderr` is available.
2. Treat it as a recoverable authorization event, not a generic command failure.
3. Use `code` first and fall back to `error_code`.
4. Bind UI state to `authRequestId` when present.
5. Render `requiredScopes` as the reason for authorization and `grantOptions` as the action set.
6. Use `data.callbacks` to drive `dws pat callback ...` follow-up actions rather than calling DingTalk APIs directly.
7. Only poll when `flowId` is present.
8. Retry the original DWS command only after `poll-flow` returns `status = "APPROVED"` and `retrySuggested = true`.

## Callback contract

The callback command names remain:

- `dws pat callback list-super-admins`
- `dws pat callback send-apply --admin-staff-id <id>`
- `dws pat callback poll-flow --flow-id <id>`
- `dws auth login --scope <scope>` via the `auth_login` callback for `PAT_SCOPE_AUTH_REQUIRED`

`poll-flow` is one-shot. On approval it may:

- exchange `authCode`
- persist the refreshed token
- return `tokenUpdated = true`
- return `retrySuggested = true`

Hosts should keep using `tokenUpdated` and `retrySuggested` in callback responses.

## Parsing rules

- Do not depend on top-level `message`, `error`, `trace_id`, or other wrapper fields.
- Do not require `flowId` to classify PAT.
- Do not require `flowId` for `PAT_SCOPE_AUTH_REQUIRED`.
- Ignore unknown keys so the contract can evolve safely.

## Migration and compatibility

- New builds should enable host-owned PAT via `CLAW_TYPE`.
- Existing third-party agents that still rely on `DWS_CHANNEL='...;host-control'` should be treated as legacy consumers.
- Keep command names `list-super-admins`, `send-apply`, and `poll-flow` unchanged for compatibility.
- Keep `tokenUpdated` and `retrySuggested` unchanged in callback responses.

## Team checklist

### Frontend/Desktop

- Treat exit code `4` as a recoverable PAT event.
- Parse stderr as JSON before showing any generic failure UI.
- Persist `authRequestId` and optional `flowId` in host state.
- Handle `PAT_SCOPE_AUTH_REQUIRED` without assuming polling is available.

### CLI/Backend

- Keep raw PAT stderr payloads clean.
- Keep `data.hostControl`, `data.callbacks`, and callback command names stable.
- Keep `DWS_CHANNEL` documented as upstream channel only, with the suffix form marked legacy-only.

### QA

- Verify `CLAW_TYPE` values trigger host-owned PAT behavior.
- Verify legacy `DWS_CHANNEL='...;host-control'` still works if compatibility is enabled.
- Verify `PAT_SCOPE_AUTH_REQUIRED` is handled correctly with and without `flowId`.
- Verify `poll-flow` approval responses preserve `tokenUpdated` and `retrySuggested`.
