# CLI 参数幻觉分析跨产品汇总与落地状态

## 总结

本轮严格以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`（2026-08-20 10:53:27 +08:00）为统一事实
基线，依次完成 report、ding、event、markdown、aisearch、whiteboard、audit、pat、devdoc、
live、mcp、hrbrain 共 12 个产品的 CLI 参数幻觉分析。

每个产品均已在 `docs/parameter-hallucination/<product>/` 交付：

1. 独立中文 Markdown 分析报告；
2. 含“汇报总览、参数问题明细、兜底解决方案、当前无法解决、分析依据”五个中文工作表的 XLSX；
3. 基于冻结正式 `internal/cli/param_concepts.json` 的完整独立候选草稿。

共交付 36 个产品文件。12 份 XLSX 均核验为 5 个工作表、5 张结构化表，逐页完成视觉检查，
公式错误扫描为 0；所有检查 sidecar 已清理。

冻结正式表 SHA-256 为
`e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。全部生成、PreParse、
alias/canonical、block/ambiguous、非目标回归和仓库政策检查均在隔离 worktree
`/private/tmp/dws-param-analysis-aa4ae9a90323` 中进行。隔离副本最终恢复到冻结提交，`git status`
为空，正式输入与生成文件无 diff。

当前工作区在任务期间由其他工作推进到
`3d7ab2690c3bce837a1d4344ca92e4469a3df801`；当前正式表 SHA-256 为
`5e02331b579fe7392ef642dcb030a253e4fd4388afa49e5866ecd654bea9979f`。本轮没有修改、覆盖或回退
当前工作区的正式 `internal/cli/param_concepts.json`。

## 落地状态总览

| 产品 | 候选改动规模 | 隔离验证结论 | 落地状态 | 正式落地前置 |
|---|---|---|---|---|
| report | 3 个既有 concept 扩围；16 个 override；15 个 fixture | 生成、PreParse、payload、保护、回归、drift、Schema 政策通过 | 条件通过 | 补 7 个 complete-command 模板；复核 Skill 发送人值域 |
| ding | 7 个既有 concept 扩围；11 个 override；15 个 fixture | 运行链路与政策通过 | 条件通过 | 补 6 个模板；若治理 `+send-by-message`，先补 Contract/Identity 使其进入 reviewed source tree |
| event | 3 个新增 + 3 个既有 concept；6 个 override；30 个 fixture（22 active） | 生成、5 组 payload、保护、回归与政策通过 | 条件通过 | 补 6 个 complete-command 模板 |
| markdown | 5 个新增 + 6 个既有 concept；5 个 override；35 个 fixture（24/11） | 生成、5 组 payload、11 组保护、回归与政策通过 | 条件通过 | 补 5 个模板；补 Skill 的 diff Usage/Flags/Examples |
| aisearch | 7 个新增 concept；4 个 override；30 个 fixture（13/17） | 生成、4 组 payload、保护、回归与政策通过 | 条件通过 | 补 4 个模板；另审父命令/路径 alias 与 hidden 兼容面 |
| whiteboard | 2 个新增 + 1 个既有 concept；2 个 override；20 个 fixture（8/12） | 临时修复 fresh-tree 可见性后全部规则与政策检查通过 | 基线阻断 | 先修 declaration-only 生成树可见性，再补 2 个模板并全量复验 |
| audit | 1 个既有 concept 扩围；3 个 override；28 个 fixture（16/12） | 生成、payload、文件落盘、保护、回归与政策通过 | 条件通过 | 补 3 个 complete-command 模板；Audit Skill 缺失作为独立契约事项 |
| pat | 0 个 concept；2 个 override；28 个 fixture（14/14） | 候选规则与政策通过；另发现 dry-run 仍写本地策略 | 条件通过 | 补 2 个模板；单独修复并回归 browser-policy dry-run Runtime 契约 |
| devdoc | 2 个既有 concept 扩围；1 个 override；17 个 fixture（8/9） | 包括完整 `internal/app` 295.860 秒在内全部通过 | 可进入落地评审 | Help/Skill 默认输出与必填描述、非法数字/双入口行为作为独立后续 |
| live | 0 个 concept/alias；1 个纯保护 override；16 个 guard | 包括完整 `internal/app` 239.904 秒在内全部通过 | 可进入落地评审 | 无 active fixture 模板依赖；保持零业务参数边界 |
| mcp | 0 个 concept/alias；1 个纯保护 override；16 个 guard | 包括完整 `internal/app` 250.336 秒在内全部通过 | 可进入落地评审 | 无 active fixture 模板依赖；保持位置参数和敏感输出边界 |
| hrbrain | 3 个新增 + 3 个既有 concept；11 个 override；59 个 fixture（31/28） | 候选规则全过；临时补 10 个模板后完整 `internal/app` 237.773 秒通过 | 条件通过（已证明配套方案） | 候选与 10 个 complete-command 模板同一变更落地 |

