---
name: dws-contact-user-get
description: "钉钉通讯录: 获取指定用户 ID 列表对应的员工详细信息，包括人员基本信息（ID、名称、主管名称、主管userId等）、所属角色信息、所在部门信息。返回结果受组织可见性规则限制：若调用者无权查看某员工（如部门隐藏、手机号设为私密等），则相…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact user get --help"
---

# contact user get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定用户 ID 列表对应的员工详细信息，包括人员基本信息（ID、名称、主管名称、主管userId等）、所属角色信息、所在部门信息。返回结果受组织可见性规则限制：若调用者无权查看某员工（如部门隐藏、手机号设为私密等），则相应字段可能被过滤或不返回该员工。适用于需要批量获取同事信息的场景，如组织架构展示、审批人选择等。仅返回调用者权限范围内的有效数据。

## Usage

```bash
dws contact user get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--ids` | ✓ | — | user_id的列表。可能也叫userId |

## Required Fields

- `user_id_list`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
