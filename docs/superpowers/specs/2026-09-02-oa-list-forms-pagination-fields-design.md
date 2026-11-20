# OA list-forms Pagination Fields Design

## Goal

Ensure `dws oa approval list-forms` preserves the OA MCP single-page response
fields `result.hasMore` and `result.nextCursor` after the downstream
`list_user_visible_process` contract added them, and lock that behavior with an
end-to-end command regression test.

## Scope

- Keep the command single-page: `--cursor` and `--limit` continue to select one
  page and no automatic pagination flags are added.
- Keep the legacy runtime response envelope and all existing input mappings.
- Document in the atomic command help that the returned single page contains
  `result.hasMore` and `result.nextCursor`, and that callers pass the latter as
  the next invocation's `--cursor` when `hasMore=true`.
- Add a command-level regression test using the complete downstream response
  shape. Assert one MCP call, exact `cursor`/`pageSize` request arguments, and
  final JSON stdout containing both pagination fields.
- Do not change `oa +list-forms`, its reviewed semantic catalog, or any Agent
  discoverability setting; the user-requested scope is the atomic command.

## Data Flow

`oa approval list-forms` continues to call `oa/list_user_visible_process` with
numeric `cursor` and `pageSize`. The shared MCP renderer continues to decode
and print the complete JSON response without projection. Because this leaf is
still on the legacy output path, it does not declare a `ResultSpec`: current
Schema assembly intentionally removes result declarations from non-unified
commands, and adding one would not publish the fields.

## Error Handling

The atomic command does not synthesize pagination values. Missing or malformed
fields remain visible exactly as returned by the MCP service. Strict validation
belongs to `oa +list-forms`, which already rejects a missing/non-boolean
`hasMore`, a missing/non-numeric `nextCursor` when more data exists, and a
non-advancing cursor.

## Testing

1. A runtime command test feeds the complete MCP response fixture and verifies
   `result.hasMore` and `result.nextCursor` in final `--format json` output.
2. The same test verifies exactly one `list_user_visible_process` call with the
   requested numeric `cursor` and `pageSize`, proving `hasMore=true` does not
   trigger automatic pagination.
3. Run focused helper tests, Schema policy checks, the full Go test suite, and
   `make build`.
