# DWS OA CLI 参数幻觉分析

> 状态说明（2026-08-21）：本文件保留 2026-08-11 的原始事实盘点和问题证据；其中 concept 数量、
> concept 名称及落位方案已被同目录 `oa_cli_param_hallucination_review_20260821.md` 与最新候选
> `param_concepts.json` 取代。最新候选基线为线上 main `11934eed057267d97e7442ddd420c711ee1802dc`，
> `form-values` 与 `request` JSON 均作为同一局部端点的模式边界处理，不是中央 concept。

## 1. 结论摘要

本报告以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，仅分析当前 OA 产品的真实 Cobra Help、同提交运行时组装 Schema、OA Skill、官方命令树、审核排除项、命令实现和正式 `internal/cli/param_concepts.json`。没有使用历史 badcase、`dws-eval`、`merged_scan.json`、历史工作簿、固定 Catalog、用户自定义 Shortcut 或插件。

OA 不是简单的“给几个参数补别名”问题。它同时存在四类审批标识符、五类以上人员角色、两套 JSON 请求模式、两套分页协议，以及公开命令、Schema exclusion、隐藏 Shortcut、原生兼容 flag 之间的可发现性边界。如果模型把这些近似参数统一写成 `--id`、`--user-id`、`--payload` 或 `--page-size`，有些会直接报 unknown flag，有些会被安全拦截；更需要注意的是，少数真实 flag 虽然能被 Cobra 接受，却没有进入最终 RPC payload。

量化结果如下：

- 官方命令树共 35 个 runnable 路径，其中 32 个可见、3 个隐藏；32 个可见路径包含 30 个带业务参数的 leaf 和 2 个可执行父命令。
- 运行时 Schema 发布 25 个 OA 工具；另有 5 个公开 runnable leaf 被 `schema_command_exclusions.go` 精确排除：`search-forms`、`ding-info`、`revert-activities`、`append-task`、`revert-task`。
- 33 个参数化官方路径合计出现 92 次 canonical 业务参数，使用 30 个不同 canonical flag 名；还有 6 次原生隐藏 flag，涉及 5 个不同名称。
- 25 个 Schema leaf 的公开业务参数与真实 Help 差异为 0。Skill 机械扫描产生的 16 条“未知命令”告警均来自上述公开 exclusion 命令，逐项复核后真实 Skill 参数漂移为 0。
- 聚合得到 7 类参数问题、74 条“问题—命令”明细，覆盖全部 33 个参数化官方路径。
- 正式基线只为 OA 生成 3 个命令条目、15 个 alias、10 个 block、0 个 ambiguous；候选草稿扩展到 30 个命令条目、340 个 alias、198 个 block、23 个 ambiguous。
- 候选相对正式文件新增 18 个 OA concept、扩展 7 个既有 concept 的 OA 命令范围、新增 18 个 OA command override；既有非 OA override、morphological rules 和 253 条 fixture 均未变化。
- 候选已通过 19 组 alias/canonical 最终 payload 等价、9 组 block/ambiguous、2 组有效原生兼容、4 组非 OA 回归、三包测试和两项政策门禁。

第一轮建议落地“同实体、同角色、同 cardinality、值可原样传递”的参数名归一，并保留强保护。候选已经具备技术落地条件，但仍应由 OA owner 审核 340/198/23 的完整影响面；实现未消费或值校验错误的真实参数需要另开代码修复，不能靠别名表伪装成已解决。

## 2. 七类参数问题

### 2.1 审批实例、任务、模板和流程节点标识符容易被统一写成 `id`

OA 中至少存在四种不同标识符：

- `--instance-id`：审批实例 `processInstanceId`；
- `--task-id`：审批任务 `taskId`；
- `--process-code`：审批模板 `processCode`；
- `--target-activity-id`：退回目标流程节点 `activityId`。

17 个命令涉及其中至少一种。`detail`、`ding-info` 这类 leaf 只有一个 ID 角色，可以在精确命令范围把 `--id` 安全归一到唯一目标；`approve`、`reject`、`append-task`、`revert-task` 同时有两到三种 ID，`--id` 没有唯一答案，必须返回 ambiguous。四类标识符之间不能只改名互传。

另外，`approve` 和 `reject` 会把不可解析的字符串 `task-id` 静默转成数字 `0`，而其他 task leaf 仍把它作为字符串传递。这属于实现层值类型/校验缺口，不是 concept 能修复的参数名问题。

### 2.2 审批参与人参数混合单值、多值和不同业务角色

代表性参数包括：

- 发起人单值：`--originator-user-id`；
- 审批人列表：`--approvers`；
- 抄送人列表：`--cc-list` / `--users`；
- 转交接收人单值：`--to-actioner-id`；
- 加签人列表：`--appender-user-ids`；
- 操作人单值：`--operator-id`。

这些参数可能都承载 userId，但角色和单复数不同。候选分别建立 originator、approver list、cc list、redirect target 和 appender list concept；在 `create-instance` 这类多角色 leaf 中，宽泛 `user/user-id/users/user-ids` 统一返回 ambiguous。不能把单值自动扩成列表，也不能根据“都是 userId”交换业务角色。

