---
name: dws-report-sent
description: "钉钉日志: 查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report sent --help"
---

# report sent

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息

## Usage

```bash
dws report sent --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | ✓ | — | 分页游标，首次传0 |
| `--end` | — | — | 日志创建的结束时间，毫秒级时间戳格式，示例：1734048000；注意：开始时间和结束时间跨度不能超过180天 |
| `--modified-end` | — | — | 日志修改的结束时间，毫秒级时间戳格式，示例：1734048000 |
| `--modified-start` | — | — | 日志修改的开始时间，毫秒级时间戳格式，示例：1734048000 |
| `--template-name` | — | — | 日志模板名称，可不传，查询的是全部日志 |
| `--size` | ✓ | — | 分页大小，最大20 |
| `--start` | — | — | 日志创建的开始时间，毫秒级时间戳格式，示例：1734048000；注意：开始时间和结束时间跨度不能超过180天 |

## Required Fields

- `cursor`
- `size`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](./SKILL.md) — Product skill
