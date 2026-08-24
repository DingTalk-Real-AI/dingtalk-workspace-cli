# HRBrain 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、官方 Cobra 树、HRBrain 实现与测试、`dingtalk-misc` 中的
HRBrain Skill，以及冻结正式 `internal/cli/param_concepts.json`。未使用固定 Catalog、历史
badcase、用户 Shortcut 或已安装插件，也没有修改当前工作区正式别名表。

HRBrain 有 11 个 Agent 可见叶，覆盖人才池 3 个、员工档案 5 个、人才搜索 3 个。Skill、Help、
Schema 和实现对公开命令、canonical flag、必填性与 JSON/逗号分隔外形基本一致。候选新增 3 个
概念（人才池编码、单工号、多工号），并把既有搜索、页码和每页条数概念按精确命令扩展到
HRBrain；为 11 个叶增加 `scope_strict` override，其中无业务参数的 `search fields` 只做保护。
候选共 59 个 fixture：31 个 active、28 个 guard。生成结果为 86 个 alias、142 个 blocked、
64 个 ambiguous，命令作用域从 569 增至 580，command path fallback 保持 34。

候选已通过生成、31 组 alias/canonical payload 等价、28 组 block/ambiguous dispatch 前保护、
JSON 外层校验、真实全局参数保持原生、非目标结构恒等、HRBrain 专项测试、`internal/cli`、
`internal/pipeline`、embedded fixture delivery、generated drift 和 Schema Catalog 政策。
候选单独替换时，完整 `internal/app` 会按政策失败：10 个含 active alias 的 HRBrain 命令尚未在
`paramAliasCompleteCommands` 中登记完整 canonical payload 模板。隔离副本临时补齐这 10 条模板后，
payload-equivalence 专项和完整 `internal/app`（237.773 秒）均通过，未发现第二处隐藏失败。因此
候选结论是**条件通过**：正式落地必须把候选与 10 条完整命令模板作为同一变更提交并重跑全门禁，
不能只替换 JSON。

## 参数问题

### 1. 员工工号在单值与多值命令中命名不一致

`profile metadata/query/career/performance` 使用单值 `--work-no`；`profile labels` 使用逗号分隔
多值 `--staff-ids`。Agent 容易在单值命令生成 `--employee-id`、`--staff-id`、`--job-number`，
也容易把单值 `--work-no` 直接用于多人工号列表。

候选分别建立 `hrbrain_work_no` 与 `hrbrain_work_nos`，只把同角色、同值域、同单复数且可原样传递
的名称归一化。单值与多值之间，以及 userId/openDingTalkId/手机号与工号之间，全部 block 或
ambiguous，绝不做自动互转。

### 2. 人才池编码容易与名称、通用 ID 或员工标识混用

`talent-pool detail/employees` 和 `search employees` 的 `--pool-code` 都表示人才池编码；
`talent-pool list --keyword` 才是人才池名称关键词。`pool-id`、裸 `id`、`pool-name` 看似接近，
但冻结接口没有证明其值域与 poolCode 等价。

候选建立 `hrbrain_pool_code`，只接受明确的 code 同义写法，并在精确命令绑定 canonical
`--pool-code`。名称、通用 ID、员工 ID 保持 block/ambiguous；不会把名称搜索伪装成编码查询。

### 3. 搜索与分页参数存在常见拼写差异，但必须限制命令范围

`talent-pool list` 和 `search employees` 的 `--keyword` 是搜索文本；4 个列表/搜索叶使用
`--page` 与 `--page-size`。这些参数可安全吸收明确同义词，但 `query` 在 `profile query` 中是
命令动作，JSON `data-queries` 也不是搜索关键字；`limit`、`offset`、`cursor` 的语义和单位不能
一概等同。

候选复用既有 `search_query`、`page_number`、`pagination_size`，仅追加已审核 HRBrain 精确命令。
会改变分页模型或单位的名称被保护，不扩散到 detail、profile 或无参数的 `search fields`。

### 4. JSON 与逗号分隔参数容易被错误包装或互换

`profile query --data-queries` 必须是非空 JSON 数组；`search employees-structured
--origin-json` 必须是 JSON object，业务 `--fields` 必须是非空 JSON 数组；`--labels`、
`--staff-ids`、`--order-by` 则是逗号分隔字符串。别名表只能改 flag 名，不能把 CSV 拆成 JSON、
把对象包成数组，或补齐数组成员字段。