`oa-cc-noticer --operator-id` 是特殊缺口：Help 和示例声明了该参数，但 RunE 没有读取它，dry-run 最终 payload 只有 `processInstanceId` 与 `userList`。候选因此主动移除了 operator concept 和任何 operator 别名扩展。

### 2.3 简单表单参数与完整 `request` JSON 是两套互斥协议

`create-instance` 和 `forecast-process` 同时支持：

- 简单模式：`--process-code`、`--form-values`、`--dept-id` 等；
- 高级模式：`--request` 完整 JSON。

两者都表现为字符串形式的 JSON，但结构、必填关系和接口模式不同。候选仅在该精确端点内允许 `form-values-json/form-data-json` 到 `--form-values`、`request-json/request-body-json` 到 `--request` 的原样改名，不建立中央 JSON concept；`data/payload/body` 无法判断应该选择哪种协议，必须 ambiguous。别名层不会拆分、合并、包装或补充 JSON 字段。

### 2.4 审批意见、评论正文和动作枚举共享相似词根

9 个命令涉及以下相邻语义：

- 审批决定、撤销、转交、退回说明：`--remark`；
- 审批评论正文：`--content`；
- 加签位置：`--type`；
- 任务激活模式：`--activate-type`；
- 全员同意字符串：`--agree-all`；
- 退回动作：`--action`；
- 审批聚合模式：`--approvers-action-type`；
- 抄送时点：`--cc-position`。

候选只在精确 leaf 内把 `comment` 等名称归一到唯一正文/说明目标，并把各枚举角色拆成独立 concept。参数值保持原样；别名层不会把中文枚举翻译成英文，也不会把 `agree-all` 的字符串值改造成 bool flag。

### 2.5 列表命令同时存在 `page` 与 `cursor` 两套分页并混用时间格式

14 个命令涉及分页或时间：

- `list-cc/executed/pending/submitted` 使用 `page + limit`；
- `list-forms/list-initiated` 使用 `cursor + limit`；
- 稳定 pending/initiated leaf 的 `start/end` 接 ISO-8601；
- 隐藏 `+list-pending` 的 `start/end` 接毫秒时间戳。

候选可以在同一协议内处理 `page-no/page-size/current-page` 或 `page-token/next-token/per-page`，并在相反协议上 block。它不能把 page 换算成 cursor，也不能把 ISO-8601 转成毫秒。

三个原生隐藏 flag 还存在实现缺陷：`list-forms --size`、`list-initiated --max-results` 和 `--next-token` 会被命令接受，但由于 fallback 先读取有默认值的 `limit/cursor`，最终 payload 仍保持 100、20 和 0。中央归一化不会覆盖已经存在的真实 flag，因此必须使用公开 `--limit/--cursor`，并由命令实现另行修复。

### 2.6 搜索词 `query` 与审批对象名称/正文容易被宽泛文本参数混用

12 个命令涉及 `query/keyword/comment`。公开列表和表单搜索可以复用 `search_query`，支持 `q/keyword/search-word` 等明确查询词名称；`name/title/text/subject` 可能表示对象名称或正文，不应无条件改成 query。

隐藏 `oa +approve-by` 使用 `keyword + comment`，当前不进入中央 alias reducer，候选保留其 canonical 行为，不伪造已治理结论。

### 2.7 Schema exclusion、隐藏 Shortcut 和真实但无效 flag 构成可发现性边界

可执行、Agent Schema 可发现、Cobra 接受和最终 payload 生效是四种不同事实：

- 25 个 OA 工具进入 runtime Schema；
- 5 个公开 leaf 可执行但被精确 exclusion；
- 3 个隐藏内置 Shortcut 可执行但不属于公开面；
- 6 次隐藏 flag 中，`text` 和 `user-list` 有效，`size/max-results/next-token` 的三个实例无效；
- 公开 `operator-id` 可解析但未被实现消费。

候选只覆盖 30 个公开且带业务参数的 leaf，不改变 Schema exclusion，不把隐藏 Shortcut 伪装成公开治理对象，也不为 no-op 真实 flag 增加新别名。

## 3. 当前别名表可以直接实施的方案

候选完整文件位于同目录 `param_concepts.json`，主要改动如下：

1. 新增 18 个 OA 专用 concept：实例、任务、模板、流程节点、五类人员角色、两类 JSON、审批说明，以及 append/revert/create 的动作和模式参数。
2. 扩展 `search_query`、`pagination_size`、`page_number`、`page_cursor`、`time_start`、`time_end`、`content_text` 七个既有 concept，仅追加 OA 精确命令范围，不改成员或 excludes。
3. 新增 18 个 command override，用于绑定宽泛参数、限定命令级别名，以及配置 `scope_strict`、block 和 ambiguous。
4. 30 个公开参数化 leaf 均进入候选生成结果；3 个隐藏 Shortcut 不进入。

