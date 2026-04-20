# PAT Frontend Integration Guide

This note is for desktop integrator teams that consume `dws` CLI PAT authorization failures and render a first-class auth flow.

## Audience map

- Frontend/Desktop team: parse PAT stderr JSON, render the approval UI, resume the original task.
- CLI/Backend team: keep the PAT JSON envelope stable and avoid mixing human-readable text into passthrough payloads.
- QA team: validate exit code, stderr JSON shape, retry behavior, and legacy `error_code` compatibility.
- Product/Integration owner: use the selector matrix below to decide which approval UX to build first.

## What the CLI emits

When `dws` hits a PAT authorization failure, it writes the raw JSON payload to `stderr` and does not wrap it in the normal CLI error formatter.

The CLI normalizes the payload to a small envelope:

```json
{
  "success": false,
  "code": "PAT_NO_PERMISSION",
  "data": {
    "requiredScopes": ["..."],
    "grantOptions": ["..."],
    "authRequestId": "...",
    "flowId": "..."
  }
}
```

The backend may also surface the same error selector under `error_code` in some responses. Frontends should accept both `code` and `error_code`.
Wrapper keys such as `success`, `message`, `error`, `trace_id`, and `class` should be treated as transport noise, not the contract surface.

Known PAT selectors in the CLI:

- `PAT_NO_PERMISSION`
- `PAT_LOW_RISK_NO_PERMISSION`
- `PAT_MEDIUM_RISK_NO_PERMISSION`
- `PAT_HIGH_RISK_NO_PERMISSION`
- `AGENT_CODE_NOT_EXISTS`

## Fields to expect

- `code` / `error_code`: stable selector for PAT-related failures. Treat it as the primary machine-readable discriminator.
- `success`: always `false` for this path.
- `data.requiredScopes`: the scopes that are missing or need approval.
- `data.grantOptions`: one or more authorization choices the UI can present to the user.
- `data.authRequestId`: correlation id for the auth request. Keep it unchanged in logs, UI state, and follow-up actions.
- `data.flowId`: device-flow or approval-flow identifier. Use it to poll authorization status when present.

Additional fields may appear in `data` depending on the upstream service. Preserve unknown fields for diagnostics.

## Exit behavior

- PAT authorization failures exit with code `4`.
- The CLI sends the raw JSON payload to `stderr`.
- If `flowId` is missing, the CLI cannot poll and will return the PAT payload to the host app instead of continuing locally.
- If `DWS_CHANNEL` contains the local tag `host-control`, the CLI will also return PAT JSON to the host even when `flowId` is present.
- For non-PAT auth failures, do not assume exit code `4`; PAT handling is a separate path.

## DWS_CHANNEL tags

Use `DWS_CHANNEL` as:

```bash
DWS_CHANNEL='Qoderwork;host-control'
```

Rules:

- `Qoderwork` is the real upstream `channelCode`.
- Extra `;tag` suffixes are local CLI tags only.
- The CLI forwards only `Qoderwork` via `x-dws-channel`; tags are not sent upstream.

When `host-control` is present:

- the CLI does not print terminal PAT guidance,
- the CLI does not open the approval link,
- the CLI does not locally poll `flowId`,
- the CLI does not retry the original command.

Instead it returns raw PAT JSON enriched with:

- `data.hostControl`
- `data.callbacks`

## Recommended frontend behavior

Follow a RewindDesktop-style pattern:

1. Detect the PAT payload as soon as `stderr` is available.
2. Treat it as a recoverable authorization event, not a generic error toast.
3. Open a dedicated authorization surface with the requested scopes and any grant options.
4. If `flowId` is present, poll the approval status using that id.
5. If `authRequestId` is present, bind the UI state to it so retries and logs stay correlated.
6. If the user approves, resume the original command path.
7. If the user rejects, the flow expires, or the request is cancelled, close the auth UI and show a retryable failure.

When `host-control` is active:

1. Treat the PAT payload as the only contract surface.
2. Render any custom card or approval flow you want.
3. Call the callback descriptors in `data.callbacks` instead of calling DingTalk APIs directly.
4. Retry the original DWS command only after `poll_flow` reports `APPROVED` and `tokenUpdated=true`.

## Practical parsing rules

- Prefer `code` first, then fall back to `error_code`.
- Do not depend on top-level `message` or other wrapper fields; keep the JSON payload intact and read from `data`.
- Ignore unknown keys so the contract can evolve without breaking the frontend.
- If the payload includes both `requiredScopes` and `grantOptions`, render `requiredScopes` as the reason and `grantOptions` as the action set.
- In host-control mode, use `data.callbacks[*].invoke` as the stable machine surface for follow-up actions.

## Callback contract

Current callbacks in host-control mode:

- `list_super_admins`: fetch admins for a custom picker.
- `send_apply`: submit a permission-change request to a selected admin.
- `poll_flow`: poll a single `flowId`; on approval the CLI exchanges `authCode` and persists the refreshed token.

## Team checklist

### Frontend/Desktop

- Treat exit code `4` as a recoverable PAT event.
- Parse stderr as JSON before showing any generic failure UI.
- Persist `authRequestId` and `flowId` in local UI state so the retry path stays correlated.
- Keep unknown `data.*` fields for diagnostics rather than dropping them.

### CLI/Backend

- Keep raw PAT passthrough on stderr clean for no-`flowId` cases.
- Continue accepting upstream `code`, `errorCode`, and `error_code`.
- Do not silently rename `data.requiredScopes`, `data.grantOptions`, `data.authRequestId`, or `data.flowId`.

### QA

- Verify both `code` and `error_code` inputs trigger PAT classification.
- Verify no-`flowId` PAT responses contain only JSON on stderr.
- Verify `flowId` responses still enter the local retry flow when the CLI owns the auth loop.
