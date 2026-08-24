# 12 产品参数别名表合并候选复核

> 历史记录：本文对应 2026-08-20/21 的 12 产品阶段候选，已被
> [全 DWS 产品参数兜底复核与落地状态（2026-08-21）](all_dws_product_param_fallback_review_20260821.md)
> 取代。当前同目录 `param_concepts.json` 是后者的 30 产品复核候选，本文中的旧哈希与旧规模不再指向当前文件。

## 结论

已基于验证时最新的线上 `origin/main` 提交
`765b961f4d127c5d270b22d8fd31092a52a6a1f2`（2026-08-21 01:06:18 +08:00，
`Merge pull request #1071 from DingTalk-Real-AI/codex/fix-stable-active-fragments`）生成并复核
12 产品统一合并候选：

- 候选：[`param_concepts.json`](param_concepts.json)
- SHA-256：`fea128df3caffcd1c5a9b90677dd40c3bc6b00c0df7e4c855aaaa3ae8303d294`
- 最新线上基线正式表 SHA-256：`d2efc9507b1455c7a41e2af16f45c7531f47aff63bbfe58401e96911af0ae440`
- 合并后规模：73 个 concept、409 个 command override、998 条验证 fixture

候选经过连续必要性复核后，将 9 个只服务单一逻辑命令的局部角色从中央 concept 降为
`scoped_aliases`，删除 15 个没有直接样本支撑的推测性 concept member，并把 7 个无法仅凭
flag 名证明值域或角色的拼写改为 fail-closed ambiguous。修复后的字典
结构、作用域和冲突处理正确、合理；生成、PreParse、alias/canonical、
block/ambiguous、嵌入式 guard、非目标保持、生成确定性和 Schema 仓库政策检查均通过。
但它目前仍是**评审候选，不应直接替换正式表**：正式落地还需要补 45 个命令的完整 payload
模板，并先修复最新 main 中 Whiteboard 在 fresh declaration-only 命令树不可见的问题。

本次未修改当前工作区正式 `internal/cli/param_concepts.json`。

## 合并范围与方法

合并产品为 report、ding、event、markdown、aisearch、whiteboard、audit、pat、devdoc、live、
mcp、hrbrain。12 份产品候选均从原统一冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 派生；本次没有依次覆盖完整 JSON，而是：

1. 从每份产品候选提取相对原冻结正式表的 concept、override、fixture 精确差异；
2. 将差异重放到最新 `origin/main` 正式表；
3. 对共享 concept 按字段语义构造并集；
4. 对冲突项逐条回到真实生成器、Cobra/Shortcut 参数和嵌入式 fixture 验证；
5. 在隔离 worktree `/private/tmp/dws-param-merged-765b961f` 中替换输入并统一验证。

最新 main 已包含 Minutes、Todo、Wiki 的后续别名改动。本候选完整保留这些内容，没有删除或
覆盖任何最新基线的 concept、override、fixture 或非目标顶层结构。

## 差异审计

| 项目 | 最新 main | 合并候选 | 说明 |
|---|---:|---:|---|
| concept | 62 | 73 | 11 个新增；另有 19 个既有 concept 扩围 |
| command override | 349 | 409 | 63 个产品差异项，其中 3 个修改既有 override，净增 60 |
| validation fixture | 675 | 998 | 原产品新增 309 条，移除 5 条错误中央归因，新增 3 条合并消歧 fixture、12 条语义收敛 fixture 和 4 条 openDingTalkId 角色保护 fixture，净增 323 |

静态三方审计结果：

- 最新基线内容全部保留；
- 39 个预期变更 concept 和 63 个预期变更 override 的产品语义均存在；
- 无重复 member、exclude、command 数组项；
- 无重复 fixture；
- 12 产品之外没有意外 override 扩散；
- 最新 main 的 Minutes/Todo/Wiki 命令范围未回退。

共享 concept 均按并集处理。尤其 `page_number` 同时保留最新 Todo、Devdoc 与 HRBrain 语义：

- members：`page`、`page-no`、`current-page`、`page-num`、`page-number`；
- commands：Devdoc 搜索、Todo 分页命令与 HRBrain 人才池/人员搜索命令的并集；
- excludes：继续阻断 `cursor`、`page-index`、`page-size`、`page-token`。

## 复核中发现并修正的三类问题

### 1. `source-file` 跨产品 concept 冲突

独立产品候选中，Markdown 的 `markdown_local_file_path` 与 Whiteboard 的
`whiteboard_source_file` 都声明了成员 `source-file`。合并后真实生成器会拒绝同一成员被两个
concept 全局认领。

