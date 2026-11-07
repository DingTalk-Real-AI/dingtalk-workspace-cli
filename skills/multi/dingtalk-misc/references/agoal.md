# Agoal（目标管理）

Agoal 覆盖战略解码、经营合约、计分卡、用户目标、目标模板和周月报规则跟催。CLI 前缀为 `dws agoal`。

所有结构化查询都带 `--format json`。已知路径直接执行；只有当前二进制的叶子命令或参数不确定时才读一次精确 `--help`，不要用 Profile、Auth、搜索其它产品或本地脚本猜命令和业务字段。

## 产品边界

- 日报、周报、月报正文的填写、提交、收件箱和已发送记录走 Report。
- 周月报规则级按时、迟交、未提交统计，人员清单和跟催走 Agoal。
- 普通待办走 Todo；Agoal 目标、规则、周期、战略解码和经营合约不转成待办查询。

## Golden Route 优先级

当前公开且经过稳定返回校验的 Agoal Shortcut 只有以下 5 条。意图能够满足时优先使用，不要手写等价的原子调用或解析脚本。

| 意图 | 首选命令 | 稳定输出 |
|---|---|---|
| 经营合约字段配置 | `dws agoal +contract-fields --format json` | `count`、`fields` |
| 规则及周期列表 | `dws agoal +user-rules --format json` | `count`、`rules` |
| 周月报规则统计 | `dws agoal +report-statistics-list --format json` | `count`、`statistics` |
| 周月报人员详情 | `dws agoal +report-submit-detail ... --format json` | `count`、`submissions`、`page`、`pageSize`、`totalCount` |
| 目标模板分页 | `dws agoal +obj-template-list ... --format json` | `count`、`templates`、`page`、`pageSize`、`totalCount` |

以下带 `+` 的 Shortcut 当前明确 `unavailable`，不要尝试：`+strategy-list`、`+strategy-get`、`+strategy-update`、`+contract-list`、`+contract-get`、`+contract-update`、`+scorecard-get`、`+scorecard-entity-get`、`+scorecard-update`、`+user-objectives`、`+obj-template-upsert`。这不表示同名原子能力一定不可用；例如当前叶子 Help 存在时仍可执行 `dws agoal strategy list ...`。确需对应能力时，只使用当前叶子 Help 存在的原子命令，并按本文件的安全规则解释结果。

## 意图路由

| 用户意图 | 路由 |
|---|---|
| 战略解码 / 战略目标 / OGSM 列表 | `strategy list` |
| 战略解码详情或更新 | `strategy detail` → 必要时 `strategy update` |
| 经营合约列表或详情 | `contract list` / `contract detail` |
| 经营合约字段、必填或强制字段 | `+contract-fields` |
| 计分卡详情、实体或搜索 | `scorecard detail` / `scorecard entity-detail` / `scorecard search-entities` |
| 目标规则、周期 | `+user-rules`；需要默认偏好时见下方 SOP |
| 个人目标正文 | `user rules` → `user objectives` |
| 周月报规则统计、迟交、未提交、跟催 | `+report-statistics-list` → 按需 `+report-submit-detail` |
| 目标模板查询 | `+obj-template-list` |
| 新增或更新目标模板 | `obj-template create-or-update`，按覆盖写安全流程 |

## 高频只读 SOP

### 本人与主部门范围

涉及“我本人”“个人与主部门分别查询”时，切换到 Contact skill 解析稳定身份并执行：

```bash
dws contact user get-self --format json
```

从真实返回提取 `orgEmployeeModel.userId` 和主部门 ID。禁止把 `me`、`self`、空字符串、用户名或 Profile 当作 `--scope-id` / `--user-id`。如果返回多个部门但没有唯一主部门标记，先让用户选择，不默认取第一项。

分别查询个人和部门范围，不用另一范围补空结果：

```bash
dws agoal strategy list --scope-type PERSONAL --scope-id <USER_ID> --format json
dws agoal strategy list --scope-type DEPT --scope-id <DEPT_ID> --format json
dws agoal contract list --scope-type PERSONAL --scope-id <USER_ID> --format json
dws agoal contract list --scope-type DEPT --scope-id <DEPT_ID> --format json
```

原子查询返回 `success=true` 且 `content=[]` 时，结论是该精确范围为空；不要换上级部门、其它人员或搜索结果补数。

### 经营合约字段

```bash
dws agoal +contract-fields --format json
dws agoal +contract-fields --keyword <KEYWORD> --format json
```

按返回字段解释，不互相推断：`required` 决定必填/可选分组，`active` 表示是否启用，`forceActive` 和 `forceRequired` 只在其真实为 `true` 时标为强制。若 `required=false` 与 `forceRequired=true`，或 `active=false` 与 `forceActive=true` 同时出现，保留原值并标记服务端字段冲突，不自行改组或选择优先级。字段编码取 `code`，类型取 `type`，格式补充可取 `scheme.format`。

### 默认规则、周期与个人目标

