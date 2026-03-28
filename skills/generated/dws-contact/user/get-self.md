---
name: dws-contact-user-get-self
description: "钉钉通讯录: 获取当前登录用户的基本信息（如姓名、工号、手机号）、当前组织信息（corpId、组织名称）、直属主管信息、所属部门列表（含部门 ID 与名称）以及角色信息（如管理员类型、自定义角色标签等）。返回内容受组织隐私与权限策略控制：…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact user get-self --help"
---

# contact user get-self

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取当前登录用户的基本信息（如姓名、工号、手机号）、当前组织信息（corpId、组织名称）、直属主管信息、所属部门列表（含部门 ID 与名称）以及角色信息（如管理员类型、自定义角色标签等）。返回内容受组织隐私与权限策略控制：若某些字段（如主管、手机号）被设为不可见，则可能被过滤或省略。

## Usage

```bash
dws contact user get-self --json '{...}'
```

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