最终处理为：

- 不再让任何中央 concept 全局认领 `source-file`；
- 在 `markdown create`、`markdown diff`、`markdown overwrite` 三个精确命令上声明
  `source-file -> file` scoped alias；
- 在 `whiteboard update` 上声明 `source-file -> source` scoped alias；
- 为四个精确命令分别保留 active fixture。

这样既消除全局歧义，又保持两个产品的实际 argv 兼容面，不会把 Whiteboard 的“源文件”概念
扩散到所有 Markdown 路径。

### 2. 5 条 fixture 错把原生参数兼容当成中央别名

嵌入式运行时测试证明以下拼写由具体 Cobra/Shortcut 命令直接拥有，不会物化为中央
semantic alias：

| 命令 | 拼写 | 真实归属 |
|---|---|---|
| `markdown fetch` | `node-id` | 命令原生 flag/兼容面 |
| `markdown create` | `parent-id` | 命令原生 flag/兼容面 |
| `markdown patch` | `doc-id` | 命令原生 flag/兼容面 |
| `pat chmod` | `agent-code` | 命令原生 flag/兼容面 |
| `pat browser-policy` | `agent-code` | 命令原生 flag/兼容面 |

候选删除了这 5 条错误 active fixture，但没有删除命令行为。临时专项测试同时确认这些拼写仍由
对应命令接受，并且不会被中央 alias 层错误重写。

### 3. 9 个命令局部角色不应提升为中央 concept

必要性复核发现，以下 9 个实体只服务一个逻辑命令，且不存在跨命令稳定身份复用：

- `markdown_diff_context_lines`；
- `markdown_patch_pattern`；
- `markdown_patch_regex`；
- `aisearch_behavior_type`；
- `aisearch_chat_scope`；
- `aisearch_direction`；
- `whiteboard_source_file`；
- `aisearch_person_dimension`；
- `hrbrain_work_nos`。

候选删除这些 concept，并把已经审核为同角色、同值域、可原样透传的拼写迁入对应命令的
`scoped_aliases`。其中裸 `aisearch` 是 `aisearch person` 的可执行兼容父入口，不是第二个
Schema identity；`hrbrain_work_nos` 也只服务 `hrbrain profile labels` 一个叶。两者的安全
保护继续由各自精确 override 的 block/ambiguous 承担。

同时收紧 5 个无法仅从 flag 名证明值域的拼写：

- `content-query`：不能自动升格为复数 CSV `queries`；
- `period`：不能确定是自然语言时间范围还是周期/duration；
- `activity-type`：不能确定是 behavior enum；
- `im-scope`：不能确定其值是自然语言群名；
- `pool`：不能确定其值是人才池编码而不是名称。

这些拼写现在均在精确命令中返回 `did-you-mean:ambiguous`，不会进入 Cobra 或 MCP 派发。

### 4. 删除 15 个无直接样本支撑的推测性 member

继续按“至少存在一条 active fixture、且不会借用其他命令正式 flag 或依赖额外角色条件”的标准
复核剩余 11 个新增 concept，删除以下未被真实样本验证的成员：

- Event：`peer-open-dingtalk-id`、`sender-open-dingtalk-id`、`max-event-count`、
  `event-count-limit`、`event-subscription-id`；
- Markdown：`local-markdown-file`、`markdown-name`；
- AI Search：`topic-queries`、`content-queries`、`search-terms`、`search-categories`、
  `search-type-list`、`date-range`、`search-period`；
- Whiteboard：`board-part-id`。

其中 `sender-open-dingtalk-id` 是 `chat message list-by-sender` 的真实原生 flag，并不是 Event
观察到的误写；`peer-open-dingtalk-id` 也需要先知道目标角色。两者因此不再自动改写，并在
`event +listen-im`、`event consume` 两个精确命令中显式返回 `did-you-mean:ambiguous`，由 4 条
新增 fixture 覆盖。其余成员没有 active fixture，也没有独立 Runtime/Cobra 证据，直接从候选
成员集合删除，不用新的推测性 guard 取代旧的推测性 alias。

复核后，剩余 11 个新增 concept 的每一个非 canonical member 都至少有一条 active fixture；
`hrbrain_pool_code` 与 `hrbrain_work_no` 的所有同义成员也都有各自命令样本，因此继续保留。

## 验证结果