只需规则和周期列表时使用 `+user-rules`。当前 Shortcut 不投影顶层 `preference`；用户问“默认/偏好规则或周期”时，改用原子读取：

```bash
dws agoal user rules --format json
```

从 `content.preference.ruleId` 和 `content.preference.periodId` 精确定位默认规则与周期，不把第一条规则、`defaultPeriodIds` 或最近开始的周期冒充偏好。

查询本人目标前，用 `contact user get-self` 取得真实用户 ID。比较“当前偏好周期与前一期”时：

1. 以 `preference.periodId` 对应周期为当前期。
2. 用 `nameEN` / `nameCn` 中的季度 `Q1`～`Q4`、半年 `S1`～`S2`、`Annual` / `年度` 识别粒度。在 `currentPeriods + historyPeriods` 的同粒度候选中，选择 `endDate < 当前期 startDate` 且 `endDate` 最大的一项；没有唯一候选时先让用户选择。
3. 每个周期单独查询以保留空周期，不为回答两期而扩展查询其它粒度。

```bash
dws agoal user objectives --user-id <USER_ID> --rule-id <RULE_ID> --period-ids <PERIOD_ID> --format json
```

秒或毫秒时间戳统一转换为 `Asia/Shanghai` 的可读时间；不要根据 FY 名称猜日期。

### 周月报统计与人员明细

```bash
dws agoal +report-statistics-list --keyword <RULE_KEYWORD> --format json
dws agoal +report-submit-detail --template-id <TEMPLATE_ID> --submit-state <ON_TIME|LATE|NOT_SUBMITTED> --query-date <ISO_DATE> --page 1 --page-size 10 --format json
```

- `enableStatistic=false` 时，列表里的 `onTime` / `late` / `notSubmitted` 零值不能证明无人提交；明确标为“统计未启用/列表值不可据此判断”。使用 `--keyword` 后仍按真实返回规则名核对匹配结果，不仅凭请求参数声称全部名称匹配。
- 用户要求找人数最多的规则，而候选统计关闭或并列时，对所有候选用同一 `query-date` 查询目标状态的第一页，以 `totalCount` 排名；并列再按用户指定规则处理，未指定时可按真实 `lastModified` 最近者展开并说明。
- 选中规则后，再分别查询 `ON_TIME`、`LATE`、`NOT_SUBMITTED`。把列表统计值和明细 `totalCount` 并排展示，差异单列。
- “今天”必须先按当前会话时区确定一个日期，并在同一任务的所有明细调用中显式传入同一个 `--query-date`，避免跨午夜漂移。
- 人员详情属于敏感数据，只返回用户要求的最小字段，不把原始人员结果写入无关文档或日志。

### 目标模板分页与去重

```bash
dws agoal +obj-template-list --keyword <KEYWORD> --page 1 --page-size 10 --format json
dws agoal +obj-template-list --keyword <KEYWORD> --page 2 --page-size 10 --format json
```

用户明确要求前两页时必须真实查询两页，不因第一页 `totalCount` 较小就声称第二页为空。逐页报告 `count`，用服务端 `totalCount` 表示系统总数；合并时优先用稳定 `id`，缺失时用 `templateId` 去重，标题相同但 ID 不同仍是不同模板。Golden Route 会在两者都缺失时失败；不要回退为按标题去重。权重只按 `objectiveWeight`、`dimensionWeight`、`computeByWeight` 的真实布尔值解释。

## 其它原子命令

仅在上述 Golden Route 不覆盖且当前叶子 Help 存在时使用：

```bash
dws agoal strategy detail --profile-id <PROFILE_ID> --format json
dws agoal contract detail --contract-id <CONTRACT_ID> --format json
dws agoal scorecard detail --selected-time <ISO_TIME> --dept-id <DEPT_ID> --format json
dws agoal scorecard entity-detail --sc-id <SC_ID> --entity-id <ENTITY_ID> --format json
dws agoal scorecard search-entities --keyword <KEYWORD> --page 1 --page-size 20 --format json
```

后续命令使用的 `profileId`、`contractId`、`scId`、`entityId`、`ruleId`、`periodId` 和 `templateId` 必须来自前一步真实返回。

## 写操作硬约束

当前没有可用的 Agoal 写 Golden Route。`strategy update`、`contract update`、`scorecard update` 和 `obj-template create-or-update` 都是覆盖写：

1. 先读取目标对象的完整详情；模板更新先读取稳定模板 ID 对应的完整数据。
2. 只在完整旧数据上修改用户指定字段，保留其余字段；禁止构造局部 JSON 覆盖。
3. 执行前展示对象、变更字段、覆盖范围和最终参数摘要，等待用户明确确认；确认后才允许追加 `--yes`。
4. 执行后回读同一稳定 ID，逐字段验证终态；读回不一致或不可用时不得宣称成功。

新增模板同样属于写操作。没有删除或恢复闭环时，不把试写当作验证手段。
