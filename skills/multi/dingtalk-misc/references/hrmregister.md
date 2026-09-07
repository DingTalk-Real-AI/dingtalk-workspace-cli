# 智能人事入转调离

命令前缀：`dws hrmregister`。本 Reference 覆盖员工花名册、合同、绩效薪资、异动、调岗、转正、离职和待入职管理。

## 执行规则

- `corpId`、`opUserId` 由运行时注入，不作为命令参数。
- 结构化结果统一加 `--format json`；多个 ID 使用逗号分隔。
- 所有写操作都先展示目标与变更内容并取得明确确认；`--yes` 只用于用户已确认后的自动化执行。
- `confirm-entry`、`confirm-termination` 前必须先调用对应 `get-confirm-*-form`，仅当 `canConfirm=true` 才继续。
- 日期使用 `YYYY-MM-DD`；计划入职时间也支持 `yyyy-MM-dd HH:mm:ss`，DWS 会转换为毫秒时间戳。
- `--groups-json`、`--input-json` 支持内联 JSON、`@文件` 或 `-`（标准输入）。

## 命令索引

### 花名册、合同、绩效与薪资

| 命令 | 用途 | 关键参数 |
|---|---|---|
| `+get-roster-fields-dbg` | 查询可访问花名册字段 | 无 |
| `+get-roster-value-dbg` | 查询员工花名册当前值 | `--staff-ids`、`--field-filters` 可选 |
| `+update-employee-roster` | 更新员工花名册 | `--user-id --groups-json` |
| `+list-contract-legal-entities` | 查询合同签约主体 | 无 |
| `+query-contract-ledgers` | 查询合同台账 | `--contract-end-date-from --contract-end-date-to`；其余筛选可选 |
| `+list-employee-data-sources` | 查询绩效/薪资数据源 | `--source-type --entity-code` |
| `+query-performance-records` | 查询绩效记录 | `--user-ids --source-id` |
| `+query-employee-salary-data` | 查询薪资主数据 | `--source-id --entity-code --user-ids` |
| `+get-employee-material-files` | 查询员工材料附件 | `--staff-id --field-codes` |

### 异动、调岗与转正

| 命令 | 用途 | 关键参数 |
|---|---|---|
| `+count-employee-change-records` | 统计异动记录 | `--start-date --end-date` |
| `+query-employee-change-events` | 查询异动事件 | 日期、类型、员工、部门与分页均可选 |
| `+query-current-position` | 查询当前岗位 | `--staff-id` |
| `+query-pending-transfers` | 查询待生效调岗 | `--current --page-size` |
| `+get-transfer-form` | 获取并校验调岗表单 | `--staff-id --target-department-ids --target-main-department-id --effective-date` |
| `+create-transfer-approval` | 创建调岗审批 | `--input-json`；写操作 |
| `+cancel-transfer-approval` | 撤回或删除调岗审批 | `--process-instance-id --action WITHDRAW\|DELETE`；高风险写操作 |
| `+query-pending-regularizations` | 查询待转正员工 | `--current --page-size` |
| `+get-regularization-form` | 获取转正表单 | `--staff-id` |
| `+query-lifecycle-recipients` | 查询生命周期消息接收人 | `--business-type --employee-user-id` |

### 离职

| 命令 | 用途 | 关键参数 |
|---|---|---|
| `+query-termination-employees` | 查询离职员工 | `--last-work-start-date --last-work-end-date` |
| `+query-pending-terminations` | 查询待离职记录 | `--current --page-size` |
| `+get-termination-by-id` | 查询离职详情 | `--termination-id` |
| `+get-employee-termination-reason` | 查询员工离职原因 | `--user-id` |
| `+get-confirm-termination-form` | 获取确认离职表单 | `--staff-id` |
| `+confirm-termination` | 确认员工离职 | `--staff-id`；高风险写操作 |
| `+update-termination-information` | 更新离职信息 | `--staff-id --termination-id --confirmation-token`，并至少修改一个业务字段 |
| `+revoke-termination-process` | 撤销离职流程 | `--staff-id --termination-id --revoke-reason --confirmation-token`；高风险写操作 |

### 待入职与入职

| 命令 | 用途 | 关键参数 |
|---|---|---|
| `+query-pre-entry-employees` | 分页查询待入职员工 | 姓名、手机号、计划入职时间和分页均可选 |
| `+query-pre-entry-by-name-mobile` | 按姓名或手机号查待入职员工 | 两者至少提供一个 |
| `+get-confirm-entry-form` | 获取确认入职表单 | `--staff-id` |
| `+confirm-entry` | 确认员工入职 | `--staff-id`；高风险写操作 |
| `+invite-perfect-info` | 邀请员工完善资料 | `--staff-id`；写操作 |
| `+add-pre-entry-employee` | 新增待入职员工 | `--pre-entry-time --name --mobile`；组织和岗位信息可选 |

参数不确定时只查询目标命令：

```bash
dws schema --cli-path "hrmregister +query-pending-transfers" --compact --format json
dws hrmregister +query-pending-transfers --help
```
