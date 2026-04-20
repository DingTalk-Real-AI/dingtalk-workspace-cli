# PAT Frontend Integration Guide

This guide is for desktop and agent hosts that intercept `dws` PAT authorization failures and render a first-class approval flow.

## Audience map

- Frontend/Desktop team: classify PAT stderr JSON, render the approval UI, and retry the original task.
- CLI/Backend team: keep the PAT payload shallow and stable.
- QA team: validate exit code, stderr JSON shape, host retry behavior, and selector handling.
- Integration owner: set `DINGTALK_AGENT=<business-agent-name>` for each business host integration.

## Primary host-control rule

Third-party business developers assign their business-specific agent name through `DINGTALK_AGENT`.

Effective request header:

```http
claw-type: <business-agent-name or default>
```

Rules:

- `DINGTALK_AGENT` empty or `default`: use the default DWS behavior.
- `claw-type != default`: when PAT is hit, DWS returns JSON and the host handles all UI and logic.

`DWS_CHANNEL` is not a host-control switch. It stays as the upstream `channelCode` only.

## Reference model from existing hosts

Use the existing split of responsibilities as the reference implementation:

- `dws-wukong` keeps the CLI side shallow: classify PAT, normalize raw JSON, exit with code `4`, and pass stderr through unchanged.
- `RewindDesktop` acts as the real host: parse PAT JSON from `stderr`, decide whether to render a card, bind host state to optional identifiers such as `authRequestId`, and trigger follow-up actions from the host side.

That split is the model this open-source contract now follows as well:

- CLI owns normalization only.
- Host owns rendering, user interaction, follow-up logic, and retry of the original command.

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
      "clawType": "sales-copilot",
      "mode": "host",
      "pollingOwner": "host",
      "retryOwner": "host"
    },
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
- `data.authRequestId`: correlation id for host state and logs.
- `data.hostControl`: indicates the host owns UI, polling, and retry.
- `data.flowId`: optional approval-flow id. It may be absent, especially for `PAT_SCOPE_AUTH_REQUIRED`.
- `data.missingScope`: optional missing scope for `PAT_SCOPE_AUTH_REQUIRED`.

Preserve unknown `data.*` fields for diagnostics and forward compatibility.

## Exit behavior

- PAT authorization failures exit with code `4`.
- PAT JSON is written to `stderr` without the normal CLI formatter.
- If `flowId` is absent, the host must not assume polling is possible.
- When `claw-type != default`, the host decides whether to show approval UI, run its own follow-up flow, and retry the original DWS command.

## Recommended host behavior

1. Detect PAT JSON as soon as `stderr` is available.
2. Treat it as a recoverable authorization event, not a generic command failure.
3. Use `code` first and fall back to `error_code`.
4. Bind UI state to `authRequestId` when present.
5. Render `requiredScopes` as the reason for authorization and `grantOptions` as the action set.
6. Implement follow-up actions inside the host or host-managed backend rather than relying on CLI callback commands.
7. Only poll when `flowId` is present.
8. For `PAT_SCOPE_AUTH_REQUIRED`, run `dws auth login --scope <scope>` or trigger an equivalent host-managed re-auth flow.
9. Retry the original DWS command only after the host confirms approval and token state are ready.

## Parsing rules

- Do not depend on top-level `message`, `error`, `trace_id`, or other wrapper fields.
- Do not require `flowId` to classify PAT.
- Do not require `flowId` for `PAT_SCOPE_AUTH_REQUIRED`.
- Ignore unknown keys so the contract can evolve safely.

## Compatibility invariants

- `DINGTALK_AGENT` maps to `claw-type: <business-agent-name or default>`.
- Empty `DINGTALK_AGENT` and `DINGTALK_AGENT=default` both mean default DWS behavior.
- When `claw-type != default`, PAT returns JSON and the host handles all UI and logic.
- `DWS_CHANNEL` remains upstream channel metadata only.
- The stable PAT CLI surface is `dws pat chmod`; follow-up approval logic is host-owned.

## Team checklist

### Frontend/Desktop

- Treat exit code `4` as a recoverable PAT event.
- Parse stderr as JSON before showing any generic failure UI.
- Persist `authRequestId` and optional `flowId` in host state.
- Handle `PAT_SCOPE_AUTH_REQUIRED` without assuming polling is available.

### CLI/Backend

- Keep raw PAT stderr payloads clean.
- Keep `data.hostControl` and the core PAT fields stable.
- Keep `DWS_CHANNEL` documented as upstream channel only, with no host-control fallback.

### QA

- Verify `DINGTALK_AGENT=<business-agent-name>` results in `claw-type=<business-agent-name>`.
- Verify empty `DINGTALK_AGENT` and `DINGTALK_AGENT=default` both keep default DWS behavior.
- Verify `claw-type != default` PAT flows return JSON for host-owned handling.
- Verify PAT host-owned flows do not depend on `DWS_CHANNEL`.
- Verify `PAT_SCOPE_AUTH_REQUIRED` is handled correctly with and without `flowId`.
- Verify host-owned approval flows can retry the original command after authorization completes.