状态统计：3 个候选已通过现有完整应用门禁，可直接进入正式落地评审；8 个候选的规则和运行链路
已通过，但需补 complete-command 模板或配套契约修复；1 个 Whiteboard 候选还受冻结基线
fresh-tree 可见性缺陷阻断。若 12 个产品一次性合并，新增模板前置合计为 45 个不同命令；Devdoc
使用既有模板，不计入新增数量。

## 交付索引与候选哈希

| 产品 | Markdown | 五页 XLSX | 完整候选 | 候选 SHA-256 |
|---|---|---|---|---|
| report | [报告](report/report_cli_param_hallucination_analysis_20260820.md) | [工作簿](report/report_cli_param_hallucination_analysis_20260820.xlsx) | [候选](report/param_concepts.json) | `543dfafca84535090c1fa29178c45c27eea42470713ea7a6816c884716c2552a` |
| ding | [报告](ding/ding_cli_param_hallucination_analysis_20260820.md) | [工作簿](ding/ding_cli_param_hallucination_analysis_20260820.xlsx) | [候选](ding/param_concepts.json) | `382e6c16c913b78bf325cf7193cca32eb8b035439af970b9df58735b133e398c` |
| event | [报告](event/event_cli_param_hallucination_analysis_20260820.md) | [工作簿](event/event_cli_param_hallucination_analysis_20260820.xlsx) | [候选](event/param_concepts.json) | `b3d0a0ddb6fa067222742b96c28130f44a62df85e7b55d699497f15ff0e39d36` |
| markdown | [报告](markdown/markdown_cli_param_hallucination_analysis_20260820.md) | [工作簿](markdown/markdown_cli_param_hallucination_analysis_20260820.xlsx) | [候选](markdown/param_concepts.json) | `7df963c21af2d92c24b2a65e9110f990d97fd575eeded01962f31a3fc0458c6f` |
| aisearch | [报告](aisearch/aisearch_cli_param_hallucination_analysis_20260820.md) | [工作簿](aisearch/aisearch_cli_param_hallucination_analysis_20260820.xlsx) | [候选](aisearch/param_concepts.json) | `1742e69d628c3f99945201acc9cd1a46fb7b93a1e080a9f19294a87a97eba514` |
| whiteboard | [报告](whiteboard/whiteboard_cli_param_hallucination_analysis_20260820.md) | [工作簿](whiteboard/whiteboard_cli_param_hallucination_analysis_20260820.xlsx) | [候选](whiteboard/param_concepts.json) | `3a45256af6fe3773a7a944463a5027003a2d25dab8b7e8613f02dcf5435dfba8` |
| audit | [报告](audit/audit_cli_param_hallucination_analysis_20260820.md) | [工作簿](audit/audit_cli_param_hallucination_analysis_20260820.xlsx) | [候选](audit/param_concepts.json) | `bf038d49c355732f40f6e033d804f3f84581ae6dc431104d191d69fe970a28f3` |
| pat | [报告](pat/pat_cli_param_hallucination_analysis_20260820.md) | [工作簿](pat/pat_cli_param_hallucination_analysis_20260820.xlsx) | [候选](pat/param_concepts.json) | `d09712be8b70294c08a67c7977d9e8f028be51777b6f9563ac51e2d87d37ea03` |
| devdoc | [报告](devdoc/devdoc_cli_param_hallucination_analysis_20260820.md) | [工作簿](devdoc/devdoc_cli_param_hallucination_analysis_20260820.xlsx) | [候选](devdoc/param_concepts.json) | `1aa25a72e583aca9853beb262316e8e11543fcde0e0b2dc59ccd070484b78b82` |
| live | [报告](live/live_cli_param_hallucination_analysis_20260820.md) | [工作簿](live/live_cli_param_hallucination_analysis_20260820.xlsx) | [候选](live/param_concepts.json) | `842d3a7f04073962622ce0ab23c8b5cf22a3f5b771917f9d0765219060d54d1b` |
| mcp | [报告](mcp/mcp_cli_param_hallucination_analysis_20260820.md) | [工作簿](mcp/mcp_cli_param_hallucination_analysis_20260820.xlsx) | [候选](mcp/param_concepts.json) | `81a99ee95f4b4da06752b13d813fa2c555df2510fcb4239e772c69a947754482` |
| hrbrain | [报告](hrbrain/hrbrain_cli_param_hallucination_analysis_20260820.md) | [工作簿](hrbrain/hrbrain_cli_param_hallucination_analysis_20260820.xlsx) | [候选](hrbrain/param_concepts.json) | `ebae9af7df01928031bf444e829e10232e97b058d5b114774a416445adacf55c` |

## 跨候选合并审计

12 份候选均从同一冻结正式表独立派生。与冻结基线比较：