生成影响面由正式基线的 3/15/10/0 扩展为 30/340/198/23（命令/alias/block/ambiguous）。这个数字较大，原因不是向全仓库扩大范围，而是分页、时间、搜索和四类 ID concept 的成员/excludes 会在每个适用 OA leaf 上展开。独立审核逐条验证：

- 30 个生成路径全部存在于同提交官方树，且都是公开参数化 leaf；
- 340 个 alias 的目标全部是该 leaf 的公开 canonical flag；
- alias 来源均不是该 leaf 已有真实 flag；
- 198 个 block 和 23 个 ambiguous 均未拦截真实 flag；
- 非 OA concept 内容、既有 override、morphological rules 和 253 条 fixture 均未改变。

因此候选规则在技术和语义准入条件上成立，建议由 OA owner 对完整 diff 做最终业务评审后落地，而不是直接用数量增长作为通过依据。

## 4. 当前能力支持不了或不应该自动处理的事项

以下事项不能通过 `param_concepts.json` 解决：

- processInstanceId、taskId、processCode、activityId 之间的查询或转换；
- 人员姓名、部门名、表单名到稳定 userId/deptId/processCode 的解析；
- 单个 userId 与人员列表之间的自动扩展/收缩；
- `form-values` 与完整 `request` 的 JSON 结构转换；
- page/cursor 换算和 ISO-8601/毫秒转换；
- 修复 `oa-cc-noticer --operator-id` 未被 RunE 消费；
- 修复 `--size/--max-results/--next-token` 被默认 canonical 值遮蔽；
- 修复 approve/reject 将非法数字 task-id 静默转成 0；
- 让 3 个隐藏 Shortcut 进入中央 reducer；
- 让 5 个审核排除命令自动进入 Agent Schema。

这些问题不阻塞第一轮安全的参数名治理，但前三类真实参数实现缺口应单独建代码修复，避免用户误以为“Help 能解析即参数生效”。

## 5. 候选草稿验证结果

候选仅在冻结快照 `/private/tmp/dws-main-param-analysis.HcTfUP` 中临时替换正式输入并重新生成、构建和测试；当前工作区正式 `internal/cli/param_concepts.json` 与 `param_aliases_generated.go` 均未修改。

验证结果：

- JSON 结构和候选范围审核通过；
- `go generate ./internal/cli` 两次结果一致，生成 308 个全局命令条目；
- 19 组 alias/canonical 最终 dry-run payload 或 Shortcut mock 结果完全等价；
- 9 组 block/ambiguous 均在 dispatch 前停止；
- 2 组有效原生兼容 `text/user-list` 保持正常；
- 4 组 Calendar、Doc、Drive、Mail 非目标 alias 最终 payload 未变化；
- `internal/cli`（131.083s）、`internal/pipeline`（0.502s）、`internal/app`（198.724s）全部通过；
- `check-generated-drift.sh` 通过；
- `check-schema-catalog.sh` 通过，最终仍为 27 个产品、1018 个工具；
- Schema assembly determinism、runtime confirmation truth 和 homology 检查通过。

已专门验证并保留三类负面证据：公开 `operator-id` 被忽略、三个隐藏分页 flag 被默认值遮蔽、approve/reject 非数字 task-id 变成 0。它们被列为“当前无法解决”，没有被候选表描述成已兜底。

## 6. 第一轮改造建议

1. 先合入 18 个 OA concept、7 个既有 concept 的 OA scope 和 18 个精确 override；不修改真实 CLI flag。
2. 将完整 generated diff 交给 OA owner 重点审核 `create-instance`、`append-task`、`revert-task`、`list-pending`、`list-initiated` 五个高影响 leaf。
3. 单独修复 `operator-id`、三个 no-op hidden pagination flag 和 approve/reject task-id 校验，并补最终 payload 回归测试。
4. 单独评审 5 个 `schema_command_exclusions.go` 项是否仍应排除；这与参数别名治理分开提交。
5. 在正式替换前，把本报告中的 19/9/2/4 行为集转为仓库长期测试，保证后续命令实现变更不会使 alias 静默失效。

## 7. 可复用到其他产品的流程

1. 冻结线上 main commit，并从该提交重建临时二进制；
2. 合并 runtime Schema、官方命令树、exclusion、内置 Shortcut 和可执行父命令形成全量清单；
3. 逐 leaf 对账 Help、完整 Schema 和 Skill；
4. 按业务实体、角色、值域、cardinality、单位和协议聚合问题；
5. 只有值可原样传递且目标唯一时配置 alias，否则使用 block/ambiguous 或列为不支持；
6. 基于正式 `param_concepts.json` 生成完整产品候选，不覆盖工作区正式文件；
7. 独立审核真实 flag 冲突、目标有效性、命令范围和非目标 diff；
8. 在隔离副本验证生成确定性、最终 payload、保护、原生兼容、非目标回归、包测试和政策门禁；
9. 报告同时交付可落地方案与真实实现缺口，不能用“命令可解析”替代最终 payload 证据。
