---
name: dws-aitable-table-create
description: "钉钉 AI 表格: 在指定 Base 中新建表格，并可在创建时附带初始化一批基础字段。
建表时单次最多附带 15 个字段；若 fields 为空，服务会自动补一个名为“标题”的 primaryDoc 首列。
若 tableName 与…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table create --help"
---

# aitable table create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在指定 Base 中新建表格，并可在创建时附带初始化一批基础字段。
建表时单次最多附带 15 个字段；若 fields 为空，服务会自动补一个名为“标题”的 primaryDoc 首列。
若 tableName 与当前 Base 下已有表重名，服务会自动续号为“原名 1 / 原名 2 ...”，并在 summary 中返回当前表名。
如需添加更多字段，或在已有表中增加字段，请使用 create_fields。

## Usage

```bash
dws aitable table create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 目标 Base ID（通过 list_bases 获取） |
| `--fields` | ✓ | — | 建表时随附创建的初始字段列表，至少包含 1 个字段，单次最多 15 个。若传空数组，系统会自动补一个名为“标题”的 primaryDoc 首列。
建议在此处定义结构清晰的基础字段（如文本、数字、日期、单选等）；
复杂字段（关联、流转等）建议建表完成后通过 create_fields 单独添加。

每个字段对象包含：
  fieldName（必填）: 字段名称
  type（必填）: 字段类型，可选值与 config 结构详见本工具说明末尾的字段参考
  config（可选）: 字段配置，结构因 type 而异，详见字段参考

示例：
[
  {"fieldName":"任务名称","type":"text"},
  {"fieldName":"优先级","type":"singleSelect","config":{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}},
  {"fieldName":"截止日期","type":"date","config":{"formatter":"YYYY-MM-DD"}},
  {"fieldName":"负责人","type":"user","config":{"multiple":false}}
] |
| `--name` | ✓ | — | 表格名称，1~100 个字符；不能包含 / \ ? * [ ] : 等字符。 |

## Required Fields

- `baseId`
- `fields`
- `tableName`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
