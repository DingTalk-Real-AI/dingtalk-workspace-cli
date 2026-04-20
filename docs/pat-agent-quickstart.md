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
- `data.callbacks`
- optional `data.flowId`

When `claw-type != default`, `data.hostControl` means the host owns UI, callback invocation, and retry.

Special case:

- `PAT_SCOPE_AUTH_REQUIRED` may not include `flowId`
- do not assume polling is available for every PAT event

## 4. Use the callback commands

- `dws pat callback list-super-admins`
- `dws pat callback send-apply --admin-staff-id <id>`
- `dws pat callback poll-flow --flow-id <id>`
- `dws auth login --scope <scope>` when the callback name is `auth_login`

Use the callback descriptors in `data.callbacks` to decide which command to invoke. Do not call DingTalk PAT APIs directly from the host.

## 5. Retry only after approval

`poll-flow` is one-shot. On approval it exchanges `authCode`, persists the refreshed token, and can return:

- `status = "APPROVED"`
- `tokenUpdated = true`
- `retrySuggested = true`

Only retry the original DWS command after `retrySuggested = true`.
