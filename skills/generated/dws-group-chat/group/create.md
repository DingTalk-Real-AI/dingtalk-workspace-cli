---
name: dws-group-chat-group-create
description: "钉钉群聊: 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat group create --help"
---

# group-chat group create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。

## Usage

```bash
dws chat group create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--users` | ✓ | — | 初始化添加的群聊成员。需要使用钉钉userId |
| `--name` | ✓ | — | 群名称 |

## Required Fields

- `groupMembers`
- `groupName`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../SKILL.md) — Product skill
