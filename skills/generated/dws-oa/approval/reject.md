---
name: dws-oa-approval-reject
description: "钉钉OA审批: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval reject --help"
---

# oa approval reject

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。

## Usage

```bash
dws oa approval reject --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--instance-id` | ✓ | — | 审批实例Id |
| `--remark` | — | — | 审批意见 |
| `--task-id` | ✓ | — | 审批任务ID |

## Required Fields

- `processInstanceId`
- `taskId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