全部验证均在隔离副本进行。为绕过最新 main 已存在的 Whiteboard fresh-tree 可见性缺陷，仅在
隔离副本临时把 `whiteboard` 加入 declaration-only 静态命令集合；该代码改动没有写入当前
工作区，也不属于本候选。

| 验证项 | 结果 |
|---|---|
| JSON Schema、生成器冲突检查 | 通过，生成 761 个命令别名表 |
| PreParse、alias/canonical、block/ambiguous、全部 reviewed guards | 通过 |
| 嵌入式正式输入加载与 guard 运行时链路 | 通过 |
| `internal/cli`、`internal/pipeline` | 通过 |
| `make build` | 通过 |
| `check-generated-drift.sh` | 通过；两轮生成一致，Schema assembly determinism 通过 |
| `check-schema-catalog.sh` | 通过；28 个产品、1,175 个工具 |
| 完整 `internal/app` | 仅 1 个失败测试；无新增非目标回归 |

完整应用回归的唯一失败为
`TestCrossPlatformCoverageReviewedParamAliasesHaveCompleteTemplatesAndRepresentativeFinalPayloads`：
252 个模板覆盖 297 个 active 命令、610 个 active case，尚缺 45 个命令。失败与各产品报告的
落地前置一致，没有发现第二项候选引入的回归。

缺模板命令如下：

- Report（7）：`report +outbox-list`、`report entry get`、`report entry stats`、
  `report entry submit`、`report inbox list`、`report outbox list`、`report template get`；
- Ding（6）：`ding +list`、`ding +recall-personal`、`ding +send-personal`、
  `ding message recall`、`ding message send-by-message`、`ding message send-personal`；
- Event（6）：`event +listen-im`、`event consume`、`event list`、`event schema`、
  `event status`、`event stop`；
- Markdown（5）：`markdown create`、`markdown diff`、`markdown fetch`、
  `markdown overwrite`、`markdown patch`；
- AI Search（4）：`aisearch`、`aisearch behavior`、`aisearch enterprise`、`aisearch person`；
- Whiteboard（2）：`whiteboard query`、`whiteboard update`；
- Audit（3）：`audit export`、`audit tail`、`audit verify`；
- PAT（2）：`pat browser-policy`、`pat chmod`；
- HRBrain（10）：`hrbrain profile career`、`hrbrain profile labels`、
  `hrbrain profile metadata`、`hrbrain profile performance`、`hrbrain profile query`、
  `hrbrain search employees`、`hrbrain search employees-structured`、
  `hrbrain talent-pool detail`、`hrbrain talent-pool employees`、`hrbrain talent-pool list`。

Devdoc 已被现有模板覆盖；Live 和 MCP 只有 fail-closed guard，没有 active alias，因此不新增模板。

## 合理性判断与落地状态

### 可以认可的部分

- alias 只处理同一值原样透传的 argv 拼写，不承担 ID 转换、单位换算、结构转换或意图猜测；
- 宽泛词通过精确 command override 或 concept command scope 收窄；
- 不同值域、分页模型、输入/输出方向和敏感参数使用 block/ambiguous fail closed；
- 无业务参数的 Live、MCP 保持零 active alias，只补保护边界；
- 共享 concept 采用显式并集，没有后写候选覆盖先写候选；
- 最新 main 后续变更完整保留，非目标产品没有回归。

### 正式落地前的阻塞与配套项

1. 为上述 45 个命令补齐 `paramAliasCandidateCompleteCommands`/正式完整 payload 模板，随后重跑
   完整 `internal/app`；
2. 修复 Whiteboard 在 `NewSchemaSourceRootCommand()` fresh declaration-only 树中的可见性，确保
   不打临时补丁也能直接 `go generate ./internal/cli`；
3. 保留各产品报告中的独立契约后续：Ding Shortcut Identity、Markdown Skill diff、
   PAT `browser-policy --dry-run` 仍写本地策略、Report 发送人值域等；这些不能由别名表掩盖；
4. 完成前置后，在同一最新 main 隔离副本中重新执行生成、全部 fixture、完整 payload、
   `internal/app`、generated drift、Schema Catalog 与 Runtime confirmation truth；
5. 只有全绿后，才评审是否将本候选内容落入正式 `internal/cli/param_concepts.json`。

## 最终判断

该合并候选可以作为 12 产品统一落地评审的正确输入：内容完整保留最新 main，跨产品冲突已被
显式消解，规则层与仓库 Schema 政策均已验证。当前状态为**候选正确、落地条件未全部满足**，
不建议现在直接覆盖正式别名表。
