# DWS TODO 产品 CLI 参数幻觉分析（2026-08-18）

## 结论摘要

本轮分析基于任务创建后最新的 `origin/main`：`c15480c452ccd607e0694a5798ce7ed37d62539b`。分析开始时已将当前 worktree 切到该提交，并以清空后的独立 Go 构建缓存执行 `make build`；得到的 `./dws` 为 29,649,634 bytes，构建时间为 `2026-08-18T20:53:51+0800`。运行时 Schema 报告 `source=runtime-assembled`、27 个产品、1,152 个工具，其中 TODO 44 个工具；Catalog hash 为 `sha256:2245e552feb409b1f63c8cb5c63f423e9a050eec081fbcce58dac4f4cf54313e`，surface hash 为 `sha256:772696e1d8e146f6a3168b91b8c7e1ff010218e4da7daf42b71d84a26efd0e7d`。

2026-08-19 修订时，`origin/main` 已推进到 `13d0ae66a6d570deeaa7f5c5cb875ca74825b404`；TODO Cobra、Shortcut、Skill 和 exclusion 均未变化，但正式参数词典已增加其他产品规则。为避免候选覆盖新规则，本次修复将同一组 TODO 增量重新叠加到 `13d0ae66…` 的正式表并完整复验。最新叠加结果组装为 27 个产品、1,153 个工具，Schema Catalog 门禁通过；任务事实与最初构建仍以创建时基线 `c15480c…` 为准。

当前 TODO 官方 Cobra 树有 50 个可执行叶子：45 个公开业务叶子、1 个隐藏 Shortcut（`todo +upload-attachment`）和 4 个隐藏顶层兼容提示叶子。46 个业务叶子由 25 个原生命令和 21 个内置 Shortcut 组成；Runtime Schema 收录 44 个工具，即 23 个原生命令和全部 21 个 Shortcut。`todo task list-sub`、`todo task remove-attachment` 是两个精确 Schema exclusion。44 个 Schema 叶子的可见参数与同提交 Cobra Help 逐命令比对，差异为 0。

相较 2026-08-10 的历史产物，当前公开业务命令由 36 增至 45，公开 Shortcut 由 11 增至 20，TODO Schema 工具由 34 增至 44。重点新增的 `+create`、`+update`、`+complete`、`+reopen`、`+search`、`+comment`、`+reminder`、`+upload-attachment` 已全部复核；历史文件仅用于确认这些变化，未作为当前参数事实。

结论分三层：

- 可以由现有 PreParse 做安全同义归一的，是同一命令、同一业务角色、同一值域、同一单位、同一单复数且值可原样传递的拼写，例如 `todo-id → task-id`、`deadline → due`、`executor-ids → executors`、`current-page → page`。
- 需要 block 的，是名称相似但角色、结构、单位或执行意图不同的输入，例如 taskId 与标题查询、人员姓名与 userId、deadline 与 reminder、ISO 时间与 Unix 毫秒、平面 tag code 列表与 JSON 对象数组、`max-pages` 与结果条数。
- 不能由别名表解决的，是需要查人、构造 JSON、换算单位、改变单值/列表、修复真实 Cobra flag 或修复 Schema/Skill 声明的事项。

已生成完整候选 [`param_concepts.json`](./param_concepts.json)，规则本身已通过最新正式表上的生成、PreParse、非目标回归和 Schema policy，但它当前仍只能作为审核草稿，**不能直接替换**正式表。主要阻塞是 26 个 active TODO complete-command payload 模板缺失；此外需要修复隐藏 `+upload-attachment` 的 Schema 可用性、清理/拒绝继承的 `--remind-at`，并更新 TODO Skill。

## 分析边界与当前事实

