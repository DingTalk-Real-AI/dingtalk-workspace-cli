# Canonical Product: oa

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉OA审批
- Description: 钉钉OA审批MCP服务，支持查询待处理审批、审批详情、同意/拒绝/撤销审批、操作记录、已发起实例列表、待审批任务及可见表单列表。
- Server key: `2721767f4caa8a6e`
- Endpoint: `https://mcp-gw.dingtalk.com/server/8faff71bdfc3cb5437894ada5305b48214eb56408ca31e378f4be2773ba4500c`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `approval approve`
  - Path: `oa.approve_processInstance`
  - CLI route: `dws oa approval approve`
  - Description: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
  - Flags: `--instance-id`, `--remark`, `--task-id`
  - Schema: `skills/generated/docs/schema/oa/approve_processInstance.json`
- `approval detail`
  - Path: `oa.get_processInstance_detail`
  - CLI route: `dws oa approval detail`
  - Description: 获取指定审批实例的详情信息
  - Flags: `--instance-id`
  - Schema: `skills/generated/docs/schema/oa/get_processInstance_detail.json`
- `approval records`
  - Path: `oa.get_processInstance_records`
  - CLI route: `dws oa approval records`
  - Description: 获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么
  - Flags: `--instance-id`
  - Schema: `skills/generated/docs/schema/oa/get_processInstance_records.json`
- `approval list-initiated`
  - Path: `oa.list_initiated_instances`
  - CLI route: `dws oa approval list-initiated`
  - Description: 查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息
  - Flags: `--end`, `--max-results`, `--next-token`, `--process-code`, `--start`
  - Schema: `skills/generated/docs/schema/oa/list_initiated_instances.json`
- `approval list-pending`
  - Path: `oa.list_pending_approvals`
  - CLI route: `dws oa approval list-pending`
  - Description: 查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。
  - Flags: `--end`, `--page`, `--size`, `--start`
  - Schema: `skills/generated/docs/schema/oa/list_pending_approvals.json`
- `approval tasks`
  - Path: `oa.list_pending_tasks`
  - CLI route: `dws oa approval tasks`
  - Description: 查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。
  - Flags: `--instance-id`
  - Schema: `skills/generated/docs/schema/oa/list_pending_tasks.json`
- `approval list-forms`
  - Path: `oa.list_user_visible_process`
  - CLI route: `dws oa approval list-forms`
  - Description: 获取当前用户可见的审批表单列表，可获取审批表单的processCode。
  - Flags: `--cursor`, `--size`
  - Schema: `skills/generated/docs/schema/oa/list_user_visible_process.json`
- `approval reject`
  - Path: `oa.reject_processInstance`
  - CLI route: `dws oa approval reject`
  - Description: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
  - Flags: `--instance-id`, `--remark`, `--task-id`
  - Schema: `skills/generated/docs/schema/oa/reject_processInstance.json`
- `approval revoke`
  - Path: `oa.revoke_processInstance`
  - CLI route: `dws oa approval revoke`
  - Description: 撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从
  - Flags: `--instance-id`, `--remark`
  - Schema: `skills/generated/docs/schema/oa/revoke_processInstance.json`
