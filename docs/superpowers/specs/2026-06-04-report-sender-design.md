# Report Sender Hybrid Submission Design

## Goal

Allow `dws report entry submit` and its deprecated `dws report create` alias to
submit a report on behalf of a specified employee with:

```bash
dws report entry submit --sender-user-id <userId> ...
```

## Constraints

- The remote MCP `report.create_report` schema does not expose a sender field.
- DingTalk's legacy OpenAPI `POST /topapi/report/create` supports the required
  `create_report_param.userid` field.
- Raw OpenAPI calls require the user's own AppKey/AppSecret and the
  "管理员工日志数据" permission. The encrypted default MCP credential cannot
  be used.
- Existing invocations without `--sender-user-id` must retain their current MCP
  behavior.

## Architecture

Submission uses two routes selected by the presence of `--sender-user-id`:

1. Without the flag, the existing command handler calls MCP
   `report.create_report` unchanged.
2. With the flag, a post-merge report hook intercepts the command and calls
   DingTalk OAPI `POST https://oapi.dingtalk.com/topapi/report/create`.

The OAPI route builds the legacy request body:

```json
{
  "create_report_param": {
    "userid": "<sender-user-id>",
    "template_id": "<template-id>",
    "dd_from": "dws",
    "to_chat": false,
    "to_userids": [],
    "contents": [
      {
        "key": "...",
        "sort": "0",
        "type": "1",
        "content_type": "markdown",
        "content": "..."
      }
    ]
  }
}
```

CLI camelCase content keys are converted to the OAPI snake_case shape. Unknown
content fields are preserved when possible so the route does not discard
forward-compatible values.

## Command Integration

A post-merge hook adds `--sender-user-id` to:

- `dws report entry submit`
- `dws report create`

The hook wraps the existing `RunE`. If the flag is empty, it delegates directly
to the original handler. If the flag is set, it validates and parses the
existing flags, then invokes the OAPI submitter.

This avoids changing the discovery envelope or sending an unsupported
`senderUserId` argument to MCP.

## Authentication And Errors

The OAPI route obtains an app-level token from the existing
`auth.AppTokenProvider`, using credentials already resolved from
`--client-id`/`--client-secret`, environment variables, or auth configuration.

If credentials are missing, the command returns an actionable authentication
error. DingTalk HTTP and business errors are surfaced without falling back to
MCP, because fallback would silently submit as the wrong sender.

## Dry Run And Output

With `--sender-user-id --dry-run`, the command prints a structured preview of
the OAPI request and does not resolve a token or perform network I/O. The
preview must include the selected sender but never expose credentials.

Successful OAPI responses are emitted through the normal command output path.
The returned DingTalk `result` report ID remains available to callers.

## Testing

Tests cover:

- The flag is attached to both canonical and deprecated command paths.
- No sender delegates to the original MCP handler unchanged.
- A sender selects OAPI and maps all request fields correctly.
- `--contents-file`, inline `--contents`, recipients, and `to-chat` map to the
  OAPI request.
- Dry run performs no network or token lookup.
- Missing app credentials and DingTalk business errors are clear and do not
  fall back to MCP.
- Existing report tests continue to pass.

## Documentation

Update the multi and mono report references, skill summary, command help, and
`CHANGELOG.md`. Documentation must state that `--sender-user-id` requires the
caller's own app credentials and the employee report management permission.