- Cobra：直接遍历 `app.NewSchemaSourceRootCommand()` 生成的真实命令树，记录本地 flag、继承的 TODO 持久 flag、隐藏性和可执行性。
- Schema：使用本轮新构建的 `./dws schema --all -f json` 运行时组装结果；未使用提交态固定 Catalog。
- Shortcut：读取全部仓库内置 TODO Shortcut 和 `semantic_catalog_todo.json`，包括 `public=false`。
- Exclusion：读取 `internal/cli/schema_command_exclusions.go` 的精确路径，未使用前缀或通配推断。
- Skill：完整读取共享 Skill、TODO Skill 及 TODO 命令参考；Skill 只作为 Agent 暴露面和文档漂移证据，不覆盖 Cobra/Contract 事实。
- 参数词典：初始分析以 `c15480c…` 的正式表为基表；修订候选已重新叠加到 `13d0ae66…` 的最新正式表。两轮验证均在独立临时 worktree 内进行，当前 worktree 的正式输入未被替换。
- 未纳入：历史固定 Catalog、历史候选中的当前事实、用户自定义 Shortcut、插件命令、真实业务实验调用。

## 参数问题与治理结论

### 1. taskId、父任务、评论和附件标识角色混淆

`--task-id` 是单个待办 taskId；`task create-sub --parent-id` 是父任务角色；`comment delete --comment-id` 和 `task remove-attachment --attachment-id` 是不同实体。裸 `--id/--ids`、复数 `--task-ids/--todo-ids` 或把子资源 ID 当 taskId 都可能命中错误对象。候选仅在具有单一 taskId 角色的精确命令上允许 `todo-id → task-id`，并对 `task create-sub` 使用命令级 bind，把单值 `task-id`、`todo-id`、`parent-task-id` 收敛到 `parent-id`；裸 ID、复数 taskId、`comment-id`、`attachment-id` 均显式 block。特别是 `task-ids` 与真实 `task-id` 只差一个字符，若不保护会被通用模糊纠错静默改写并把逗号串当作一个 taskId，本候选已增加回归 fixture 阻止该路径。

代表命令：`todo +complete`、`todo +reopen`、`todo +comment`、`todo task get`、`todo task create-sub`、`todo comment delete`、`todo task remove-attachment`。

### 2. 标题写入与标题查询共用 task/title 词形

`task create`、`+create/+assign/+assign-multi/+remind` 的值是新任务标题；`task update`、`+update` 同时存在目标 taskId 和替换标题；`task create-sub` 同时存在父任务 taskId 和子任务标题；`+todo-done --task`、`+search --query` 则是标题查询。候选不再创建全局 `todo_task_title` concept，而是在 5 个纯创建场景用精确 override 映射 `task ↔ title`；对两个 update 和 create-sub 的裸 `task` 标记 ambiguous 并在执行前停止。`+todo-done` 的 `query/keyword/title` 映射到真实 `--task`，`+search` 的 `subject/title/task/keyword` 映射到 `--query`，同时 block `+todo-done` 的 taskId 拼写。

### 3. 人员姓名、userId、执行人和参与人混淆

原生命令的 `--executors/--participants` 传 userId 列表；`+assign --to` 传一个姓名，`+assign-multi --to` 传姓名列表并在运行时解析为 userId。候选只为 userId 列表提供 `executor-ids → executors`、`participant-ids → participants`，并为两个 Shortcut 分别提供精确的 `assignee-name → to`、`assignee-names → to`；单数 `executor-id/participant-id`、交叉角色、`executors/participants` 与 `user-id(s)` 在不匹配的精确命令上显式 block。另增加 `roles → role-types`，仅覆盖 4 个接受 creator/executor/participant 列表的 TODO 查询命令。姓名到 userId 需要查询，不能伪装成改名；单姓名和列表也不能互换。

### 4. 截止时间、提醒时间、偏移量和提醒规则混淆

`--due` 是任务 deadline；`+remind --at` 实际仍写 deadline；`task add-reminder --reminder-time-stamp` 与 `+reminder --at` 是独立 customTime 提醒；`--due-date-offset` 是分钟偏移；`task reset-reminder --reminder-rules` 是 JSON 数组。候选允许 deadline 同义词归一到 `due`，允许一个 ISO customTime 在 `at` 和 `reminder-time-stamp` 之间原样映射，但交叉 block deadline、提醒、偏移量和 JSON 规则。

