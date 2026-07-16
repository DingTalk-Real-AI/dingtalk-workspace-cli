# Shortcut 真实测试：后端 / MCP 问题整理

这份报告只汇总 `failure_category = backend-or-mcp-error` 的 case，已尽量排除权限、缺真实资源、当前账号无数据等噪音。

## 总览

- Backend/MCP case 总数：33
- 聚合问题数：8
- 复现口径：真实 dws CLI；无 mock；无 dry-run；命令输入和 trace_id 均来自真实测试结果。

## 建议优先看

1. [P1] Chat/IM 会话 ID 字段在 MCP/后端映射中疑似丢失（15 case）
2. [P1] Chat card 发送 receiverUid 疑似未从 receiver 透传（1 case）
3. [P1] Chat 入群审批 applicantUid/inviterUid 疑似未透传（1 case）
4. [P1] AI 表格 MCP 错误 envelope 语义不一致：success=true 但 error 非空/status=error（5 case）
5. [P1] AI 表格 Workflow 查询在真实 Base 下返回系统级错误（2 case）
6. [P1] AI 表格 roleId 参数疑似未被 MCP 正确读取（3 case）
7. [P2] AI 表格记录主文档查询在真实 record 下返回 no record/SYSTEM_ERROR（2 case）
8. [P2] AI 表格无效 Base/Table/Field/Record 被包装成 SYSTEM_ERROR（4 case）

## Chat/IM 会话 ID 字段在 MCP/后端映射中疑似丢失

