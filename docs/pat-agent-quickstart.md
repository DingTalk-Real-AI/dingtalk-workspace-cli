# PAT Agent Quickstart

Use this mode when your host or agent wants to own the PAT card UI instead of using the CLI's terminal UX.

## 1. Enable host-owned PAT

```bash
export DWS_CHANNEL='Qoderwork;host-control'
```

- `Qoderwork` is the real upstream `channelCode`
- `host-control` is a local CLI tag that forces raw PAT JSON handoff even when `flowId` exists

Only `Qoderwork` is sent upstream via `x-dws-channel`.

## 2. Run the original DWS command

If PAT authorization is required, the CLI exits with code `4` and writes PAT JSON to `stderr`.

## 3. Parse the PAT JSON

Read:

- `code` / `error_code`
- `data.requiredScopes`
- `data.grantOptions`
- `data.authRequestId`
- `data.flowId`
- `data.hostControl`
- `data.callbacks`

`data.hostControl` means the host owns UI, polling, and retry.

## 4. Use the callback commands

- `dws pat callback list-super-admins`
- `dws pat callback send-apply --admin-staff-id <id>`
- `dws pat callback poll-flow --flow-id <id>`

`poll-flow` is one-shot. On approval it exchanges `authCode`, persists the refreshed token, and returns:

- `status = "APPROVED"`
- `tokenUpdated = true`
- `retrySuggested = true`

After that, retry the original DWS command yourself.
