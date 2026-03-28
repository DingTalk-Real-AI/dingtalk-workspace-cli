---
name: dws-oa-approval-list-pending
description: "钉钉OA审批: 查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval list-pending --help"
---

# oa approval list-pending

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。

## Usage

```bash
dws oa approval list-pending --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | ✓ | — | 审批单创建时间的结束时间。毫秒时间戳格式，表示时间范围的上限。 |
| `--page` | — | — | 分页页码，从1开始 |
| `--size` | — | — | 每页大小 |
| `--start` | ✓ | — | 审批单创建时间的开始时间。毫秒时间戳格式，表示时间范围的下限 |

## Required Fields

- `endTime`
- `starTime`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