运行时能拒绝非法 JSON、空数组和错误的 object/array 外形，但当前不会逐项验证
`data-queries` 的 `modelCode/fields` 或 structured `fields` 的成员结构。候选只做名称保护，不声称
完成值转换或深层 Schema 校验。

### 5. `--fields` 在同产品内存在业务 flag 与全局 flag 的角色碰撞

`search employees-structured --fields` 是必填业务 JSON 数组，会进入接口 payload；其余 HRBrain
叶继承的 `--fields` 是全局输出字段投影。两者拼写完全相同，但含义、值格式和落点不同。

中央参数字典不能在解析前仅凭 flag 名区分用户意图，也不能把一个真实 flag 重写成另一个角色。
候选保持两种原生行为，不为 `fields/columns/select` 增加跨角色 alias；报告和 Skill 必须继续明确
structured 叶需要 JSON 数组，而其他叶的 `--fields` 只是输出投影。

### 6. 数字分页只校验类型，没有正数范围契约

`--page`、`--page-size` 是 int，默认分别为 1 和 20；非整数由 Cobra 拒绝，但 `--page 0` 会通过
并进入 payload。别名表不能新增数值范围，也不能安全地把 offset/cursor 换算成页码。

候选不修改数值，只做同单位、同模型的名称归一化。若业务要求正数，必须在 leaf Contract/Runtime
增加显式范围约束并补测试，不能靠 alias 表假装已校验。

## 当前别名表可以实施的方案

1. 新增 `hrbrain_pool_code`，绑定人才池详情、人才池人员与简单人才搜索中的 `--pool-code`。
2. 新增 `hrbrain_work_no`，绑定 4 个单员工档案叶的 `--work-no`。
3. 新增 `hrbrain_work_nos`，只绑定标签查询的逗号分隔 `--staff-ids`。
4. 将既有 `search_query` 精确扩展到人才池列表和简单人才搜索。
5. 将既有 `page_number`、`pagination_size` 精确扩展到 4 个列表/搜索叶。
6. 为 11 个叶增加 `scope_strict` override，用 block/ambiguous 隔离 ID 值域、单复数、JSON/CSV、
   分页模型、字段角色和不存在的身份参数。
7. 保持所有真实 canonical flag 和全局 flag 原生，尤其保持 structured 业务 `--fields` 与其他叶
   全局输出 `--fields` 的各自行为。
8. 正式落地时同步登记 10 条完整命令 canonical payload 模板，使 active fixture 进入最终等价门禁。

## 当前能力支持不了的事项

- 在工号、userId、openDingTalkId、手机号之间查询或转换；
- 将单工号和逗号分隔多工号自动拆分、合并或去重；
- 从人才池名称查出 poolCode，或把通用 pool ID 转换为 poolCode；
- 把 CSV 的 labels/staff-ids/order-by 转成 JSON，或反向转换；
- 给 data-queries、origin-json、structured fields 自动补 JSON 包装或成员字段；
- 深层验证 `data-queries[].modelCode/fields` 和 structured `fields[]` 的业务结构；
- 在解析前消除业务 `--fields` 与全局输出 `--fields` 的同名角色碰撞；
- 把 offset/cursor/limit 换算成 page/page-size；
- 约束 page/page-size 必须为正数；
- 根据姓名、部门或手机号先查人再填工号；这属于 `aisearch/contact` 编排，不是别名改写；
- 绕过人才池查看权限、档案权限或登录 profile 边界。

## 第一轮改造建议

第一轮可落地 3 个专属概念、3 个既有概念的精确命令扩展、11 个严格 override 和 59 个 fixture，
但必须同时补齐 10 条 `paramAliasCompleteCommands` 模板。这样既能吸收同值域、同角色、同单位的
常见拼写，又能在 dispatch 前阻止工号值域、单复数、人才池名称/编码、JSON/CSV、分页模型和
fields 角色误用。JSON 深层结构、正数范围和跨产品身份解析应作为 Runtime/Contract 或编排层后续，
不能扩大本轮 alias 范围。

## 候选 `param_concepts.json` 改动与审核

