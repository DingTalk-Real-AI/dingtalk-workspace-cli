# Agoal（目标管理）

## 产品说明

Agoal 是钉钉目标管理工具，支持战略解码、经营合约、计分卡、用户目标、目标模板、周月报六大模块，帮助组织将战略目标从顶层分解到个人并持续跟踪。

**CLI 前缀**: `dws agoal`

## 产品边界与执行优先级

- 日报、周报、月报正文的填写、提交和收发记录走 Report；规则级按时、迟交、未提交统计、人员清单和跟催走 Agoal。
- 所有结构化查询都带 `--format json`；后续 ID 必须来自前一步真实返回。
- 意图能够由公开 Shortcut 满足时优先使用：`+contract-fields`、`+user-rules`、`+report-statistics-list`、`+report-submit-detail`、`+obj-template-list`。
- `+strategy-*`、`+contract-list/get/update`、`+scorecard-*`、`+user-objectives` 和 `+obj-template-upsert` 当前明确 unavailable，不要尝试。这只约束带 `+` 的 Shortcut，不代表当前叶子 Help 存在的同名原子命令不可用。

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
| `+contract-fields` | 获取经营合约字段列表 | - | 稳定校验字段数组和字段 id，支持本地关键词筛选 |
| `contract detail` | 获取经营合约详情 | `--contract-id` | 根据合约 id 查询 |
| `contract update` | 更新经营合约 | `--contract-id` `--dimensions` | 覆盖逻辑；可选 `--audit-config`、`--objective-template` |

`contract update` 同样是覆盖式更新：必须基于 `contract detail` 返回的数据修改后再传入。

### scorecard (计分卡管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `scorecard detail` | 获取计分卡详情 | `--selected-time` `--dept-id` | selectedTime 为 ISO-8601 字符串，如 `"2026-01-01T00:00:00+08:00"` |
| `scorecard entity-detail` | 获取计分卡实体详情 | `--sc-id` `--entity-id` | 根据计分卡 id 和实体 id 查询 |
| `scorecard update` | 更新计分卡 | `--dept-id` `--selected-time` `--id` `--tracking-period-type` `--content` | trackingPeriodType: MONTHLY/QUARTERLY |
| `scorecard search-entities` | 搜索计分卡指标与关键事项 | `--keyword` | 可选 `--page`、`--page-size`；keyword 为标题模糊匹配关键词 |

### user (用户目标管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `+user-rules` | 获取用户规则周期列表 | - | 可选 `--user-id`，不传则默认取操作人自己；默认偏好需用原子 `user rules` |
| `user objectives` | 查询用户目标列表 | `--user-id` `--rule-id` `--period-ids` | `--period-ids` 为逗号分隔的周期 id 列表 |

### report (周月报管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `+report-statistics-list` | 获取周月报数据跟催列表 | - | 返回各规则的人员提交情况统计；可选 `--keyword` |
| `+report-submit-detail` | 获取周月报规则提交详情 | `--template-id` `--submit-state` | submitState: ON_TIME/LATE/NOT_SUBMITTED；可选 `--query-date`、`--page`、`--page-size`、`--keyword` |

### obj-template (目标模板管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `+obj-template-list` | 获取目标模板列表 | - | 稳定校验模板 id 和分页事实；可选 `--keyword`、`--page`、`--page-size` |
| `obj-template create-or-update` | 新增或更新目标模板 | `--dimensions` | 覆盖逻辑；新增时 `--title` 必填；更新时 `--template-id` 必填 |

## 意图判断

用户说"战略解码/战略目标/OGSM":
- 查看/列表 → `strategy list`
- 详情 → `strategy detail`
- 修改/更新 → `strategy update`（先查后改）

用户说"经营合约/合约/KPI合约":
- 查看/列表 → `contract list`
- 字段配置 → `+contract-fields`
- 详情 → `contract detail`
- 修改/更新 → `contract update`（先查后改）

用户说"计分卡/scorecard/绩效看板":
- 查看详情 → `scorecard detail`
- 实体详情 → `scorecard entity-detail`
- 修改/更新 → `scorecard update`
- 搜索计分卡指标与关键事项 → `scorecard search-entities --keyword "关键词"`

用户说"目标/OKR/我的目标/个人目标":
- 规则周期 → `+user-rules`；问默认偏好时用原子 `user rules`
- 目标列表 → `user objectives`

用户说"目标模板/模板管理":
- 查看模板列表 → `+obj-template-list`
- 新增模板 → `obj-template create-or-update --title "模板名称"`
- 更新模板 → `obj-template create-or-update --template-id TPL_ID`

用户说"周月报/周报统计/提交情况/跟催/迟交/未提交":
- 查看提交统计列表 → `+report-statistics-list`
- 查看某规则的提交详情 → `+report-submit-detail`