- 优先级：P1
- 建议 owner：IM MCP / IM 后端字段映射
- 现象：CLI 已传 group/conversation-id/open-conversation-id（部分 case 使用真实 cid），后端仍报 openCid/openConversationId/cid required。
- 期望：MCP schema/网关应接受并透传 openConversationId/openCid/cid 中的兼容字段；如果资源无效，应返回“无效会话”，而不是 required。
- 涉及 case：15

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `read` | `chat +chat-members-get` | `tools/call` | `2127d89817840997754345760e07bd` | [UNCLASSIFIED] openCid or cid is required (operation: im/list_group_member_by_ids) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +chat-members-get --id DWSREALREADNOSUCHID0000000000000 --users '冬翔' --yes --format json` |
| `read` | `chat +chat-messages` | `tools/call` | `2104a64c17840997767792656e085e` | [UNCLASSIFIED] openCid or cid is required hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +chat-messages --group cid3Jijzhe2aqs9ysOXjhi05g== --time '2026-07-15 10:00:00' --limit 10 --direction older --yes --format json` |
| `read` | `chat +messages-list` | `tools/call` | `2127d89817840997797873841e0757` | [UNCLASSIFIED] openCid or cid is required hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +messages-list --group cid3Jijzhe2aqs9ysOXjhi05g== --time '2026-07-15 10:00:00' --forward --limit 10 --yes --format json` |
| `write` | `chat +chat-mute-member` | `tools/call` | `2104a64c17840999166036583e08a3` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/set_group_member_mute_list) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +chat-mute-member --group cidDWSREALTESTNOSUCHCONV --users __DWS_SHORTCUT_REAL_TEST_NO_SUCH_USER__ --mute-time 1 --off --yes --format json` |
| `write` | `chat +chat-transfer-owner` | `tools/call` | `0b5deb3217840999222318863e087a` | [UNCLASSIFIED] openConversationId is required (operation: im/transfer_group_owner) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +chat-transfer-owner --group cidDWSREALTESTNOSUCHCONV --new-owner __DWS_SHORTCUT_REAL_TEST_NO_SUCH_USER__ --yes --format json` |
| `write` | `chat +conversation-clear-messages` | `tools/call` | `2127d89817840999254816721e079b` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/clear_conversation_messages) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-clear-messages --conversation-id cidDWSREALTESTNOSUCHCONV --yes --format json` |
| `write` | `chat +conversation-clear-red-point` | `tools/call` | `2104a64c17840999265511295e085f` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/clear_conversation_red_point) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-clear-red-point --conversation-id cidDWSREALTESTNOSUCHCONV --yes --format json` |
| `write` | `chat +conversation-hide` | `tools/call` | `2127d89817840999276117140e079b` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/hide_conversation) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-hide --conversation-id cidDWSREALTESTNOSUCHCONV --yes --format json` |
| `write` | `chat +conversation-mark-unread` | `tools/call` | `0bb7c36217840999298744910e0758` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/mark_conversation_unread) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-mark-unread --conversation-id cidDWSREALTESTNOSUCHCONV --yes --format json` |
| `write` | `chat +conversation-mute` | `tools/call` | `2104a64c17840999309241865e085f` | [UNCLASSIFIED] openConversationId is required (operation: im/update_notification_off) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-mute --conversation-id cidDWSREALTESTNOSUCHCONV --off --yes --format json` |
| `write` | `chat +conversation-mute-at-all` | `tools/call` | `2127d89817840999320028068e07dd` | [UNCLASSIFIED] openConversationId is required (operation: im/update_at_all_notification_off) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-mute-at-all --conversation-id cidDWSREALTESTNOSUCHCONV --off --yes --format json` |
| `write` | `chat +conversation-mute-red-envelope` | `tools/call` | `0bb7c36217840999330228283e07fe` | [UNCLASSIFIED] openConversationId is required (operation: im/update_red_env_notification_off) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-mute-red-envelope --conversation-id cidDWSREALTESTNOSUCHCONV --off --yes --format json` |
| `write` | `chat +conversation-set-top` | `tools/call` | `2127d89817840999341302961e07fe` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/set_top_conversation) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +conversation-set-top --conversation-id cidDWSREALTESTNOSUCHCONV --off --yes --format json` |
| `write` | `chat +messages-set-pin` | `tools/call` | `0bb7c36217840999521117991e0758` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/set_pin_message) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +messages-set-pin --open-conversation-id cidDWSREALTESTNOSUCHCONV --msg-id DWSREALTESTNOSUCHID0000000000000 --yes --format json` |
| `write` | `chat +messages-unset-pin` | `tools/call` | `2104a64c17840999544428103e08ee` | [UNCLASSIFIED] openConversationId or cid is required (operation: im/unset_pin_message) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +messages-unset-pin --open-conversation-id cidDWSREALTESTNOSUCHCONV --msg-id DWSREALTESTNOSUCHID0000000000000 --yes --format json` |

## Chat card 发送 receiverUid 疑似未从 receiver 透传

- 优先级：P1
- 建议 owner：IM MCP / card 发送参数映射
- 现象：CLI 传入 receiver=103262，后端仍报 receiverUid 和 openConversationId 不能同时为空。
- 期望：receiver 应映射为 receiverUid，或 schema 明确要求 receiverUid；真实入参不应在 MCP 层丢失。
- 涉及 case：1

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `write` | `chat +messages-send-card` | `tools/call` | `2104a64c17840999509255753e081a` | [UNCLASSIFIED] receiverUid和openConversationId不能同时为空 (operation: im/create_and_send_card) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +messages-send-card --receiver 103262 --yes --format json` |

## Chat 入群审批 applicantUid/inviterUid 疑似未透传

- 优先级：P1
- 建议 owner：IM MCP / 入群审批参数映射
- 现象：CLI 传入 applicant=103262、inviter=519019，后端仍报 applicantUid required。
- 期望：applicant/inviter 应映射为 applicantUid/inviterUid；如果 recordId/group 无效，应返回对应资源错误而不是 applicantUid 缺失。
- 涉及 case：1

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `write` | `chat +chat-audit-join` | `tools/call` | `0bb7c36217840999154832733e0758` | [UNCLASSIFIED] applicantUid is required (operation: im/audit_join_group) hint: Use --verbose for detailed error logs |
| | input | | | `/private/tmp/dws-real-test chat +chat-audit-join --group cidDWSREALTESTNOSUCHCONV --record-id 999999999999 --applicant 103262 --inviter 519019 --status AuditApprove --description 'DWS shortcut 真实测试描述，可删除' --yes --format json` |

## AI 表格 MCP 错误 envelope 语义不一致：success=true 但 error 非空/status=error

