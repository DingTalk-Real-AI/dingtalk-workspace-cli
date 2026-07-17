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
| `strategy update` | 更新战略解码 | `--profile-id` `--content` | **覆盖逻辑，必须基于查询返回的老数据修改后再传入**；`--content` 为 JSON 数组 |

> **`strategy update` 是覆盖式更新**：一定要先 `strategy detail` 获取完整数据，在原数据基础上修改后再传入，会根据战略解码id进行对应的修改。

#### strategy update --content 实体字段说明

| 字段 | 说明 |
|------|------|
| `id` | 实体 id |
| `title` | 标题对象，如 `{"title":"标题文本"}` |
| `linkEntityId` | 所属实体 ID（查询接口中有值时必须回传） |
| `entityType` | 类型枚举：`OGSM_OBJECTIVE`(目的)、`OGSM_GOAL`(目标)、`OGSM_STRATEGY`(策略)、`OGSM_MEASUREMENT`(衡量标准)、`OGSM_TACTICS`(行动方案) |
| `status` | 状态枚举：`NORMAL`(正常)、`PRE_PUBLISH_THEN_UPDATE`(预发布更新)、`PRE_PUBLISH_THEN_CREATE`(预发布新增)、`PRE_PUBLISH_THEN_DELETE`(预发布删除) |
| `supporters` | 承接人数组 `[{type, dingId, staffId}]`；type: `USER`/`DEPARTMENT`；staffId 在 type=USER 时必填 |
| `indicators` | 关键指标 id 字符串数组 |
| `linkSources` | 资源关联数组 `[{id, linkType, linkId, source, objectId, keyResultId}]`；linkType: project/task/campaign/product/doc/standard；source: teambition |
| `executors` | 人员 dingId 字符串数组 |
| `teams` | 部门 dingId 字符串数组 |

### contract (经营合约管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `contract list` | 获取经营合约列表 | `--scope-type` `--scope-id` | scopeType: DEPT/PERSONAL；scope-id 为 scope-type 对应的部门 id 或用户 id |
| `contract fields` | 获取经营合约字段列表 | — | 获取组织下经营合约的字段配置 |
| `contract detail` | 获取经营合约详情 | `--contract-id` | 根据合约 id 查询 |
| `contract update` | 更新经营合约 | `--contract-id` `--dimensions` | **覆盖逻辑**；可选 `--audit-config`、`--objective-template` |

> **`contract update` 同样是覆盖式更新**：必须基于 `contract detail` 返回的数据修改后再传入。

#### contract update --dimensions 维度字段说明

| 字段 | 说明 |
|------|------|
| `id` | 维度 id |
| `title` | 维度名称 |
| `description` | 维度描述 |
| `weight` | 维度权重 |
| `objectives` | 目标列表 |
| `dimensionConfig` | 维度配置 |
| `children` | 子维度列表 |

#### contract update 可选参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `--audit-config` | 审批配置 JSON | `{"needAudit":true,"processTemplateId":"TPL_ID"}` |
| `--objective-template` | 合约模板 JSON | `{"id":"TPL_ID","title":"模板名称"}` |

### scorecard (计分卡管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `scorecard detail` | 获取计分卡详情 | `--selected-time` `--dept-id` | selectedTime 为 ISO-8601 字符串，如 `"2026-01-01T00:00:00+08:00"` |
| `scorecard entity-detail` | 获取计分卡实体详情 | `--sc-id` `--entity-id` | 根据计分卡 id 和实体 id 查询 |
| `scorecard update` | 更新计分卡 | `--dept-id` `--selected-time` `--id` `--tracking-period-type` `--content` | trackingPeriodType: MONTHLY/QUARTERLY |

#### selectedTime 时间说明

`--selected-time` 接受 ISO-8601 字符串，传入对应周期起始时刻：

- **2026年** → `"2026-01-01T00:00:00+08:00"`
- **2026年5月** → `"2026-05-01T00:00:00+08:00"`

#### scorecard update --content 维度字段说明

| 字段 | 说明 |
|------|------|
| `id` | 维度 id |
| `title` | 维度名称 |
| `items` | 指标或关键事项列表 |

items 每项包含：

| 字段 | 说明 |
|------|------|
| `id` | 实体 id |
| `title` | 名称 |
| `reference` | 参考信息 |
| `start` | 起始值 |
| `target` | 目标值 |
| `executors` | 负责人列表，每项包含 `openId` |

#### trackingPeriodType 枚举

| 值 | 说明 |
|------|------|
| `MONTHLY` | 月度追踪 |
| `QUARTERLY` | 季度追踪 |

### user (用户目标管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `user rules` | 获取用户规则周期列表 | — | 可选 `--user-id`，不传则默认取操作人自己 |
| `user objectives` | 查询用户目标列表 | `--user-id` `--rule-id` `--period-ids` | `--period-ids` 为逗号分隔的周期 id 列表 |

### obj-template (目标模板管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `obj-template list` | 获取目标模板列表 | — | 可选 `--keyword` 搜索关键词、`--page` 页码、`--page-size` 每页数量 |
| `obj-template create-or-update` | 新增或更新目标模板 | `--dimensions` | **覆盖逻辑**；新增时 `--title` 必填；更新时 `--template-id` 必填，dimensions 必须基于老数据修改 |

> **`obj-template create-or-update` 同样是覆盖式更新**：更新时必须基于 `obj-template list` 返回的数据修改后再传入。新增时建议先参考已存在的模板数据再构建 dimensions。

