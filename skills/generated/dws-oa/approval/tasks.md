---
name: dws-oa-approval-tasks
description: "钉钉OA审批: 查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval tasks --help"
---

# oa approval tasks

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。

## Usage

```bash
dws oa approval tasks --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--instance-id` | ✓ | — | 待我处理的审批实例Id，可通过list_pending_approvals工具获取 |

## Required Fields

- `processInstanceId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
