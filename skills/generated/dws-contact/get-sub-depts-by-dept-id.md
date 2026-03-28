---
name: dws-contact-get-sub-depts-by-dept-id
description: "钉钉通讯录: 根据指定的部门 ID，获取其直接子部门列表，返回每个子部门的部门 ID、名称。结果受组织架构可见性控制：仅返回调用者有权限查看的子部门；若父部门不可见或无子部门，则返回空列表。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact get_sub_depts_by_dept_id --help"
---

# contact get_sub_depts_by_dept_id

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据指定的部门 ID，获取其直接子部门列表，返回每个子部门的部门 ID、名称。结果受组织架构可见性控制：仅返回调用者有权限查看的子部门；若父部门不可见或无子部门，则返回空列表。

## Usage

```bash
dws contact get_sub_depts_by_dept_id --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--deptId` | ✓ | — | 部门Id |

## Required Fields

- `deptId`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](./SKILL.md) — Product skill
