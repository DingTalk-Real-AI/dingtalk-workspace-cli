---
name: dws-contact-dept-search
description: "钉钉通讯录: 根据关键词模糊搜索部门，返回匹配的部门列表，包含每个部门的 ID、名称。搜索范围限于调用者有权限查看的组织架构；若关键词无匹配结果或部门因可见性设置被隐藏，则相应部门不会出现在结果中。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact dept search --help"
---

# contact dept search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据关键词模糊搜索部门，返回匹配的部门列表，包含每个部门的 ID、名称。搜索范围限于调用者有权限查看的组织架构；若关键词无匹配结果或部门因可见性设置被隐藏，则相应部门不会出现在结果中。

## Usage

```bash
dws contact dept search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--keyword` | ✓ | — | 搜索关键词 |

## Required Fields

- `query`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
