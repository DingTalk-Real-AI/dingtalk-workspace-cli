---
name: dws-bot-create-robot
description: "机器人消息: 创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot create_robot --help"
---

# bot create_robot

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。

## Usage

```bash
dws bot create_robot --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--desc` | ✓ | — | 机器人描述 |
| `--robot-name` | ✓ | — | 机器人名称 |

## Required Fields

- `desc`
- `robot_name`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](./SKILL.md) — Product skill