- 新增 3 个 concept：`hrbrain_pool_code`、`hrbrain_work_no`、`hrbrain_work_nos`；
- 扩展既有 `search_query`、`page_number`、`pagination_size` 的 HRBrain 精确命令范围；
- 新增 11 个 `scope_strict` command override；
- 新增 59 个 fixture：31 active、28 guard；active 覆盖 10 个有业务参数的叶；
- `search fields` 没有业务参数，只做 guard，不制造 alias；
- `go generate ./internal/cli` 的命令作用域 569→580；HRBrain 生成 86 alias、142 blocked、
  64 ambiguous；command path fallback 仍为 34；
- 删除 HRBrain 改动后，非目标 concept、override、fixture 与冻结正式表结构恒等；
- 自动 alias 的 source 均不是该精确命令中具有不同语义的真实 flag，target 均为真实 canonical flag；
- 31 个 active case 全部与 canonical payload 等价，28 个 guard 均在 dispatch 前停止；
- 非法 JSON/空数组/错误外形和非整数分页被 Runtime/Cobra 拒绝；`page=0` 仍会进入 payload，已明确
  列为 Runtime 范围缺口；
- 当前工作区正式别名表未被修改；候选是基于冻结正式表的完整独立草稿，不累计其他产品候选。

审核结论：规则本身范围正确且行为验证通过，但仓库政策要求 active alias 命令具备完整 canonical
payload 模板。候选位置：`docs/parameter-hallucination/hrbrain/param_concepts.json`。正式落地状态为
**条件通过**，必须与 10 条模板一起提交。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 580 个命令作用域；HRBrain 86 alias、142 blocked、64 ambiguous |
| active alias/canonical | 通过 | 31 组 PreParse 后 canonical argv 与 payload 等价 |
| block/ambiguous | 通过 | 28 组均在 HRBrain dispatch 前停止，dispatch 0 |
| JSON/类型校验 | 通过（有限） | 5 类非法值拒绝；不覆盖数组成员深层结构和正数范围 |
| 全局参数原生行为 | 通过 | hidden/global flag 未被覆盖；fields 角色按命令保持原生 |
| 非目标回归 | 通过 | 正式 JSON 非目标结构恒等；generated diff 仅 HRBrain；fallback 不变 |
| HRBrain 专项测试 | 通过 | `TestHrbrain*`，0.948 秒 |
| embedded fixture delivery | 通过 | fixture 经最终嵌入加载路径生效，0.792 秒 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 72.963 秒；pipeline 0.490 秒 |
| generated drift | 通过 | 参数别名与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整应用（候选单独） | 阻断 | 10 个 active 命令缺少 complete-command payload 模板 |
| 模板补齐验证 | 通过 | 临时补 10 条模板后 payload-equivalence 专项通过，1.613 秒 |
| 完整应用（候选+模板） | 通过 | `go test ./internal/app -count=1`，237.773 秒 |

正式替换前必须在 `internal/app/param_alias_payload_equivalence_test.go` 为以下 10 个命令登记完整
canonical payload 模板，并与候选一起评审：`talent-pool list/detail/employees`、
`profile metadata/query/labels/career/performance`、`search employees/employees-structured`。
落地后应从同一基线合并所有获批产品差异，而不是依次覆盖独立候选，并重跑生成、PreParse、
payload equivalence、全量应用和全部仓库政策。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00；
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`；
- 候选 SHA-256：
  `ebae9af7df01928031bf444e829e10232e97b058d5b114774a416445adacf55c`；
- 命令实现与声明：`internal/helpers/hrbrain.go`；
- 专项测试：`internal/helpers/hrbrain_test.go`；
- Skill：`dingtalk-misc/references/hrbrain.md`；
- Schema：同一冻结二进制通过 `ResolveSchemaBuild` 运行时声明组装；
- 官方树边界：11 个 Agent 可见叶，无用户 Shortcut 或安装插件；
- 隔离副本：`/private/tmp/dws-param-analysis-aa4ae9a90323`；
- 候选验证只使用 dry-run/mock/测试 seam，不访问或修改真实员工档案、人才池与组织数据；
- 明确未使用：固定 Catalog、历史 badcase、评测工作簿、用户 Shortcut、已安装插件。

## 可复用分析流程

对同时包含标识符、列表搜索、分页、JSON 与 CSV 的产品，先按“实体、角色、值域、单复数、单位、
值格式”拆分，再检查同名 flag 是否在业务层和全局层承担不同角色。只有原值可直接传递的同义名称
进入精确命令 alias；跨值域、跨单复数、跨分页模型和结构转换全部 fail closed。最后必须用完整
canonical payload 模板验证 active fixture，而不能只证明 PreParse 文本发生了改写。
