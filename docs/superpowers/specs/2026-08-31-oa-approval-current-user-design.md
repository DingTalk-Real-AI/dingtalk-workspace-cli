# OA approval list commands narrow current-user and status/result filters

## Goal

The four approval list commands must always query for the user represented by
the active DWS login/profile. Callers must not be able to override that user by
passing `--user-id`, and each command must expose only its supported approval
status/result filters.

Affected command pairs:

- `dws oa approval list-pending` / `dws oa +list-pending`
- `dws oa approval list-executed` / `dws oa +list-executed`
- `dws oa approval list-submitted` / `dws oa +list-submitted`
- `dws oa approval list-cc` / `dws oa +list-cc`

## Interface changes

Remove `--user-id` from each affected Cobra or shortcut flag set and from its
declared Schema parameters. Passing the retired flag must fail as an unknown
flag before any MCP call instead of being accepted or silently ignored.

The status/result filters are also narrowed per command:

| Command | `--process-instance-status` | `--process-instance-result` |
|---|---|---|
| `list-pending` / `+list-pending` | remove | remove |
| `list-executed` / `+list-executed` | keep | remove |
| `list-submitted` / `+list-submitted` | keep | remove |
| `list-cc` / `+list-cc` | remove | remove |

Each removed flag must fail as unknown before any MCP call and must be absent
from the corresponding Schema tool. The MCP payload builder must likewise omit
the removed `processInstanceStatus` or `processInstanceResult` property.

Update affected Help, Contract descriptions, and Agent selection prose that
currently say “指定用户” to say “当前登录用户”. The global `--profile` option
remains the supported way to select another authenticated account/profile.
Also remove any Help, Contract, example, or Agent-selection claim that an
affected command supports a status/result filter retired by the matrix above.

Keep `--originator-user-id`. It filters by the approval instance originator and
does not select the authenticated/current user.

## MCP request behavior

The shared approval-list argument builders must not emit the `userId` property.
They must expose status/result bindings only when enabled for the specific
command by the matrix above. The MCP tools determine the current user from the
authenticated DWS session. All other recently added filters and their property
mappings remain unchanged.

## Documentation and generated inputs

Remove `--user-id` and the command-specific retired status/result flags from the
affected approval-list sections in the mono and multi OA references. Run
`make generate-schema` to regenerate parameter aliases and verify Schema
assembly determinism. Do not generate or commit a Catalog, and do not add a
compatibility alias for any retired flag.

## Tests

Before changing production declarations, add regression assertions that fail
against the current implementation:

1. Each of the four primary and four shortcut paths has no `user-id` flag.
2. The eight paths expose status/result flags exactly as specified by the
   command matrix.
3. Executing each path with any flag retired from that path fails before any MCP
   call.
4. Each MCP call omits `userId` and every retired status/result property while
   retaining the allowed filter mappings.
5. The assembled Schema for all four primary and four shortcut canonical tools
   omits `user-id` and the command-specific retired status/result parameters.
6. `--originator-user-id` remains declared and mapped to `originatorUserId`.
7. Pending retains `create-before`/`createBefore`; CC retains
   `unread-only`/`unreadOnly`, including explicit `--unread-only=false`.
8. Global `--profile` behavior is unchanged.

After implementation, run the focused OA/helper/app tests, generation and
Schema policy checks, `gofmt`, `git diff --check`, and the repository build.

## Non-goals

- Do not alter the upstream MCP input schemas.
- Do not remove `userId` concepts from unrelated OA commands.
- Do not change authentication/profile selection behavior.
