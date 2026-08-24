# Minutes、TODO、Wiki 联合参数别名表草稿与复审

> 状态：联合审核草稿，未替换正式 `internal/cli/param_concepts.json`，未生成或提交正式 Go 产物。

## 1. 结论摘要

本次以刷新后的最新 `origin/main` 提交 `aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 中的正式 `internal/cli/param_concepts.json` 为基线，只提取并叠加 Minutes、TODO、Wiki 三份产品草稿的产品内增量，生成联合草稿：

- [联合 param_concepts.json](./param_concepts.json)
- 基线正式表：52 个 concepts、241 条 command override、533 条 validation fixture。
- 联合草稿：62 个 concepts、349 条 command override、676 条 validation fixture。
- 净增量：10 个 concepts、108 条 command override、143 条 validation fixture。

结构化复核确认：

1. 三个产品的 concept 作用域、command override 和 fixture 与各自 Worktree 草稿逐字段一致。
2. 移除 Minutes、TODO、Wiki 增量后，联合草稿与最新 main 正式表逐字段完全一致，没有修改、删除或回退其他产品规则。
3. 三个产品之间没有 command path 冲突、fixture 主键冲突、concept 元数据冲突或跨产品规则串扰。
4. 最新 main 上 `go generate ./internal/cli` 成功，生成 705 个命令规则；676 条 fixture 和全部 guard 运行时验证通过。
5. 已补齐联合草稿所需的 53 个完整 payload 模板：Minutes 20 个、TODO 26 个、Wiki 7 个。最新 main 隔离验证达到 253/253 个 active command、456 条 active alias fixture 全覆盖，联合草稿已具备替换正式表后继续走完整仓库门禁的测试基础。

## 2. 为什么不能直接拼接三个完整 JSON

三份草稿使用的正式表基线并不完全相同：

- Minutes 草稿保留了较早基线，并带有后来 Calendar 规则；
- TODO 草稿已重叠到更新后的正式表；
- Wiki 草稿基于更新的 main 重建；
- 当前工作区分支还额外带有一条未进入最新 main 的 `chat group-role set-user` block。

直接选择任意一份完整草稿作为底稿再覆盖另外两份，会回退其他产品规则或带入过期规则。本次采用的合并方式是：

1. 从最新 `origin/main` 读取正式表；
2. 对既有 concept 只追加以 `minutes `、`todo `、`wiki ` 开头的命令作用域；
3. 只新增产品专属 concept；
4. 只新增对应产品前缀的 command override；
5. 只新增对应产品前缀的 validation fixture；
6. 对非目标内容做反向剥离比较，要求与最新正式表完全相等。

复审过程中实际验证了直接沿用当前工作区正式表会失败：最新 main 中 `chat group-role set-user --role-id` 已是原生真实 flag，旧分支中的同名 block 会被生成器拒绝。因此联合草稿必须基于最新 main，而不能机械继承当前旧分支的整张正式表。

## 3. 三个产品的改动汇总

| 产品 | 新增专属 concept | 扩展既有 concept | 新增 override | 新增 fixture | 最新 main 生成结果 |
|---|---:|---:|---:|---:|---|
| Minutes | 4 | 7 | 49 | 61 | 59 commands / 274 aliases / 476 blocked / 3 ambiguous |
| TODO | 6 | 4 | 23 | 72 | 41 commands / 136 aliases / 498 blocked / 3 ambiguous |
| Wiki | 0 | 6 | 36 | 10 | 36 commands / 292 aliases / 426 blocked / 45 ambiguous |

TODO 原报告中的 `436 blocked` 与当前最终草稿在最新生成器上的结果不一致；重新单独安装 TODO 增量后得到的权威值是 `498 blocked`。联合安装仍为 498，说明该变化不是三个产品合并导致的串扰，而是原报告中的量化数据已经过期；TODO 草稿规则本身未被联合过程改写。

### 3.1 Minutes

新增 4 个专属 concept：

- `minutes_task_uuid`：单个听记 taskUuid；
- `minutes_task_uuids`：听记 taskUuid 列表；
- `minutes_hot_words`：热词列表；
- `minutes_upload_session_id`：上传会话 ID。

复用并扩展 `search_query`、`pagination_size`、`page_cursor`、`content_text`、`time_start`、`time_end`、`user_ids`。单值/列表、taskUuid/异步 taskId、上传 session、模板、标签和用户身份值域保持分离；`permission` 与数字 `policy`、本地文件路径与上传元数据等需要值转换或 IO 的场景继续 block，不伪装成 name-only alias。

### 3.2 TODO

新增 6 个专属 concept：

- `todo_task_id`：单个待办 taskId；
- `todo_due_time`：截止时间；
- `todo_executor_ids`：执行人 userId 列表；
- `todo_participant_ids`：参与人 userId 列表；
- `todo_completion_state`：布尔完成状态；
- `todo_role_types`：查询角色列表。

复用并扩展 `search_query`、`pagination_size`、`page_number`、`content_text`。taskId 与 parent/comment/attachment ID、截止时间与提醒时间、执行人与参与人、姓名与 userId、单值优先级与过滤列表保持分离；不做时间单位转换、身份解析、JSON 构造或单复数值转换。

### 3.3 Wiki

不新增 concept，复用 `workspace_id`、`search_query`、`pagination_size`、`page_cursor`、`plain_description`、`user_ids`，并通过 36 条精确命令 override 处理 node、folder、workspace、Drive storage space 和成员角色。

知识库 workspaceId、Drive 数字 spaceId、Wiki nodeId、目标 folderId 和成员 userId 没有被合并为同一实体；多 ID 角色命令的裸 `id` 继续 block 或 ambiguous。规则保持保守，不将空间名称、节点名称或用户姓名猜成 ID。

## 4. 与三个 Worktree 的对齐结果

主目录中的三份源草稿与各自 Codex Worktree 文件 SHA-256 完全相同：

| 产品 | Worktree | concept 投影 | override 投影 | fixture 投影 |
|---|---|---|---|---|
| Minutes | `/Users/hyz/.codex/worktrees/412e/dingtalk-workspace-cli` | 11/11 项一致 | 49/49 一致 | 61/61 一致 |
| TODO | `/Users/hyz/.codex/worktrees/21e4/dingtalk-workspace-cli` | 10/10 项一致 | 23/23 一致 | 72/72 一致 |
| Wiki | `/Users/hyz/.codex/worktrees/3af1/dingtalk-workspace-cli` | 6/6 项一致 | 36/36 一致 | 10/10 一致 |

这里的“对齐”是指参数别名表语义完全对齐。Minutes Worktree 还存在别名表之外的命令拥有层改动，例如严格解析 `--url`、约束 `+transcript --id/--keyword` 互斥；这些内容不能写进 JSON 联合草稿，如需获得对应能力，必须单独迁移并按最新 main 适配。

本轮已经在主工作区补入三产品共 53 个候选 complete-command 模板、17 个最终 payload 代表 case 和 4 个确认边界 case。测试仅在联合草稿中的精确 fixture 生效时启用，因此可以先于正式表落地；当前正式表状态下不会改变既有模板或代表 case 的覆盖计数。

## 5. 合理性与正确性复审

### 5.1 已确认合理的部分

- 新增的 10 个 concept 都是产品专属实体，没有与正式 concept 重名。
- 三个产品复用共享 concept 时，`members`、`excludes`、`denotes`、`canonical_hint` 和 `risk` 与最新正式表一致，只增加精确命令作用域。
- 所有 alias 都是名称归一，值原样传递；没有混入身份查询、单位换算、URL 解析、文件 IO、JSON 构造或单复数数据转换。
- 单值/列表、来源/目标、资源/子资源、名称/ID、workspace/Drive space/node/folder 等边界有明确 block 或 ambiguous 保护。
- 108 条新增 override 的命令路径均能绑定最新 main 的 runnable Cobra leaf，alias 目标均为当前真实 flag。
- 三个产品的命令前缀互不交叉，联合后各产品生成结果与单独安装对应增量时一致。

### 5.2 仍需明确或另行修复的边界

1. **Minutes 的代码层改动未进入联合 JSON。** 当前 main 仍不具备 Worktree 中的严格 URL 提取和 `+transcript` 双路由互斥行为；只替换别名表不能获得这两项能力。
2. **TODO 有真实 flag/Schema/Skill 问题不能由表修复。** 包括部分叶子继承但不消费的隐藏 `--remind-at`、不可用的隐藏 `+upload-attachment` 契约以及 Skill 参数描述漂移。
3. **Wiki 有真实隐藏 flag 未消费问题。** 原生分页命令的 10 个隐藏分页入口和成员命令的 15 个隐藏 user 入口已经是 Cobra 真 flag，PreParse 必须避让；它们需要在命令拥有层修复。
4. **Wiki `feed list --id` 政策不完全一致。** 原生 `wiki feed list` 将 `id` 精确映射为 `workspace`，Shortcut `wiki +feed-list` 则 block `id`。当前 Shortcut 规则是保守且安全的，不会误执行，但两个入口都只有 workspace 范围角色，正式落地前应选择统一兼容，或在评审中保留差异理由。
5. **`public=false` Shortcut 仍不在中央治理范围。** 例如 `minutes +minutes-search` 不属于生成器认可的 runnable path，联合草稿没有伪造作用域。

因此，本表在“中央参数名归一化规则”范围内是合理、正确且没有跨产品回归的，所需永久 payload 模板也已经补齐。上述命令拥有层问题不应伪装成别名表能力；它们不阻断本次参数拟合草稿进入正式替换和完整仓库门禁阶段，但需要作为独立边界保留在评审说明中。

## 6. 验证记录

所有验证均在 `/private/tmp` 的最新 `origin/main` 隔离副本中进行，没有替换主工作区正式表，也没有触发真实业务调用。

| 验证 | 结果 |
|---|---|
| `jq empty` | 通过 |
| 产品投影与三个 Worktree 草稿逐字段比较 | 通过 |
| 剥离三产品增量后与最新 main 正式表比较 | 通过，完全相等 |
| `go generate ./internal/cli` | 通过，commands=705 |
| 连续生成确定性 | 通过，生成文件哈希不变 |
| `go test ./internal/cli ./internal/pipeline` | 通过 |
| 全部 validation fixture 经 embedded delivery / PreParse | 通过 |
| 全部 reviewed guard 到运行时契约 | 通过 |
| complete-command payload 模板门禁 | 通过，253/253 个 active command、456 条 active alias fixture |
| 三产品新增最终 payload 代表 case | 通过，17/17；alias 与 canonical transport payload 等价 |
| 三产品新增确认边界代表 case | 通过，4/4；移除 `--yes` 后均在零 transport call 前停止 |
| `go test ./internal/shortcut/minutes ./internal/shortcut/todo ./internal/shortcut/wiki` | 通过 |
| `go test ./internal/app`（联合草稿） | 通过；需允许 `httptest` 监听本机临时端口 |
| `go test ./internal/app`（当前正式表） | 通过；证明候选测试代码未改变正式状态行为 |

## 7. 后续落地顺序

1. 将联合草稿替换到正式 `internal/cli/param_concepts.json`，重新生成 `param_aliases_generated.go`。
2. 运行 generated drift、Schema policy、完整仓库测试和非目标产品回归，确认正式落地状态与本轮隔离验证一致。
3. 将 Minutes Worktree 的 URL、互斥约束按最新 main 重新评估；如要实施，作为命令契约改动单独复审，而不是混入别名表提交。
4. 对 TODO/Wiki 的真实隐藏 flag 未消费问题单独立项；不要用 `param_concepts.json` 覆盖真实 Cobra flag。
5. 决定 `wiki +feed-list --id` 的长期兼容政策；当前保守 block 可继续保留。
