# 智能人事入转调离

命令前缀：`dws hrmregister`。用于员工花名册、合同、绩效薪资、异动、调岗、转正、离职和待入职管理。

## 最小执行规则

- `corpId`、`opUserId` 由运行时注入，不向用户索要。
- 多个 ID 用逗号分隔；日期用 `YYYY-MM-DD`；结构化结果加 `--format json`。
- 写操作先展示目标和变更并取得明确确认。确认入职/离职前先调用对应表单查询，只有 `canConfirm=true` 才继续。
- JSON 参数支持内联、`@文件` 或 `-` 标准输入。

## 命令分组

- 花名册：`+get-roster-fields-dbg`、`+get-roster-value-dbg`、`+update-employee-roster`
- 合同/绩效/薪资：`+list-contract-legal-entities`、`+query-contract-ledgers`、`+list-employee-data-sources`、`+query-performance-records`、`+query-employee-salary-data`
- 异动/材料：`+count-employee-change-records`、`+query-employee-change-events`、`+get-employee-material-files`、`+query-lifecycle-recipients`
- 调岗：`+query-current-position`、`+query-pending-transfers`、`+get-transfer-form`、`+create-transfer-approval`、`+cancel-transfer-approval`
- 转正：`+query-pending-regularizations`、`+get-regularization-form`
- 离职：`+query-termination-employees`、`+query-pending-terminations`、`+get-termination-by-id`、`+get-employee-termination-reason`、`+get-confirm-termination-form`、`+confirm-termination`、`+update-termination-information`、`+revoke-termination-process`
- 入职：`+query-pre-entry-employees`、`+query-pre-entry-by-name-mobile`、`+get-confirm-entry-form`、`+confirm-entry`、`+invite-perfect-info`、`+add-pre-entry-employee`

使用 `dws hrmregister <命令> --help` 查看参数；只在参数契约不确定时读取精确 leaf Schema。
