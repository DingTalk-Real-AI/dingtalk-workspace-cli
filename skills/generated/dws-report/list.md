---
name: dws-report-list
description: "钉钉日志: 查询当前人收到的日志列表."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report list --help"
---

# report list

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询当前人收到的日志列表

## Usage

```bash
dws report list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | ✓ | — | 起始游标，从 0 开始，必填。 |
| `--end` | ✓ | — | 结束时间，毫秒时间戳，必填。 |
| `--size` | ✓ | — | 每页条数，最大 20，必填。 |
| `--start` | ✓ | — | 开始时间，毫秒时间戳，必填。 |

## Required Fields

- `cursor`
- `endTime`
- `size`
- `startTime`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](./SKILL.md) — Product skill