真实 Cobra 仍在 17 个 `todo task` 叶子上继承隐藏 `--remind-at`；只有 create、create-sub、update 会显式拒绝，其余叶子可能接受后忽略。因为它是真实 flag，PreParse 无法 block，必须在命令框架层移除、缩小挂载范围或在全部叶子一致拒绝。

### 5. 完成状态、固定动作和上游枚举混淆

`task done --status`、`task update --done`、`task list --status`、`+get-my-tasks --status`、`+search --status` 都使用字符串 `true/false`，可在这些精确命令上安全归一 `status/done/is-done/todo-status`。`+complete` 和 `+reopen` 的目标状态由命令固定，候选 block 这 4 种拼写，避免制造“仍可选状态”的假象。`+get-related-tasks --status` 透传上游 TODO-style 枚举，故只允许精确 `todo-status → status`，并明确 block 布尔词 `done/is-done`，不与布尔 concept 混用。

### 6. 分页大小、总量目标和遍历安全上限混淆

`comment list`、`+list-comment` 的 `--size` 是每页大小；`+get-my-tasks --size` 也是每页大小，且 `--all` 路径会删除 pageSize；`task list --size` 才是自动翻页后的总结果目标；`+search --max-pages` 是遍历安全上限。候选在评论列表保留常见分页大小同义词；`+get-my-tasks` 只允许 `page-size/per-page → size`，并 block `limit/max-result(s)/take/top`，避免把总量承诺悄悄降成每页大小；`task list` 仅允许总量词 `limit/max-results → size`。4 个 TODO 分页命令用精确 override 支持 `page-number → page`，不污染其他产品；`+search` block 所有条数/分页拼写。

### 7. ISO-8601 与 Unix 毫秒时间范围同名不同单位

`task list --plan-finish-date-start/end` 要求 ISO-8601；`+get-my-tasks --plan-finish-start/end` 要求 Unix 毫秒。任何 name-only alias 都不能完成单位转换。候选在两端互相 block 对方的词形；安全做法是保留真实 flag 和原单位，未来若要转换需显式 typed converter。

### 8. 优先级单值与列表值域不同

创建/更新的 `--priority` 是单值 10/20/30/40；`task list` 和 `+get-my-tasks` 的 `--priority` 是过滤列表。候选只在列表命令上允许 `priorities → priority`，不在写命令间传播复数拼写，也不处理文字优先级到数字的转换。

### 9. 标签编码列表、名称与 JSON 对象数组混淆

`tag add/delete --tag-codes` 是逗号分隔 code 列表，`tag create --name` 是单个名称，`tag update --user-tags` 是 JSON 对象数组。候选在 add/delete 上 block `tags/tag-ids/tag-names`，在 update 上额外 block `tag-codes`；别名表不构造 JSON，也不把名称或 ID 猜成 code。

### 10. 评论正文的 content/text/body 词形漂移

`comment add` 与 `+comment` 都只有一个正文角色，且值为原样字符串。复用现有 `content_text` concept，把 `text/body → content` 限定在这两个命令，属于低风险 name-only 归一。

### 11. Help、Schema、Shortcut Catalog 与 Skill 的公开契约漂移

参数集合的 Help/Schema 对账为 44/44、0 差异，但仍有三类非参数表问题：

- `+upload-attachment` 在语义目录中为 `public=false`、`availability=unavailable`，运行时固定报错并路由到 `todo task add-attachment`；Runtime Schema 却发布为 `availability=available`。候选不为它增加别名，需修复 Contract/Schema 可用性或不再组装该叶子。
- TODO Skill 的可见 Shortcut 表有 18 项，漏掉两个当前公开 Shortcut：`+complete`、`+reopen`；Skill 还声称 `--id/--ids` 是隐藏兼容别名，但当前 Cobra 没有这两个 flag。
- TODO reference 把 `task list --size` 写成每页数量、同时宣称不支持独立 reminder，又把 `tag update --user-tags` 说成与 `tag create` 同格式。这些都应按当前 Help/Schema/运行时更新。

这类问题不能靠 `param_concepts.json` 修复；否则只会掩盖源声明漂移。