- 优先级：P1
- 建议 owner：AI 表格 MCP wrapper
- 现象：多条 AI 表格命令返回 MCP_TOOL_ERROR，内部 JSON 同时出现 success=true、status=error、error 非空。
- 期望：只要 error 非空或 status=error，success 应为 false，外层也应按业务错误返回稳定错误码/trace。
- 涉及 case：5

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `read` | `aitable +export-data` | `-` | `2104a64c17840997514714448e0817` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"taskId cannot be combined with scope, format, tableId or viewId","retryable":false,"type":"INPUT_ERROR"},"m… |
| | input | | | `/private/tmp/dws-real-test aitable +export-data --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --task-id DWSREALREADNOSUCHID0000000000000 --scope all --format excel --table-id hERWDMS --view-id qvGDAH2 --timeout-ms 1 --yes` |
| `write` | `aitable +chart-update` | `-` | `2106d98117840998553244877e08df` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"config is required","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success":true,"summ… |
| | input | | | `/private/tmp/dws-real-test aitable +chart-update --base-id DWSREALTESTNOSUCHID0000000000000 --dashboard-id DWSREALTESTNOSUCHID0000000000000 --chart-id DWSREALTESTNOSUCHID0000000000000 --config '{}' --layout '{}' --yes --format json` |
| `write` | `aitable +record-update` | `-` | `0bab027317840998747383236e090b` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_RECORDS","message":"records must contain at least one writable record","retryable":false,"type":"INPUT_ERROR"},"meta":{},"stat… |
| | input | | | `/private/tmp/dws-real-test aitable +record-update --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --records '[{"recordId":"recDWSREALTEST","cells":{}}]' --yes --format json` |
| `write` | `aitable +record-upsert` | `-` | `2106d98117840998759832182e087b` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMETER","message":"records is required and must not be empty","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"e… |
| | input | | | `/private/tmp/dws-real-test aitable +record-upsert --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --records '[]' --yes --format json` |
| `write` | `aitable +view-set-fill-color-rule` | `-` | `2106d98117840998924181709e08df` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"conditionalFormats is required","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success… |
| | input | | | `/private/tmp/dws-real-test aitable +view-set-fill-color-rule --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --view-id DWSREALTESTNOSUCHID0000000000000 --json '{}' --yes --format json` |

## AI 表格 Workflow 查询在真实 Base 下返回系统级错误

- 优先级：P1
- 建议 owner：AI 表格 Workflow MCP / 后端
- 现象：使用真实可访问 Base 查询 workflow list/get，返回 LIST_WORKFLOWS_ERROR/GET_WORKFLOW_ERROR。
- 期望：无 workflow 时应返回空列表或 WORKFLOW_NOT_FOUND；有后端异常时需提供稳定错误码和可排查 trace。
- 涉及 case：2

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `read` | `aitable +workflow-get` | `-` | `2104a64c17840997556363676e08ee` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"GET_WORKFLOW_ERROR","message":"调用远程服务业务异常","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary"… |
| | input | | | `/private/tmp/dws-real-test aitable +workflow-get --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --workflow-id DWSREALREADNOSUCHID0000000000000 --yes --format json` |
| `read` | `aitable +workflow-list` | `-` | `2127d89817840997572427879e075d` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"LIST_WORKFLOWS_ERROR","message":"biz error","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary… |
| | input | | | `/private/tmp/dws-real-test aitable +workflow-list --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --limit 10 --offset 1 --yes --format json` |

## AI 表格 roleId 参数疑似未被 MCP 正确读取

- 优先级：P1
- 建议 owner：AI 表格 MCP role 接口
- 现象：CLI 已传 --role-id，但 MCP 返回 roleId is required。
- 期望：role-id/roleId 字段应被正确映射；如果 role 不存在，返回 ROLE_NOT_FOUND，而不是 required。
- 涉及 case：3

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `read` | `aitable +role-get` | `-` | `2104a64c17840997542904838e0817` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"roleId is required","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success":true,"summ… |
| | input | | | `/private/tmp/dws-real-test aitable +role-get --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --role-id x --yes --format json` |
| `write` | `aitable +role-delete` | `-` | `2106d98117840998782482701e089c` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"roleId is required","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success":true,"summ… |
| | input | | | `/private/tmp/dws-real-test aitable +role-delete --base-id DWSREALTESTNOSUCHID0000000000000 --role-id DWSREALTESTNOSUCHID0000000000000 --yes --format json` |
| `write` | `aitable +role-update` | `-` | `2132f5ca17840998794483634e08d8` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"INVALID_PARAMS","message":"roleId is required","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success":true,"summ… |
| | input | | | `/private/tmp/dws-real-test aitable +role-update --base-id DWSREALTESTNOSUCHID0000000000000 --role-id DWSREALTESTNOSUCHID0000000000000 --name 'DWS shortcut 真实测试 20260715-151724' --role-type x --flow-type x --sub-roles '[]' --yes --format json` |

