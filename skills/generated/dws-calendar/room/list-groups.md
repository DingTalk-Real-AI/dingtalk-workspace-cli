---
name: dws-calendar-room-list-groups
description: "钉钉日历: 分页查询当前企业下的会议室分组列表，返回每个分组的名称（groupName）、唯一 ID（groupId）及其父分组 ID（parentId，0 表示根分组）。结果按组织架构权限过滤，仅包含调用者有权限查看的分组。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar room list-groups --help"
---

# calendar room list-groups

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

分页查询当前企业下的会议室分组列表，返回每个分组的名称（groupName）、唯一 ID（groupId）及其父分组 ID（parentId，0 表示根分组）。结果按组织架构权限过滤，仅包含调用者有权限查看的分组。

## Usage

```bash
dws calendar room list-groups --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--pageIndex` | — | — | 分页开始位置 ，不填默认 0 |
| `--pageSize` | — | — | 页大小，不填默认 100。超过100的，按照100来处理 |

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
