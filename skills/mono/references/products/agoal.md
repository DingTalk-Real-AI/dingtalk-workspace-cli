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

`strategy update` 是覆盖式更新：一定要先 `strategy detail` 获取完整数据，在原数据基础上修改后再传入。

### contract (经营合约管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `contract list` | 获取经营合约列表 | `--scope-type` `--scope-id` | scopeType: DEPT/PERSONAL；scope-id 为 scope-type 对应的部门 id 或用户 id |
| `contract fields` | 获取经营合约字段列表 | - | 获取组织下经营合约的字段配置 |
| `contract detail` | 获取经营合约详情 | `--contract-id` | 根据合约 id 查询 |
| `contract update` | 更新经营合约 | `--contract-id` `--dimensions` | 覆盖逻辑；可选 `--audit-config`、`--objective-template` |

`contract update` 同样是覆盖式更新：必须基于 `contract detail` 返回的数据修改后再传入。

### scorecard (计分卡管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `scorecard detail` | 获取计分卡详情 | `--selected-time` `--dept-id` | selectedTime 为 ISO-8601 字符串，如 `"2026-01-01T00:00:00+08:00"` |
| `scorecard entity-detail` | 获取计分卡实体详情 | `--sc-id` `--entity-id` | 根据计分卡 id 和实体 id 查询 |
| `scorecard update` | 更新计分卡 | `--dept-id` `--selected-time` `--id` `--tracking-period-type` `--content` | trackingPeriodType: MONTHLY/QUARTERLY |

### user (用户目标管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `user rules` | 获取用户规则周期列表 | - | 可选 `--user-id`，不传则默认取操作人自己 |
| `user objectives` | 查询用户目标列表 | `--user-id` `--rule-id` `--period-ids` | `--period-ids` 为逗号分隔的周期 id 列表 |

### report (周月报管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `report list-statistics` | 获取周月报数据跟催列表 | - | 返回各规则的人员提交情况统计；可选 `--keyword` |
| `report submit-detail` | 获取周月报规则提交详情 | `--template-id` `--submit-state` | submitState: ON_TIME/LATE/NOT_SUBMITTED；可选 `--query-date`、`--page`、`--page-size`、`--keyword` |

### obj-template (目标模板管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `obj-template list` | 获取目标模板列表 | - | 可选 `--keyword`、`--page`、`--page-size` |
| `obj-template create-or-update` | 新增或更新目标模板 | `--dimensions` | 覆盖逻辑；新增时 `--title` 必填；更新时 `--template-id` 必填 |

## 意图判断

用户说"战略解码/战略目标/OGSM":
- 查看/列表 → `strategy list`
- 详情 → `strategy detail`
- 修改/更新 → `strategy update`（先查后改）

用户说"经营合约/合约/KPI合约":
- 查看/列表 → `contract list`
- 字段配置 → `contract fields`
- 详情 → `contract detail`
- 修改/更新 → `contract update`（先查后改）

用户说"计分卡/scorecard/绩效看板":
- 查看详情 → `scorecard detail`
- 实体详情 → `scorecard entity-detail`
- 修改/更新 → `scorecard update`

用户说"目标/OKR/我的目标/个人目标":
- 规则周期 → `user rules`
- 目标列表 → `user objectives`

用户说"目标模板/模板管理":
- 查看模板列表 → `obj-template list`
- 新增模板 → `obj-template create-or-update --title "模板名称"`
- 更新模板 → `obj-template create-or-update --template-id TPL_ID`

用户说"周月报/周报统计/提交情况/跟催/迟交/未提交":
- 查看提交统计列表 → `report list-statistics`
- 查看某规则的提交详情 → `report submit-detail`

## 核心工作流

```bash
# 查看战略解码列表与详情
dws agoal strategy list --scope-type DEPT --scope-id DEPT_ID --format json
dws agoal strategy detail --profile-id PROFILE_ID --format json

# 更新战略解码：必须基于 detail 返回内容修改后传入
dws agoal strategy update --profile-id PROFILE_ID \
  --content '[{"id":"entity1","title":{"title":"新目标"},"entityType":"OGSM_OBJECTIVE","status":"NORMAL","executors":["dingId1"]}]' \
  --format json

# 查看经营合约列表、字段与详情
dws agoal contract list --scope-type PERSONAL --scope-id USER_ID --format json
dws agoal contract fields --format json
dws agoal contract detail --contract-id CONTRACT_ID --format json

# 更新经营合约：必须基于 detail 返回内容修改后传入
dws agoal contract update --contract-id CONTRACT_ID \
  --dimensions '[{"id":"dim1","title":"维度名称","objectives":[]}]' \
  --format json

# 查看计分卡
dws agoal scorecard detail --selected-time "2026-01-01T00:00:00+08:00" --dept-id DEPT_ID --format json
dws agoal scorecard entity-detail --sc-id SC_ID --entity-id ENTITY_ID --format json

# 更新计分卡
dws agoal scorecard update --dept-id DEPT_ID --selected-time "2026-01-01T00:00:00+08:00" \
  --id SC_ID --tracking-period-type MONTHLY \
  --content '[{"id":"dim1","title":"业绩","items":[{"id":"item1","title":"收入","target":"100"}]}]' \
  --format json

# 查询用户目标
dws agoal user rules --user-id USER_ID --format json
dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2" --format json

# 周月报提交统计与详情
dws agoal report list-statistics --format json
dws agoal report list-statistics --keyword "周报规则" --format json
dws agoal report submit-detail --template-id TPL_ID --submit-state ON_TIME --format json
dws agoal report submit-detail --template-id TPL_ID --submit-state LATE --query-date "2026-06-18T00:00:00+08:00" --page 1 --page-size 20 --format json

# 目标模板
dws agoal obj-template list --format json
dws agoal obj-template list --keyword "业绩" --format json
dws agoal obj-template create-or-update --title "业绩模板" --objective-weight --dimension-weight --dimensions '[...]' --format json
dws agoal obj-template create-or-update --template-id TPL_ID --title "业绩模板" --dimensions '[...]' --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `strategy list` | `profileId` | `strategy detail` / `strategy update` 的 `--profile-id` |
| `strategy detail` | 完整实体数据 | `strategy update` 的 `--content`（基于此修改） |
| `contract list` | `contractId` | `contract detail` / `contract update` 的 `--contract-id` |
| `contract detail` | 完整维度数据 | `contract update` 的 `--dimensions`（基于此修改） |
| `scorecard detail` | `scId`、`entityId` | `scorecard entity-detail` / `scorecard update` 的 `--id` |
| `user rules` | `ruleId`、`periodIds` | `user objectives` 的 `--rule-id` `--period-ids` |
| `report list-statistics` | `templateId` | `report submit-detail` 的 `--template-id` |
| `obj-template list` | `templateId` | `obj-template create-or-update` 的 `--template-id`（更新时） |

## 注意事项

- 所有 update / create-or-update 命令都是覆盖逻辑：必须先用对应 detail/list 查询完整数据，在原数据基础上修改后再传入。
- 所有命令支持可选参数 `--request-id`。
- `--scope-type` 仅支持 `DEPT` 和 `PERSONAL`。
- `--selected-time` 接受 ISO-8601 字符串，如 `"2026-01-01T00:00:00+08:00"`。
- `--period-ids` 为逗号分隔字符串，如 `"period1,period2"`。
- `report submit-detail` 的 `--query-date` 接受 ISO-8601 字符串；不传则默认当天。
