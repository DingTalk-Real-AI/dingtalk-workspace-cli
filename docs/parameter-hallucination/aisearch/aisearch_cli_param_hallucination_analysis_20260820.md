# AI Search 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、AI Search 实现、dingtalk-aisearch Skill 及冻结正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut 或
已安装插件，也没有修改当前工作区正式别名表。

AI Search 有 3 个 Agent 可见 Schema 叶：`person`、`enterprise`、`behavior`；另有一个
可执行兼容父命令 `dws aisearch --query ...`，等价于 person。`person` 还注册 11 个命令
路径 alias。冻结正式别名表对 AI Search 完全没有 concept、override 或 fixture。主要风险
是槽位幻觉：把人员目标与 dimension 合成一个 flag，把企业内容的 queries/types/time-range
塞进一句自然语言，把行为 action/direction/chat-scope 混成 sender/receiver/group ID，或
把完整手机号、已知 userId 继续送进语义搜人。

候选已通过真实生成器、PreParse、4 组 alias/canonical dry-run 逐字节比较、15 组代表
block/ambiguous、非目标结构恒等、`internal/cli`、`internal/pipeline`、generated drift 和
Schema Catalog 政策。完整 `internal/app` 运行 291.439 秒后仅
`TestCrossPlatformCoverageReviewedParamAliasesHaveCompleteTemplatesAndRepresentativeFinalPayloads`
失败：4 个作用域缺 complete-command E2E 模板，13 条 active alias fixture 未进入最终
payload 等价验证。正式状态为“规则与链路已验证，补 4 个模板后方可落地”。

## 参数问题

### 1. 可执行父命令、Schema 叶和命令路径 alias 容易混为一层

规范路径是 `aisearch person`，但冻结 Cobra 还接受裸 `aisearch --query`，以及
`search/search-person/search-user/user/user-search/query/people/ask/find/lookup/contact` 等
person 路径 alias；enterprise 也有 knowledge/content 等路径 alias。Schema 只发布三个
规范叶，不发布可执行父命令。

命令路径 alias 不属于参数 concept。候选为可执行父命令单独建立精确作用域，只允许人员
query/dimension 的参数别名，并 block enterprise/behavior 槽位；不把 `contact` 路径 alias
解释成通讯录产品参数，也不改任何现有命令身份。

### 2. person 的 query 与 dimension 必须成对表达不同角色

`--query` 是完整保真的人员搜索目标，`--dimension` 是
all/name/department/position/duty/supervisor/subordinate/phone/jobNumber 的逗号分隔枚举。
`--job-number W123`、`--department 产品部` 或 `--phone 138...` 都不是同义 flag：它们还
隐含必须设置 dimension，中央表不能一次生成两个参数。

候选新增 person dimension concept，允许 `dimensions/search-dimension/search-by`；
`search-term/person-query/person-name → query` 只传原值。role-shaped
`job-number/department-id/mobile` block，泛化 `target/phone/department/duty` ambiguous。
维度枚举值本身不做大小写、连字符或中文转换。

### 3. 完整手机号与已知 userId 属于 Contact，不是参数别名

手机号语义线索可用 `person --query ... --dimension phone`；完整手机号精确反查必须用
`contact user search-mobile --mobile`。拿到 userId 后查邮箱/部门/职位必须用
`contact user get --ids`。多候选不能自动取第一个。

候选在 person/root 上 block `mobile/phone-number/user-id/open-dingtalk-id/ids`，避免把
跨产品 SOP 误装成 `query` alias；`phone/id/target` 需消歧。别名层不检查手机号是否完整，
不查人员、不唯一化候选，也不编造返回字段。

### 4. enterprise 要求 queries、types、time-range 已完成槽位拆分

`--queries` 只放主题词；`--types` 是
all/document/im/calendar/todo/minute/report/image/link/notable/baike/mail 的 CSV；
`--time-range` 只接用户明确给出的时间词。诸如“最近 OKR 相关邮件”必须拆成
`queries=OKR`、`types=mail`、`time-range=最近`，不能把原句放进 query。

候选新增 content queries/types/time-range 三个 concept，允许 topics/content-query、
content-types/resource-types、time-window/date-range 等角色完整的名称。`question`、
`natural-language`、泛化 `type/source/range/date/time` ambiguous；behavior/direction、人员 ID
和资源 ID block。候选不从字符串中剥离时间/类型词，也不自动补 types。

### 5. behavior 的 action、direction 与 chat-scope 不能由零散角色拼装

behavior 在 enterprise 三槽之外还有：

- `--behavior-type=all|send|create|share|edit|receive`；
- `--direction` 完整字符串，如 `我->汐峰`；
- `--chat-scope` 仅在 IM 且用户明确群名时使用。

候选新增 behavior type、direction、chat scope concept；`action-type → behavior-type`、
`interaction-flow → direction`、`group-name → chat-scope`。`from/to/sender/receiver` block，
因为别名表不能组合 direction；`chat-id/group-id` block，因为真实参数要自然群名而非 ID；
泛化 action/actor/target/chat/group ambiguous。它也不会因 chat-scope 自动设置 types=im。

### 6. 单值 query、复数 queries 和 CSV 枚举不能自动互换或改值