## 候选 param_concepts.json

候选现已基于 `13d0ae66…` 的最新正式表重新生成，是完整正式表加 TODO 增量。结构化 diff 为 108 行：101 行新增、7 行修改；正式表与候选的规模如下：

| 项目 | 正式表 | 候选表 | 增量 |
|---|---:|---:|---:|
| concept | 52 | 58 | +6 |
| command override | 242 | 265 | +23 |
| validation fixture | 534 | 606 | +72 |

新增 concept 为 `todo_task_id`、`todo_due_time`、`todo_executor_ids`、`todo_participant_ids`、`todo_completion_state`、`todo_role_types`；同时只给既有 `search_query`、`pagination_size`、`page_number`、`content_text` 追加精确 TODO 命令范围。优先级列表只有两个命令复用，已改用命令级 `priorities → priority` override 并保留 `priority-id`/`priority-name` block，避免占用全表唯一的通用 `priority`/`priorities` concept 成员。标题兼容同样全部使用命令级 override，避免与最新正式表的 `calendar_event_title` 发生全局成员冲突。

真实生成结果覆盖 41 个 TODO 命令，得到 136 个 alias、436 个 block、3 个 ambiguous。3 个 ambiguous 分别保护 `todo +update`、`todo task update` 和 `todo task create-sub` 的裸 `task`。block 数量较大，是因为 concept 的 `excludes` 会按命令展开为防误映射保护；其中新增保护明确阻断 taskId 复数被通用模糊纠错收敛为单值，以及 executor/participant 单值与角色交叉。审核确认不存在目标 flag 缺失、不存在 alias 来源与当前真实 flag 的异义冲突，且相对 `13d0ae66…` 正式生成结果的非 TODO drift 为 0。隐藏 `+upload-attachment` 未纳入候选。

审核结论：本次修复后规则的业务方向合理，值均可原样传递；没有姓名查找、单位换算、单复数转换或 JSON 构造混入 alias。此前不安全的 update `task → title` 和 `+get-my-tasks limit → size` 已删除并改为拦截。但因 complete-command 模板门禁未补齐，候选状态仍是“规则可进入代码评审、测试前置未满足”。

## 验证结果

候选分别在 `c15480c…` 与 `13d0ae66…` 的独立临时 worktree 内替换正式输入；验证结束后两处正式输入和生成文件均已恢复为干净状态，当前 worktree 的 `internal/cli/param_concepts.json` 始终未修改。写命令只走 embedded fixture、dry-run 或注入 caller/Runner，没有发出真实业务写调用。

| 验证项 | 结果 | 说明 |
|---|---|---|
| 基线提交与新构建 | 通过 | 任务创建基线为 `c15480c…`，构建使用独立空缓存；修订时另在 `13d0ae66…` 复验 |
| JSON 读取与结构化 diff | 通过 | `jq empty`；最新正式表完整副本，仅 TODO 增量 |
| `go generate ./internal/cli` | 通过 | 最新正式表叠加后生成 610 个命令的参数别名表 |
| `make generate-schema` | 通过 | 两次组装哈希相同，determinism 通过；最新组装 27 产品/1,153 工具 |
| Help/Schema 对账 | 通过 | TODO 44/44 叶子，可见参数差异 0 |
| alias 目标与真实 flag 审核 | 通过 | 缺失目标 0；alias 来源异义真实 flag 冲突 0 |
| PreParse alias/block/ambiguous | 通过 | 全部 606 fixtures 经 embedded delivery/PreParse；TODO 136 alias、436 block、3 ambiguous；`task-ids` 不再进入模糊纠错 |
| 代表性最终 payload | 通过 | 注入 caller 验证 `+get-my-tasks` 每页语义、`+get-related-tasks` 枚举状态和 `+search` 布尔状态；无真实调用 |
| 非目标产品回归 | 通过 | 生成审计显示非 TODO drift 0；`internal/cli`、`internal/pipeline` 通过 |
| Shortcut 回归 | 通过 | 最新正式表叠加后 `internal/shortcut/todo`、`internal/shortcut/smart` 通过；基线轮次的 builtin 也通过 |
| 生成/参数/policy | 通过 | generated drift、param concepts、alias co-occurrence、runtime confirmation、Skill command integrity 均通过 |
| Schema policy | 通过 | 沙箱内回环端口受限；在获准环境同输入重跑通过，27 产品/1,153 工具 |
| complete-command 模板门禁 | **未通过** | 200/226 个 active 命令有模板；缺 26 个 TODO 模板，候选不能直接落地 |
| `go test ./internal/app` 全包 | **未通过（已定位）** | complete-command 门禁必然失败；相关 embedded PreParse、注入 payload、Schema policy 均已单独通过 |

