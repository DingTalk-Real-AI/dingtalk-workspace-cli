---
name: dws-shared
description: "DWS shared reference for authentication, command patterns, and safety rules."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
---

# dws - Shared Reference

## Installation

Ensure `dws` is installed and accessible from `$PATH`.

## Authentication

```bash
dws auth login
dws auth status
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--format <FORMAT>` | Output format: `json`, `table`, `raw` |
| `--dry-run` | Preview the operation without executing it |
| `--verbose` | Show verbose logs |
| `--yes` | Skip confirmation prompts for sensitive operations |

## Global Rules

- Output defaults to JSON. Use `--format table` for human-readable output.
- Confirm with user before any write/delete/revoke action.
- Never fabricate IDs; always extract from command output.
- For risky operations, run a read/list check before executing write operations.

## Command Pattern

```bash
dws <product> <tool> --json '{...}'
dws schema <product>
dws schema <product>.<tool>
```

## Services

- `dws-ding`
- `dws-bot`
- `dws-aitable`
- `dws-oa`
- `dws-workbench`
- `dws-devdoc`
- `dws-todo`
- `dws-calendar`
- `dws-report`
- `dws-group-chat`
- `dws-attendance`
- `dws-contact`
