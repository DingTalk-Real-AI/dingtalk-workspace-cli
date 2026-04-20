# PAT Agent Quickstart

Use this mode when your host or agent owns the PAT approval UI instead of the CLI terminal UX.

## 1. Set `DINGTALK_AGENT`

Third-party business developers set their business-specific agent name through `DINGTALK_AGENT`.

```bash
export DINGTALK_AGENT=sales-copilot
```

Effective request header:

```http
claw-type: sales-copilot
```

Default behavior:

- `DINGTALK_AGENT` empty: `claw-type: default`
- `DINGTALK_AGENT=default`: `claw-type: default`
- `claw-type != default`: PAT returns JSON and the host handles all UI and logic

`DWS_CHANNEL` remains the upstream `channelCode` only. Do not use it to enable host-owned PAT.

## 2. Run the original DWS command

If PAT authorization is required, the CLI exits with code `4` and writes PAT JSON to `stderr`.

## 3. Parse the PAT JSON

Read:

- `code` / `error_code`
- `data.requiredScopes`
- `data.grantOptions`
- `data.authRequestId`
- `data.hostControl`
- optional `data.flowId`
- optional `data.missingScope`

When `claw-type != default`, `data.hostControl` means the host owns UI, follow-up handling, and retry.

Special case:

- `PAT_SCOPE_AUTH_REQUIRED` may not include `flowId`
- do not assume polling is available for every PAT event

## 4. Handle follow-up in the host

The open-source CLI does not expose a dedicated `dws pat callback ...` surface.
Hosts are responsible for their own follow-up logic after parsing the PAT payload.

Typical host-owned actions:

- render an approval card from `requiredScopes` and `grantOptions`
- bind host state to `authRequestId`
- list approvers or super admins through host-managed services
- submit approval requests through host-managed services
- poll approval state only when `flowId` is present
- for `PAT_SCOPE_AUTH_REQUIRED`, run `dws auth login --scope <scope>` or trigger an equivalent host re-auth flow

## 5. Retry only after approval

Only retry the original DWS command after the host confirms approval is complete and any required token refresh has finished.