#### obj-template create-or-update 参数说明

| 参数 | 说明 | 类型 |
|------|------|------|
| `--template-id` | 模板 id（更新时必填） | string |
| `--title` | 模板标题（新增时必填） | string |
| `--objective-weight` | 是否启用目标权重 | bool |
| `--dimension-weight` | 是否启用维度权重 | bool |
| `--compute-by-weight` | 维度是否参与计算 | bool |
| `--dimensions` | 模板关联的维度 JSON 字符串 | string |

### report (周月报管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `report list-statistics` | 获取周月报数据跟催列表 | — | 返回各规则的人员提交情况统计（按时/迟交/未提交人数）；可选 `--keyword` 搜索规则名称 |
| `report submit-detail` | 获取周月报规则提交详情 | `--template-id` `--submit-state` | submitState: `ON_TIME`(按时)/`LATE`(迟交)/`NOT_SUBMITTED`(未提交)；可选 `--query-date`(ISO-8601)、`--page`、`--page-size`、`--keyword`(搜索员工名称) |

#### report submit-detail --submit-state 枚举

| 值 | 说明 |
|------|------|
| `ON_TIME` | 按时提交 |
| `LATE` | 迟交 |
| `NOT_SUBMITTED` | 未提交 |

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
- 查看某规则的提交详情（按时/迟交/未提交人员明细） → `report submit-detail`

## 核心工作流

```bash
# 1. 查看战略解码列表（按部门）
dws agoal strategy list --scope-type DEPT --scope-id DEPT_ID

# 1.1 查看战略解码列表（按个人）
dws agoal strategy list --scope-type PERSONAL --scope-id USER_ID

# 2. 查看战略解码详情
dws agoal strategy detail --profile-id PROFILE_ID

# 3. 更新战略解码（基于详情返回的数据修改后传入）
dws agoal strategy update --profile-id PROFILE_ID \
  --content '[{"id":"entity1","title":{"title":"新目标"},"entityType":"OGSM_OBJECTIVE","status":"NORMAL","executors":["dingId1"]}]'

# 4. 查看经营合约列表（按个人）
dws agoal contract list --scope-type PERSONAL --scope-id USER_ID

# 4.1 查看经营合约列表（按部门）
dws agoal contract list --scope-type DEPT --scope-id DEPT_ID

# 5. 查看经营合约字段列表
dws agoal contract fields

# 6. 查看经营合约详情
dws agoal contract detail --contract-id CONTRACT_ID

# 7 更新经营合约（基于详情返回的数据修改后传入）
dws agoal contract update --contract-id CONTRACT_ID \
  --dimensions '[{"id":"dim1","title":"维度名称","objectives":[...]}]'

# 8. 查看计分卡详情
dws agoal scorecard detail --selected-time "2025-01-01T00:00:00+08:00" --dept-id DEPT_ID

# 9. 查看计分卡实体详情
dws agoal scorecard entity-detail --sc-id SC_ID --entity-id ENTITY_ID

# 10. 更新计分卡
dws agoal scorecard update --dept-id DEPT_ID --selected-time "2025-01-01T00:00:00+08:00" --id SC_ID --tracking-period-type MONTHLY --content '[{"id":"dim1","title":"业绩","items":[{"id":"item1","title":"收入","target":"100"}]}]'

# 11. 查看用户规则 → 提取 ruleId 和 periodId
dws agoal user rules --user-id USER_ID

# 12. 查看用户目标
dws agoal user objectives --user-id USER_ID --rule-id RULE_ID --period-ids "period1,period2"

# 13. 查看周月报提交统计列表
dws agoal report list-statistics

# 13.1 按关键词搜索规则
dws agoal report list-statistics --keyword "周报规则"

# 14. 查看某规则的按时提交详情
dws agoal report submit-detail --template-id TPL_ID --submit-state ON_TIME

# 14.1 查看迟交详情（带分页和日期）
dws agoal report submit-detail --template-id TPL_ID --submit-state LATE --query-date "2026-06-18T00:00:00+08:00" --page 1 --page-size 20

# 15. 获取目标模板列表
dws agoal obj-template list

# 15.1 搜索目标模板
dws agoal obj-template list --keyword "业绩"

# 16. 新增目标模板
dws agoal obj-template create-or-update --title "业绩模板" --objective-weight --dimension-weight --dimensions '[...]'

# 16.1 更新目标模板
dws agoal obj-template create-or-update --template-id TPL_ID --title "业绩模板" --dimensions '[...]'
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
| `report list-statistics` | `templateId`（列表项中） | `report submit-detail` 的 `--template-id` |
| `obj-template list` | `templateId` | `obj-template create-or-update` 的 `--template-id`(更新时) |

## 注意事项

- **所有 update 命令都是覆盖逻辑**：必须先用对应的 detail/list 查询到完整数据，在原数据基础上修改后再传入，否则未传入的数据会被删除
- 所有命令支持可选参数 `--request-id`
- `--scope-type` 仅支持 `DEPT`（按部门）和 `PERSONAL`（按个人）两种
- `--selected-time` 接受 ISO-8601 字符串（如 `"2026-01-01T00:00:00+08:00"`）
- `--period-ids` 为逗号分隔的字符串，如 `"period1,period2"`
- `report submit-detail` 的 `--query-date` 接受 ISO-8601 字符串（如 `"2026-06-18T00:00:00+08:00"`），不传则默认当天
- `report submit-detail` 的 `--page` 和 `--page-size` 默认为 1 和 10，不传时由服务端使用默认值