person `query` 是一个完整搜索目标；enterprise/behavior `queries` 是零到多个主题 CSV。
types、dimension 也是 CSV，但值域不同。Runtime 仅对 search type 的 `doc` 做
`document` 兼容，其他中文词、复数、行为同义词或 `job-number → jobNumber` 不会被候选
转换。

候选因此不复用正式 `search_query`，而建立独立 `aisearch_content_queries`；person 上
`queries` block，enterprise/behavior 上 `dimension` block。所有 concept 都排除无角色
type/category/scope，值保持原样。

### 7. 原生 hidden flags 与 Help/Schema 的兼容面需单独治理

person 原生接受 hidden `keyword/name/q/text/type`；enterprise/behavior 原生接受 hidden
`query/keyword/search-types/searchTypes/timeRange`，behavior 还接受 chatScope/behaviorType。
这些是真实 Cobra flag，生成器禁止中央表 rewrite/block。`type` 在 person 只允许
person/user/people，与 dimension 完全不同。

候选保持这些原生兼容路径，不重复实现。正式后续应评审哪些 hidden flag 应发布到
Help/Schema、哪些只保留兼容期，以及 `contact` 命令路径 alias 是否会放大跨产品误路由；
这些都不是 param_concepts 的所有权。

## 当前别名表可以实施的方案

1. 新增 person dimension、内容 queries/types/time-range、behavior type/chat scope/direction
   七个严格命令范围 concept。
2. 为可执行父命令和三个规范叶建立 4 个 `scope_strict` override。
3. 对同角色名称做原值映射，对完整手机号、已知 ID、资源读写、跨槽输入做 block，对
   `target/type/source/range/action/chat` 等做 ambiguous。
4. 保持命令路径 alias 和所有 hidden native flag 原生；不重复覆盖。
5. 补齐 4 个 complete-command payload 模板后再评审正式替换。

## 当前能力支持不了的事项

- 从完整自然语言自动抽取 query/queries、dimension、types、time-range、behavior-type；
- 一次把 `--job-number/--department/--phone` 改写成 query 加 dimension 两个 flag；
- 判断手机号是否完整并自动切换到 contact；
- 把 userId/openDingTalkId 补成联系人详情，或在多候选中自动选择；
- 把单值 query 与 CSV queries 自动拆分/合并；
- 把中文类型词、行为同义词或 `job-number` 改成枚举值；
- 从 sender/receiver/from/to 拼装 direction；
- 从 chat/group ID 查询群名，或因 chat-scope 自动补 types=im；
- 用别名表新增/删除命令路径 alias 或修改 Help/Schema hidden flag；
- 在没有 complete-command 模板时直接替换正式表。

## 第一轮改造建议

第一轮建议落地 7 个 typed concept 和 4 个作用域的低风险别名/保护。落地 PR 必须为
`aisearch`、`aisearch person`、`aisearch enterprise`、`aisearch behavior` 补
complete-command E2E 模板，覆盖 13 条 active fixture。另行评审父命令与 person 的路径
alias、hidden type/query 兼容面及 Skill 中“完整目标保真”和“剥离维度词”的表述一致性。

## 候选 `param_concepts.json` 改动与审核

- 新增 7 个 AI Search 专用 concept；未扩大任何既有 concept；
- 新增 4 个 command override；
- 新增 30 个 fixture，其中 13 个 active alias、17 个 block/ambiguous；
- `go generate ./internal/cli` 从 569 个命令作用域变为 573 个；
- 非目标 concept、override、fixture 结构恒等；
- 生成 Go diff 只新增 4 个 AI Search entry，fallback 无变化；
- 4 组代表 alias/canonical stdout/stderr 逐字节相同；
- 15 组直接保护检查稳定返回 `blocked_flag` 或 `ambiguous_flag`；
- 审核中发现 `group-name` 已归正式 `group_name` concept，已移出新 concept，改为 behavior
  命令级 alias，避免跨产品语义合并。

候选位置：`docs/parameter-hallucination/aisearch/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 573 个命令作用域 |
| PreParse 与 alias/canonical | 通过 | root-person、person、enterprise、behavior 四组逐字节一致 |
| block/ambiguous | 通过 | 15 组代表性错误均在派发前停止；候选共 17 条保护 fixture |
| 原生参数 | 通过 | hidden flags 与命令路径 alias 保持原生 |
| 非目标回归 | 通过 | JSON 结构恒等；生成 diff 仅 4 个 entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 80.576 秒 |
| generated drift | 通过 | alias 与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 未通过 | 291.439 秒；仅 complete-command 覆盖测试失败 |
| complete-command payload 门禁 | 未通过 | 200/204 个活跃命令已有模板；AI Search 缺 4 个命令、13 条 active fixture；392 active cases |

正式替换前必须补齐 4 个模板并重跑完整 `internal/app` 和政策门禁；未完成前，本候选只
作为完整待审核草稿。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `1742e69d628c3f99945201acc9cd1a46fb7b93a1e080a9f19294a87a97eba514`。
- 命令实现：`internal/helpers/aisearch.go`。
- Skill：dingtalk-aisearch 根 Skill、aisearch reference、intent guide、lite recipes。
- Schema 来源：同一冻结二进制运行时声明组装；未使用历史或固定 Catalog。
