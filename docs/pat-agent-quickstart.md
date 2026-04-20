# PAT Agent Quickstart

Use this mode when your host or agent owns the PAT approval UI instead of the CLI terminal UX.

## 1. Enable host-owned PAT with `CLAW_TYPE`

Choose one of the supported values:

```bash
export CLAW_TYPE=host-control
```

Also supported:

- `rewind-desktop`
- `dws-wukong`
- `wukong`

`DWS_CHANNEL` remains the upstream `channelCode` only.

Legacy compatibility only:

```bash
export DWS_CHANNEL='Qoderwork;host-control'
```

Use that suffix form only for older agents that have not migrated to `CLAW_TYPE`.

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

`data.hostControl` means the host owns UI, callback invocation, and retry.

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
