---
name: dws-oa-approval-list-initiated
description: "钉钉OA审批: 查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval list-initiated --help"
---

# oa approval list-initiated

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息

## Usage

```bash
dws oa approval list-initiated --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | ✓ | — | 审批实例开始时间，Unix时间戳，单位毫秒 |
| `--max-results` | ✓ | — | 分页查询的每页大小，最大值20 |
| `--next-token` | ✓ | — | 分页查询的分页游标，如果是首次查询，该参数传0，非首次调用，传上次返回的nextToken |
| `--process-code` | ✓ | — | 需要查询实例列表的表单processCode |
| `--start` | ✓ | — | 审批实例开始时间，Unix时间戳，单位毫秒 |

## Required Fields

- `endTime`
- `maxResults`
- `nextToken`
- `processCode`
- `startTime`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
