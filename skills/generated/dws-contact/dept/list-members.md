---
name: dws-contact-dept-list-members
description: "钉钉通讯录: 获取指定部门下的所有成员，返回每位成员的用户 ID（userId）和显示名称（如真实姓名或昵称）。结果受组织可见性控制：若调用者无权查看某成员（例如该成员所在子部门被隐藏，或其个人信息设为私密），则该成员不会出现在返回列表中…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact dept list-members --help"
---

# contact dept list-members

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定部门下的所有成员，返回每位成员的用户 ID（userId）和显示名称（如真实姓名或昵称）。结果受组织可见性控制：若调用者无权查看某成员（例如该成员所在子部门被隐藏，或其个人信息设为私密），则该成员不会出现在返回列表中。适用于需要展示部门人员列表、选择协作成员等场景，仅支持调用者有权限访问的部门。

## Usage

```bash
dws contact dept list-members --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--ids` | ✓ | — | 需要获取部门的ID |

## Required Fields

- `deptIds`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
