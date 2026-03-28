---
name: dws-calendar-room-search
description: "钉钉日历: 根据时间筛选出符合闲忙条件的会议室列表。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar room search --help"
---

# calendar room search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据时间筛选出符合闲忙条件的会议室列表。

## Usage

```bash
dws calendar room search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | ✓ | — | 结束时间，时间戳（毫秒级） |
| `--group-id` | — | — | 会议室分组ID。可选字段。若不填写，则默认查询根目录下的空闲会议室。
建议使用方式：首次查询时请留空此字段；若因当前企业会议室数量超过100条而返回错误，请先调用分页查询当前企业下的会议室分组列表，再根据具体的分组ID分别查询各分组下的会议室数据。 |
| `--available` | — | — | - |
| `--start` | ✓ | — | 开始时间，时间戳（毫秒级） |

## Required Fields

- `endTime`
- `startTime`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
