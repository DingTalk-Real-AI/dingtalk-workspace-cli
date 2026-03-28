---
name: dws-aitable-field-get
description: "钉钉 AI 表格: 批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置（如格式化、选项、AI 配置等）。
传 fieldIds 时单次最多获取 10 个字段；若需更多字段，请拆分多…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable field get --help"
---

# aitable field get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置（如格式化、选项、AI 配置等）。
传 fieldIds 时单次最多获取 10 个字段；若需更多字段，请拆分多次调用。
适用于在 get_tables 拿到字段目录后，按需展开少量字段的完整配置，避免大 options 字段放大 get_tables 返回值。
AI 字段的返回结果中，config 仅包含字段物理配置，aiConfig 作为同级字段单独返回，结构与 create_fields 写入参数一致。

## Usage

```bash
dws aitable field get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID（可通过 list_bases 获取） |
| `--field-ids` | — | — | 待获取详情的字段 ID 列表，可通过 get_tables 获取；建议只传真正需要展开完整配置的字段，单次最多 10 个；不传则默认返回当前表下全部字段。建议优先显式传入，以控制返回体大小，避免上下文突增 |
| `--table-id` | ✓ | — | Table ID（可通过 get_base 获取） |

## Required Fields

- `baseId`
- `tableId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