## 高频只读 SOP

### 本人与主部门

切换到 Contact skill，执行 `dws contact user get-self --format json`，从真实返回提取 `orgEmployeeModel.userId` 和主部门 ID。禁止把 `me`、`self`、空字符串、用户名或 Profile 当作 `--scope-id` / `--user-id`；多部门且没有唯一主部门标记时先让用户选择。个人和部门范围分别查询，`success=true` 且 `content=[]` 就如实报告该精确范围为空，不换其它范围补数。

### 经营合约字段

使用 `dws agoal +contract-fields --format json`。`required` 决定必填/可选，`active` 表示启用；`forceActive` 和 `forceRequired` 只在真实为 `true` 时标为强制，不从其它字段推断。若普通标记与强制标记冲突，保留原值并标记服务端字段冲突，不自行改组或选择优先级。

### 默认规则、周期与目标

`+user-rules` 只投影规则数组；用户问默认/偏好时执行 `dws agoal user rules --format json`，从 `content.preference.ruleId` 和 `content.preference.periodId` 精确定位。比较前一期时，用 `nameEN` / `nameCn` 的 `Q1`～`Q4`、`S1`～`S2`、`Annual` / `年度` 识别相同粒度，再选 `endDate < 当前期 startDate` 且 `endDate` 最大的一项；没有唯一候选时让用户选择。每期单独执行一次 `user objectives` 以保留空周期。本人目标所需 userId 来自 `contact user get-self`，不传 `me`。秒或毫秒时间戳统一转换为 `Asia/Shanghai`，不根据 FY 名称猜日期。

### 周月报统计与明细

`enableStatistic=false` 时，列表里的零值不能证明无人提交。使用 `--keyword` 后仍按真实返回规则名核对匹配结果。需要选出人数最多的规则时，对所有候选用同一个显式 `--query-date` 查询目标状态的明细 `totalCount`；选中后再分别查 `ON_TIME`、`LATE`、`NOT_SUBMITTED`，把统计值和明细总数并排展示并标记差异。“今天”按当前会话时区确定一次日期，整项任务保持不变。人员详情只返回用户要求的最小字段。

### 模板分页与去重

用户明确要求前两页时必须真实查询两页。逐页报告 `count`，用服务端 `totalCount` 表示系统总数；合并时优先用稳定 `id`，缺失时用 `templateId` 去重，标题相同但 ID 不同仍是不同模板。两者都缺失时停止，不回退为按标题去重。

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
dws agoal +contract-fields --format json
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

# 搜索计分卡指标与关键事项
dws agoal scorecard search-entities --keyword "业绩" --format json
dws agoal scorecard search-entities --keyword "业绩" --page 1 --page-size 20 --format json

# 查询用户目标
dws agoal +user-rules --user-id USER_ID --format json
dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2" --format json

# 周月报提交统计与详情
dws agoal +report-statistics-list --format json
dws agoal +report-statistics-list --keyword "周报规则" --format json
dws agoal +report-submit-detail --template-id TPL_ID --submit-state ON_TIME --format json
dws agoal +report-submit-detail --template-id TPL_ID --submit-state LATE --query-date "2026-06-18T00:00:00+08:00" --page 1 --page-size 20 --format json

# 目标模板
dws agoal +obj-template-list --page 1 --page-size 20 --format json
dws agoal +obj-template-list --keyword "业绩" --page 1 --page-size 20 --format json
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
| `+user-rules` / 原子 `user rules` | `ruleId`、`periodIds`、原子返回中的 `preference` | `user objectives` 的 `--rule-id` `--period-ids` |
| `+report-statistics-list` | `templateId` | `+report-submit-detail` 的 `--template-id` |
| `+obj-template-list` | `id` / `templateId` | `obj-template create-or-update` 的 `--template-id`（更新时） |

## 注意事项

- 所有 update / create-or-update 命令都是覆盖逻辑：必须先用对应 detail/list 查询完整数据，在原数据基础上修改后再传入。
- 当前没有可用的 Agoal 写 Golden Route。执行覆盖写前展示对象、变更字段、覆盖范围和最终参数摘要，等待明确确认；执行后回读同一稳定 ID 验证终态。
- 所有命令支持可选参数 `--request-id`。
- `--scope-type` 仅支持 `DEPT` 和 `PERSONAL`。
- `--selected-time` 接受 ISO-8601 字符串，如 `"2026-01-01T00:00:00+08:00"`。
- `--period-ids` 为逗号分隔字符串，如 `"period1,period2"`。
- `report submit-detail` 的 `--query-date` 接受 ISO-8601 字符串；不传则默认当天。
