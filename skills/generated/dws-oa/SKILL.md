---
name: dws-oa
description: "钉钉OA审批MCP服务，支持查询待处理审批、审批详情、同意/拒绝/撤销审批、操作记录、已发起实例列表、待审批任务及可见表单列表。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa --help"
---

# oa

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉OA审批
- Description: 钉钉OA审批MCP服务，支持查询待处理审批、审批详情、同意/拒绝/撤销审批、操作记录、已发起实例列表、待审批任务及可见表单列表。
- Endpoint: `https://mcp-gw.dingtalk.com/server/8faff71bdfc3cb5437894ada5305b48214eb56408ca31e378f4be2773ba4500c`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws oa <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-oa-approval-approve`](./approval/approve.md) | `approve_processInstance` | 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。 |
| [`dws-oa-approval-detail`](./approval/detail.md) | `get_processInstance_detail` | 获取指定审批实例的详情信息 |
| [`dws-oa-approval-records`](./approval/records.md) | `get_processInstance_records` | 获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么 |
| [`dws-oa-approval-list-initiated`](./approval/list-initiated.md) | `list_initiated_instances` | 查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息 |
| [`dws-oa-approval-list-pending`](./approval/list-pending.md) | `list_pending_approvals` | 查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。 |
| [`dws-oa-approval-tasks`](./approval/tasks.md) | `list_pending_tasks` | 查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。 |
| [`dws-oa-approval-list-forms`](./approval/list-forms.md) | `list_user_visible_process` | 获取当前用户可见的审批表单列表，可获取审批表单的processCode。 |
| [`dws-oa-approval-reject`](./approval/reject.md) | `reject_processInstance` | 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。 |
| [`dws-oa-approval-revoke`](./approval/revoke.md) | `revoke_processInstance` | 撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从 |

## API Tools

### `approve_processInstance`

- Canonical path: `oa.approve_processInstance`
- CLI route: `dws oa approval approve`
- Description: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
- Required fields: `processInstanceId`, `taskId`
- Sensitive: `false`

### `get_processInstance_detail`

- Canonical path: `oa.get_processInstance_detail`
- CLI route: `dws oa approval detail`
- Description: 获取指定审批实例的详情信息
- Required fields: `processInstanceId`
- Sensitive: `false`

### `get_processInstance_records`

- Canonical path: `oa.get_processInstance_records`
- CLI route: `dws oa approval records`
- Description: 获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么
- Required fields: `processInstanceId`
- Sensitive: `false`

### `list_initiated_instances`

- Canonical path: `oa.list_initiated_instances`
- CLI route: `dws oa approval list-initiated`
- Description: 查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息
- Required fields: `endTime`, `maxResults`, `nextToken`, `processCode`, `startTime`
- Sensitive: `false`

### `list_pending_approvals`

- Canonical path: `oa.list_pending_approvals`
- CLI route: `dws oa approval list-pending`
- Description: 查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。
- Required fields: `endTime`, `starTime`
- Sensitive: `false`

### `list_pending_tasks`

- Canonical path: `oa.list_pending_tasks`
- CLI route: `dws oa approval tasks`
- Description: 查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。
- Required fields: `processInstanceId`
- Sensitive: `false`

### `list_user_visible_process`

- Canonical path: `oa.list_user_visible_process`
- CLI route: `dws oa approval list-forms`
- Description: 获取当前用户可见的审批表单列表，可获取审批表单的processCode。
- Required fields: `cursor`, `pageSize`
- Sensitive: `false`

### `reject_processInstance`

- Canonical path: `oa.reject_processInstance`
- CLI route: `dws oa approval reject`
- Description: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
- Required fields: `processInstanceId`, `taskId`
- Sensitive: `false`

### `revoke_processInstance`

- Canonical path: `oa.revoke_processInstance`
- CLI route: `dws oa approval revoke`
- Description: 撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从
- Required fields: `processInstanceId`
- Sensitive: `true`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema oa                     # inspect product tools (JSON)
dws schema oa.<tool>              # inspect tool schema (JSON)
```