- 共涉及 39 个唯一 concept，其中 20 个新增、19 个既有 concept 精确扩围；
- 共新增或修改 63 个精确 command override；
- 共新增 309 个验证 fixture；
- 没有删除任何冻结 concept、override 或 fixture；
- `$schema`、version、morphological rules 等非目标结构全部保持不变；
- 所有 override 都属于对应目标产品；
- 跨产品不存在同名 command override 冲突，也不存在重复新增 fixture。

有 6 个 concept 被多个产品共同扩展：

| 共享 concept | 产品 | 合并结论 |
|---|---|---|
| `pagination_size` | report、hrbrain | 非命令字段相同，合并 commands 并集 |
| `content_text` | ding、markdown | 非命令字段相同，合并 commands 并集 |
| `open_conversation_id` | ding、event | 非命令字段相同，合并 commands 并集 |
| `search_query` | event、devdoc、hrbrain | 非命令字段相同，合并 commands 并集 |
| `local_output_path` | markdown、audit | 非命令字段相同，合并 commands 并集 |
| `page_number` | devdoc、hrbrain | 需要显式并集：保留 Devdoc 新成员 `page-number`，同时加入 HRBrain 精确 commands |

前五项没有语义冲突；`page_number` 也不是业务矛盾，但不能让后写入的独立候选覆盖前一个候选的
成员或命令范围，必须由评审者明确构造并集。

## 跨产品共性结论

### 可以由当前参数字典安全处理

- 同一实体、同一角色、同一值域、同一单复数、同一单位，且值可原样传递的 flag 拼写；
- 只在精确命令内成立的 command-level alias；
- 宽泛真实参数在精确命令中的 concept 绑定；
- 值域、角色、单复数、分页模型、输入/输出或产品边界明确错误时的 block；
- 同一输入存在多个合理 canonical 目标时的 ambiguous；
- 无业务参数、位置参数或敏感输出型命令的纯 fail-closed 保护。

### 当前参数字典不能解决

- 位置参数与 flag 之间的 argv 角色转换；
- userId、openDingTalkId、工号、手机号、会话 ID、消息 ID、人才池编码等值域查询或互转；
- 单值与多值拆分/合并，CSV 与 JSON 互转，正文包装为结构化字段；
- 日期补时区、duration/TTL/timeout 单位换算、offset/cursor/page 模型转换；
- JSON 数组成员或表达式的深层业务校验；
- 同名真实 flag 在业务层与全局层承担不同角色时的自动意图消歧；
- 新增真实命令、flag、权限、搜索、连接配置、干运行能力或安全确认；
- 修复 Help/Schema/Skill/Runtime 的事实漂移。

这些事项应落在 leaf Contract/Runtime、CLI 命名与校验、Skill/Schema 来源或显式跨产品编排，
不能通过扩大 alias 范围掩盖。

## 建议落地顺序

1. 先单独落地并回归 Devdoc、Live、MCP 三个全门禁通过候选。
2. 为 Report、Ding、Event、Markdown、AI Search、Audit、PAT、HRBrain 补齐对应 complete-command
   模板；HRBrain 已在隔离副本证明补齐后全量应用通过。
3. 先修 Whiteboard 在 `NewSchemaSourceRootCommand()` fresh declaration-only 树中的可见性，证明
   原样 `go generate ./internal/cli` 可运行，再加入 2 条模板。
4. 同步处理明确的配套契约项：Ding Shortcut Identity、Markdown Skill diff、PAT dry-run 写入、
   Report 发送人值域；Devdoc Help/Skill 漂移可作为非阻断后续但不应遗忘。
5. 从**冻结正式表或选定的新正式基线**重建一个合并候选，按概念/override/fixture 语义合并，
   不要依次复制 12 个完整 JSON 文件。
6. 对 `page_number` 显式保留 Devdoc 的 `page-number` member 与 HRBrain commands 并集，对另外 5 个
   共享 concept 合并命令范围。
7. 统一执行 `go generate ./internal/cli`、全部 fixture/PreParse、complete-command canonical payload、
   block/ambiguous dispatch 前保护、非目标回归、`internal/cli`、`internal/pipeline`、产品专项、
   完整 `internal/app`、generated drift、Schema Catalog 与 Runtime confirmation truth。
8. 只有合并候选在新的同一提交上全绿后，才替换正式 `internal/cli/param_concepts.json`；本轮各
   产品候选仍保持“评审草稿”身份。

## 最终落地判断

本轮分析和候选设计已经完成，12 个产品的参数事实、可安全兜底范围、当前能力边界与落地依赖均
已形成可审计交付。可以立即进入正式评审的是 Devdoc、Live、MCP；其余候选不是规则逻辑未知，
而是明确受 complete-command 模板、Whiteboard 生成树可见性或已识别 Runtime/Skill 契约缺口约束。

当前不建议把任意一份候选直接覆盖正式表，也不建议把 12 个完整候选按顺序复制。正确落地形态是：
先修前置、按冻结差异合并、显式处理共享 concept、补 45 个模板，再在一个隔离合并副本中统一跑完
全门禁。
