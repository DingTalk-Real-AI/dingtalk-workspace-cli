# cli_to_mcp smoke tests

This directory contains lightweight command-to-tool contract tests for hardcoded
DWS commands synced from `dws-wukong`.

The tests do not call live DingTalk APIs. They exercise command help, validation,
and `--dry-run` output so command paths and MCP argument mappings stay stable.

Run with an already built binary:

```bash
DWS_BIN=/path/to/dws pytest auto-test/cli_to_mcp/testcases
```

If `DWS_BIN` is not set, the runner falls back to `go run ./cmd` from the repo
root.
