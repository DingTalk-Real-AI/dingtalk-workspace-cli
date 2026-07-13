# Agoal（目标管理）

## 产品说明

Agoal 是钉钉目标管理工具，支持战略解码、经营合约、计分卡、用户目标、目标模板、周月报六大模块，帮助组织将战略目标从顶层分解到个人并持续跟踪。

**CLI 前缀**: `dws agoal`

## 命令总览

### strategy (战略解码管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `strategy list` | 获取战略解码列表 | `--scope-type` `--scope-id` | scopeType: DEPT/PERSONAL；scope-id 为 scope-type 对应的部门 id 或用户 id |
| `strategy detail` | 获取战略解码详情 | `--profile-id` | 根据战略解码 id 查询 |
| `strategy update` | 更新战略解码 | `--profile-id` `--content` | 覆盖逻辑，必须基于查询返回的老数据修改后再传入；`--content` 为 JSON 数组 |

### contract (经营合约管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `contract list` | 获取经营合约列表 | `--scope-type` `--scope-id` | scopeType: DEPT/PERSONAL；scope-id 为 scope-type 对应的部门 id 或用户 id |
| `contract fields` | 获取经营合约字段列表 | - | 获取组织下经营合约的字段配置 |
| `contract detail` | 获取经营合约详情 | `--contract-id` | 根据合约 id 查询 |
| `contract update` | 更新经营合约 | `--contract-id` `--dimensions` | 覆盖逻辑；可选 `--audit-config`、`--objective-template` |

### scorecard (计分卡管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `scorecard detail` | 获取计分卡详情 | `--selected-time` `--dept-id` | selectedTime 为 ISO-8601 字符串 |
| `scorecard entity-detail` | 获取计分卡实体详情 | `--sc-id` `--entity-id` | 根据计分卡 id 和实体 id 查询 |
| `scorecard update` | 更新计分卡 | `--dept-id` `--selected-time` `--id` `--tracking-period-type` `--content` | trackingPeriodType: MONTHLY/QUARTERLY |

### user / report / obj-template

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `user rules` | 获取用户规则周期列表 | - |
| `user objectives` | 查询用户目标列表 | `--user-id` `--rule-id` `--period-ids` |
| `report list-statistics` | 获取周月报数据跟催列表 | - |
| `report submit-detail` | 获取周月报规则提交详情 | `--template-id` `--submit-state` |
| `obj-template list` | 获取目标模板列表 | - |
| `obj-template create-or-update` | 新增或更新目标模板 | `--dimensions` |

## 意图判断

- "战略解码/战略目标/OGSM" → `strategy list/detail/update`
- "经营合约/合约/KPI合约" → `contract list/fields/detail/update`
- "计分卡/scorecard/绩效看板" → `scorecard detail/entity-detail/update`
- "目标/OKR/我的目标/个人目标" → `user rules/objectives`
- "目标模板/模板管理" → `obj-template list/create-or-update`
- "周月报/周报统计/提交情况/跟催/迟交/未提交" → `report list-statistics/submit-detail`

## 常用命令

```bash
dws agoal strategy list --scope-type DEPT --scope-id DEPT_ID --format json
dws agoal strategy detail --profile-id PROFILE_ID --format json
dws agoal contract list --scope-type PERSONAL --scope-id USER_ID --format json
dws agoal contract fields --format json
dws agoal contract detail --contract-id CONTRACT_ID --format json
dws agoal scorecard detail --selected-time "2026-01-01T00:00:00+08:00" --dept-id DEPT_ID --format json
dws agoal user rules --user-id USER_ID --format json
dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2" --format json
dws agoal report list-statistics --format json
dws agoal report submit-detail --template-id TPL_ID --submit-state ON_TIME --format json
dws agoal obj-template list --keyword "业绩" --format json
```

## 安全规则

- 所有 update / create-or-update 命令都是覆盖逻辑：必须先用对应 detail/list 查询完整数据，在原数据基础上修改后再传入。
- 写入类命令执行前必须展示变更摘要并等待用户确认。
- `--selected-time` / `--query-date` 接受 ISO-8601 字符串。
- `--period-ids` 为逗号分隔字符串。
