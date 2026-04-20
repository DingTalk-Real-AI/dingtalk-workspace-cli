# PAT Frontend Integration Guide

This note is for desktop integrator teams that consume `dws` CLI PAT authorization failures and render a first-class auth flow.

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
- For non-PAT auth failures, do not assume exit code `4`; PAT handling is a separate path.

## Recommended frontend behavior

Follow a RewindDesktop-style pattern:

1. Detect the PAT payload as soon as `stderr` is available.
2. Treat it as a recoverable authorization event, not a generic error toast.
3. Open a dedicated authorization surface with the requested scopes and any grant options.
4. If `flowId` is present, poll the approval status using that id.
5. If `authRequestId` is present, bind the UI state to it so retries and logs stay correlated.
6. If the user approves, resume the original command path.
7. If the user rejects, the flow expires, or the request is cancelled, close the auth UI and show a retryable failure.

## Practical parsing rules

- Prefer `code` first, then fall back to `error_code`.
- Do not depend on top-level `message` or other wrapper fields; keep the JSON payload intact and read from `data`.
- Ignore unknown keys so the contract can evolve without breaking the frontend.
- If the payload includes both `requiredScopes` and `grantOptions`, render `requiredScopes` as the reason and `grantOptions` as the action set.