## AI 表格记录主文档查询在真实 record 下返回 no record/SYSTEM_ERROR

- 优先级：P2
- 建议 owner：AI 表格 primary doc MCP / 后端
- 现象：record-query 已能查到真实 recordId，但 primary-doc 查询返回 no record、type=SYSTEM_ERROR。
- 期望：若该记录无主文档，应返回空/未创建；若 recordId 语义不匹配，应返回明确参数错误，不应是系统错误。
- 涉及 case：2

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `read` | `aitable +base-get-primary-doc-id` | `-` | `0b5deb3217840997466255627e08ee` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"-1","message":"no record","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary":"Failed to query… |
| | input | | | `/private/tmp/dws-real-test aitable +base-get-primary-doc-id --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --table-id hERWDMS --record-id 1015oH3OXy --yes --format json` |
| `read` | `aitable +record-primary-doc-get` | `-` | `2127d89817840997528393730e079c` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"-1","message":"no record","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary":"Failed to query… |
| | input | | | `/private/tmp/dws-real-test aitable +record-primary-doc-get --base-id gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk --table-id hERWDMS --record-id 1015oH3OXy --yes --format json` |

## AI 表格无效 Base/Table/Field/Record 被包装成 SYSTEM_ERROR

- 优先级：P2
- 建议 owner：AI 表格 MCP wrapper / 后端错误码
- 现象：安全负向 ID 下，部分写接口返回 getDentryDTO returns null、type=SYSTEM_ERROR、retryable=true。
- 期望：资源不存在应返回 INPUT_ERROR/NOT_FOUND 且 retryable=false，避免误导调用方重试。
- 涉及 case：4

| 套件 | shortcut | operation | trace_id | 证据 |
|---|---|---|---|---|
| `write` | `aitable +field-delete` | `-` | `0bab027317840998611338768e08c8` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"404","message":"getDentryDTO returns null","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary"… |
| | input | | | `/private/tmp/dws-real-test aitable +field-delete --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --field-id DWSREALTESTNOSUCHID0000000000000 --yes --format json` |
| `write` | `aitable +field-update` | `-` | `213ee25c17840998623207342e08e2` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"404","message":"getDentryDTO returns null","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary"… |
| | input | | | `/private/tmp/dws-real-test aitable +field-update --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --field-id DWSREALTESTNOSUCHID0000000000000 --name 'DWS shortcut 真实测试 20260715-151724' --config '{}' --ai-config '{}' --yes --format json` |
| `write` | `aitable +record-delete` | `-` | `2132f5ca17840998724753149e0853` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"404","message":"getDentryDTO returns null","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary"… |
| | input | | | `/private/tmp/dws-real-test aitable +record-delete --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --record-ids DWSREALTESTNOSUCHID0000000000000 --yes --format json` |
| `write` | `aitable +table-update` | `-` | `213ee25c17840998877246229e087f` | [MCP_TOOL_ERROR] {"data":{},"error":{"code":"404","message":"getDentryDTO returns null","retryable":true,"type":"SYSTEM_ERROR"},"meta":{},"status":"error","success":true,"summary"… |
| | input | | | `/private/tmp/dws-real-test aitable +table-update --base-id DWSREALTESTNOSUCHID0000000000000 --table-id DWSREALTESTNOSUCHID0000000000000 --name 'DWS shortcut 真实测试 20260715-151724' --description 'DWS shortcut 真实测试描述，可删除' --record-name-key task --yes --format json` |
