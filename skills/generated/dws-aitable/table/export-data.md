---
name: dws-aitable-table-export-data
description: "钉钉 AI 表格: 导出 AI 表格数据的统一入口。
不传 taskId 时，会根据 scope / format 创建一个新的导出任务，并在 timeoutMs 时间内同步等待结果；若在等待窗口内完成，则直接返回 downloadUr…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table export_data --help"
---

# aitable table export_data

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

导出 AI 表格数据的统一入口。
不传 taskId 时，会根据 scope / format 创建一个新的导出任务，并在 timeoutMs 时间内同步等待结果；若在等待窗口内完成，则直接返回 downloadUrl 和 fileName。
传入 taskId 时，不会重新创建任务，而是继续等待该任务；若仍未完成，则继续返回同一个 taskId，供下一次调用继续等待。
当前稳定支持的 scope：all、table、view；暂不开放按字段导出。
当前稳定支持的 format：excel、attachment、excel_and_attachment、excel_with_inline_images。

## Usage

```bash
dws aitable table export_data --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--format` | — | — | 可选，导出格式。创建新任务时必填。
支持值：excel、attachment、excel_and_attachment、excel_with_inline_images。 |
| `--scope` | — | — | 可选，导出范围。创建新任务时必填。
支持值：all（整个 Base）、table（指定数据表）、view（指定视图）。
scope=table 时必须传 tableId；scope=view 时必须传 tableId 和 viewId。 |
| `--tableId` | — | — | 可选，Table ID。scope=table 或 scope=view 时必填；可通过 get_base 获取。 |
| `--taskId` | — | — | 可选，已有导出任务 ID。传入后表示继续等待该任务；此时不要再传 scope、format、tableId、viewId。 |
| `--timeoutMs` | — | — | 可选，单次等待超时时间（毫秒）。默认 30000，最小 200，最大 30000。超时后会返回 taskId，供下一次继续等待。 |
| `--viewId` | — | — | 可选，View ID。scope=view 时必填；可通过 list_views 或 get_views 获取。 |

## Required Fields

- `baseId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