缺失模板的 26 个命令为：`todo task get`、`todo task create-sub`、`todo task create`、`todo +create`、`todo +assign`、`todo +assign-multi`、`todo +update`、`todo +remind`、`todo task add-executor`、`todo task add-participant`、`todo task update`、`todo task done`、`todo +search`、`todo +get-my-tasks`、`todo +get-related-tasks`、`todo +due-today`、`todo comment add`、`todo +comment`、`todo comment list`、`todo +list-comment`、`todo task list`、`todo +todo-done`、`todo +reminder`、`todo task add-reminder`、`todo +complete`、`todo +reopen`。

## 当前别名表无法解决的事项

1. 姓名解析为 userId、单姓名扩展为列表或 executor/participant 角色转换。
2. ISO-8601 与 Unix 毫秒的解析和单位换算。
3. 原子 reminder 字段与 `reminder-rules` JSON 数组的构造/拆解。
4. tag name、tag code、tag ID 和 `user-tags` JSON 对象数组的转换。
5. `+todo-done` 的标题搜索结果定位、歧义消解和 taskId 选择。
6. 隐藏真实 flag `--remind-at` 的拦截；真实 Cobra flag 会先于“未知参数”治理存在。
7. `+upload-attachment` 的可用性/执行契约漂移，以及 Skill 可见表和参数文档漂移。
8. 用户自定义 Shortcut、插件或未来命令的自动命中；候选仅覆盖当前精确内置路径。

## 第一轮改造建议

1. 先补齐上述 26 个 complete-command payload 模板，并将本轮三组代表性注入 Runner 测试转为正式测试；模板应覆盖 alias 与 canonical 的同 payload，以及 block/ambiguous 在 dispatch 前终止。
2. 修复 `+upload-attachment`：优先让 Runtime Schema 保持 unavailable/hidden，或删除不可执行 Shortcut 的 Agent-visible Contract；继续引导使用 `todo task add-attachment --file`。
3. 将 `--remind-at` 从 `todo task` 持久 flag 缩小到确有兼容需求的叶子，或在所有继承叶子一致拒绝；补“不得静默忽略”的回归测试。
4. 更新 TODO Skill：可见表增加 `+complete/+reopen`，删除虚构的 `--id/--ids` 声明，修正 `task list --size`、独立 reminder 和 `tag update` JSON 说明，并补新增 Shortcut 的参数路由。
5. 补齐模板后完成 `DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/app`，再将候选提交为正式表变更；落地时同时提交重新生成的 `param_aliases_generated.go`。

## 可复用分析流程

1. 固定最新基线提交，以空构建缓存生成二进制并记录 hash、时间和大小。
2. 从真实 Cobra 树、运行时组装 Schema、精确 exclusion、全部内置 Shortcut 和 Skill 建立同提交清单。
3. 先按实体角色、值域、单位、单复数、结构和运行意图聚合问题，再决定 alias、block/ambiguous 或不支持。
4. 从正式词典制作完整候选；只追加目标产品精确路径，结构化审阅每一个来源 flag 和目标 flag。
5. 在独立 worktree 替换正式输入，跑生成、确定性、PreParse、payload、非目标回归和 policy；写操作只用 dry-run 或注入 Runner。
6. 将环境限制、真实门禁失败和产品契约漂移分别记录；只有完整测试和仓库模板门禁都满足后，才能把候选标为可直接落地。
