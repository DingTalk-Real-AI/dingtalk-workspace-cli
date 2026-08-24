# DWS Doc 参数幻觉静态审计与治理方案

> 分析日期：2026-08-06
> 审计基线：`fix/param-hallucination@b94e21331d7d3dafa08dca8764d695d5ae3317f2`
> 方法：遵循 `specs/product-cli-param-hallucination-analysis-spec.md`，以当前 Cobra/`--help` 为最高事实，其次为运行时组装 Schema、Doc Skill、当前参数兜底配置。
> 数据边界：本报告的静态结论不使用历史实验频次；`/Users/hyz/works/data/doc` 的逐 case 结果单列于 `doc_experiment_hallucination_observations_20260806.md`，只用于验证和补充候选项。
> 分支说明：本地 `origin/main@1fe01999` 比审计分支多两个发布元数据提交，但没有修改 Doc shortcut、Doc 命令声明或 Doc Schema；其相对差异中删除参数/命令兜底文件，是因为本 PR 尚未合入 main，不代表 Doc 命令面回退。

## 结论摘要

当前 Doc 命令面的公开契约总体健康：命令树共盘点 108 个 Doc 节点，稳定 Schema 发布 90 个工具；47 个 Doc shortcut 中 45 个公开并进入 Schema，2 个隐藏兼容 shortcut 未发布。对 90 个 Schema 工具逐项比较当前 Help 的本地公开 flag，参数名差异为 **0**。扫描 41 个 Doc Skill 文件中的 955 条 `dws doc` 代码片段后，也没有发现真实可执行示例使用当前命令不存在的 flag；扫描器初报的 15 条路径均为 `[flags]`、`create/update`、`version save/list/revert` 或省略号等文档记法，不是可执行命令。

风险不在“Schema 与 Help 大面积不一致”，而在同一业务实体跨稳定命令和 shortcut 使用不同参数名，以及部分相似名称需要值转换、角色判断或多参数展开。新增的 28 个 shortcut 扩大了这一表面：例如文档目标通常叫 `--node`，内容格式在 shortcut 中叫 `--doc-format`，历史版本与编辑 revision 又必须严格分域。

本轮候选 `param_concepts.json` 采用三类治理：

1. 对可原样透传的同实体、同角色参数增加严格命令范围的 concept 或命令级别名；
2. 对需要文件读取、URL 构造、值依赖展开或角色选择的输入，在执行前 block/ambiguous；
3. 保持真实 Cobra 参数、必填/互斥约束和安全确认不变，不用别名创造能力。

候选表已在隔离副本中完成生成器校验：生成 281 个命令的参数映射；相关 `internal/cli` 参数/命令兜底测试和 `internal/pipeline` 测试通过。候选仍只位于本目录，尚未同步到正式 `internal/cli` 文件。

## 一、审计范围和当前事实

| 指标 | 结果 | 说明 |
|---|---:|---|
| Doc 命令树节点 | 108 | 包含可执行父命令、稳定叶子、shortcut 和兼容入口，不含名称前缀误命中的 `doctor` |
| Schema 工具 | 90 | 当前运行时组装 `schema --all` 中 product=doc 的工具 |
| Doc shortcut | 47 | 45 个公开、2 个隐藏兼容 |
| 相比实验基线 `000bc134` 新增 shortcut | 28 | 旧快照 19 个、当前 47 个 |
| Help/Schema 参数名漂移 | 0 | 90 个 Schema 工具逐命令对账 |
| Doc Skill 文件/代码片段 | 41 / 955 | 未发现真实可执行片段的 flag 漂移 |
| 当前正式表中涉及 Doc 的 concept/override 命令 | 31 | 变更前覆盖，仍有新增 shortcut 缺口 |
| 候选 Doc concept / command override | 10 / 25 | 是完整候选表中的 Doc 相关配置，不是全局展开表 |
| 候选 Doc 验证 fixture | 57 | 包含既有和本轮新增正反例 |

45 个公开 shortcut 均已发布到 Schema。隐藏兼容项是：

- `doc +comment-create-inline`
- `doc +template-apply`

这两项仍可执行，但不能视为 Agent 正常选路面；候选参数治理不会把它们重新发布到 Schema。

## 二、聚合参数问题

### 2.1 文档节点标识在新增 shortcut 上缺少一致兜底

Doc 的主要文档目标是 `--node`，模型常生成 `--node-id`、`--doc-id`、`--file-id`、`--document-id`、`--doc` 或 `--url`。这些名称只有在目标参数确实接受同一个 nodeId/URL/token 且值可原样传递时才等价。

候选扩展 `doc_node_id` 到当前真实接受 `--node` 或特例 `--doc`、且不存在第二种媒体/资源身份的 Doc 命令，包括 access、background、checkpoint、comment、export、fetch、history、inspect、review 等新增 shortcut，以及对应稳定命令。媒体和资源 shortcut 采用更窄的命令级映射：只有 `doc/doc-id/document-id/node-id` 可归一到 `--node`；`file-id/url` 因可能表示附件、封面或图片 URL，必须按歧义停止。以下边界明确排除：

- `doc +share` 的真实目标是 `--url`，不能只把 nodeId 政名为 URL；应 block 并提示提供共享链接。
- `doc +media-*`、`doc +resource-*` 同时存在文档节点与媒体/资源角色，不能把通用 `--file-id/--url` 自动归一为文档 `--node`。
- 导入/导出任务的 `--job-id`、`--task-id`，评论 `--comment-key`、块 `--block-id`、历史 `--version` 和编辑 `--revision` 都不是文档节点。
- 通用 `--id` 角色不明确，继续排除。

结论：同值域、同角色部分可由 concept 解决；需要 URL 构造或角色判断的部分必须拦截。

### 2.2 workspace、folder、搜索和分页是低风险补齐项

新增 shortcut 广泛使用 `--workspace`、`--folder`、`--query`、`--limit`、`--cursor`，而模型会沿用稳定命令中的 `--workspace-id`、`--parent-folder-id`、`--keyword`、`--page-size`、`--page-token`。

候选方案：

- 扩展 `space_id` 到当前可用的 Doc create/import/access/list/move 等命令，保持同一个 workspace 值不变；
- 只在精确 Doc 命令上把 `folder-id/parent-folder/parent-folder-id/parent-node-id` 归一到 `--folder`；
- `parent-id` 继续 block，因为它可能携带数字 Drive dentryId，不能证明与 Doc folder nodeId 同值域；
- 扩展 `search_query`、`pagination_size`、`page_cursor` 到新增 shortcut 和仍公开的稳定命令；继续排除 page、offset、count 等不同分页语义。

结论：均可在现有别名表中实现，但 folder 必须采用命令级严格范围，不能建立跨产品全局映射。

### 2.3 内容、内容格式与文件输入不能混为一类

文本体的 `text/content/body` 可以在评论、checkpoint、create、append 等确实接收同一原始文本的命令中归一。`--content-format` 与 `--doc-format` 在 create/update 上同为 `markdown|jsonml`，也可通过新 concept `doc_content_format` 原样映射。

`--content-file` 不同：`doc +create`/`doc +update` 的 `--content` 支持 `@relative-path` 或 stdin，而模型传入的是裸文件路径。中央别名只改 flag 名，不会读取文件，也不会自动补 `@`。如果直接改成 `--content /tmp/a.md`，文件路径会被当成正文，产生静默错误。

结论：文本和格式可自动别名；`content-file` 在 shortcut 上必须 block，提示改用 `--content @relative-path`、stdin，或使用真实支持 `--content-file` 的稳定命令。

### 2.4 历史版本、编辑 revision、评论 key 必须分域

- `--version/--version-number/--version-no` 表示历史版本号，可在 history/version revert 范围归一；
- `--revision/--expected-revision` 表示乐观并发编辑修订号，只适用于 update；
- `--comment-key/--comment-id` 表示 commentKey，只适用于评论 reply/update/delete。

候选新增 `doc_edit_revision`，扩展 `doc_version_number` 和 `doc_comment_key`。历史 version 与 edit revision 互相 block；通用 `--id` 不进入评论 key concept。

结论：可做小范围 concept，但不能因名称接近而互相兜底。

### 2.5 fetch/inspect 的范围与 section 参数有角色语义

`doc +fetch` 的 `--start-block-id`、`--end-block-id` 分别是区间边界。`start-block/end-block` 可保持角色映射；角色不明的 `--block-id` 无法决定起点还是终点，必须 ambiguous。

`doc +inspect` 的基础文档信息始终返回：

- `--include-versions` 可精确映射为 `--include-history`；
- `--include-info` 是冗余幻想，block 并说明无需参数；
- `--include blocks` 需要读取值后决定是否改用 `+fetch`，不是单纯 flag 改名，必须 block。

结论：精确 section 同义词可映射；通用 include 和无角色 block-id 不可猜测。

### 2.6 update 操作选择可别名，但不能扩大取值集合

`doc +update --command append|overwrite` 与模型常写的 `--mode append|overwrite` 在该 shortcut 上含义一致，可做命令级 `mode → command`。该映射只改变参数名，最终仍由目标命令校验允许值，不会把其他 mode 变成合法操作。

结论：可实现，必须限制在 `doc +update`，并保留 Cobra 的必填、取值和安全确认。

### 2.7 access/share 的收件人、文档目标和共享 URL 是不同角色

access 系列的 `--to`、grant-and-share 的 `--node` 与 `--url`、share 的 `--url` 虽然都围绕“分享文档”，但值域和角色不同。候选只在 `doc +grant-and-share` 将明确文档 ID 拼写映射到 `--node`，原生 `--url` 继续表示发送给收件人的共享链接；不把 `to/user/member` 进行全局互换。

结论：只做精确文档目标别名；收件人解析和 node→URL 转换超出当前能力。

### 2.8 已存在的真实 flag 与参数组合问题不属于别名表

真实 Cobra flag 优先于中央别名。历史上过宽的 hidden `--id`、`doc export` 本地 `--format` 与全局输出格式冲突，以及 block/parent/ref、content/content-file 等组合约束，都不能通过 `param_concepts.json` 覆盖。

结论：需要修改命令声明、运行时校验或 Schema constraints；本轮候选不扩大这些行为。

## 三、候选参数表的具体动作

候选文件：`docs/parameter-hallucination/doc/param_concepts.json`。

| 治理对象 | 建议配置 | 处理方式 | 验证重点 |
|---|---|---|---|
| 文档 node 标识 | 扩展 `doc_node_id`，仅纳入值可原样传递且无第二资源身份的命令；media/resource 改为强身份命令级别名 | 增加概念别名/命令级别名 | 不进入 job/task/comment/block/version/revision 值域；media/resource 的 `file-id/url` 保持 ambiguous |
| workspace | 扩展 `space_id` | 增加概念别名 | `workspace-id` 与 `workspace` 值不变 |
| Doc folder | 精确命令 `folder-id/parent-folder-* → folder`；block `parent-id` | 命令级别名 + 安全拦截 | 数字 Drive dentryId 不被静默传入 |
| 搜索与分页 | 扩展 `search_query`、`pagination_size`、`page_cursor` | 增加概念别名 | 不把 page/offset/count 当 cursor/limit |
| 文本内容 | 扩展 `content_text` | 增加概念别名 | 只接受原始文本，不承担文件读取 |
| 内容格式 | 新增 `doc_content_format` | 增加概念别名 | `content-format → doc-format`，排除通用 format |
| 文件内容 | `+create/+update` block `content-file` | 安全拦截 | 明确提示 `@relative-path`/stdin/稳定命令 |
| 编辑 revision | 新增 `doc_edit_revision` | 增加概念别名 + version 隔离 | revision 与历史版本不互换 |
| inspect | `include-versions → include-history`；block include/include-info | 命令级别名 + 安全拦截 | 不做值依赖展开，不制造冗余 flag |
| fetch | start/end 精确别名；block-id ambiguous | 命令级别名 + 提示歧义 | 保留区间边界角色 |
| share | block 文档 ID 拼写 | 安全拦截 | 不把 nodeId 改名伪装成 URL |
| grant-and-share | 明确文档 ID 拼写仅映射 `node` | 命令级别名 | `url` 与 `node` 两个真实角色不混合 |

## 四、当前能力无法解决或不应解决

| 场景 | 为什么别名表不能处理 | 当前安全做法 | 未来能力 |
|---|---|---|---|
| `content-file` 裸路径变正文 | 需要读文件或补 `@`，属于值转换 | block 并给出替代命令 | 受控参数转换器/文件输入类型 |
| `include blocks` | 需要读取值并选择参数或切换命令 | block，提示 `+fetch` | 值依赖展开/路由器 |
| nodeId 变共享 URL | 需要查询或构造 URL | block，要求真实 `--url` | 受控编排转换 |
| 无角色 `block-id` | 起点/终点/父块/参考块均可能 | ambiguous | 多参数角色解析 |
| 原生 hidden `--id` 过宽 | 已被 Cobra 接受，中央预解析不接管 | 保持现状并单独治理 | 原生 alias 审核/收窄 |
| `doc export --format` 冲突 | 是真实本地 flag 与全局 flag 冲突 | 不新增别名 | 删除/迁移真实兼容 flag |
| 参数组合互斥/依赖 | 不是参数名问题 | 依赖运行时校验 | 完整 Contract constraints |
| 接收人名称解析/单复数转换 | 需要查人、拆分或合并值 | 不自动转换 | 类型化 resolver |

## 五、验证结果与剩余门禁

已完成：

- 当前二进制构建成功；
- 90 个 Schema 工具与 Help 的公开参数名对账，差异 0；
- 41 个 Skill 文件、955 条 Doc 代码片段审计，无真实 executable flag 漂移；
- 候选参数表 JSON 校验通过；
- 隔离副本执行 `go generate ./internal/cli` 成功，输出 281 个命令映射；
- 候选命令名表与参数表共同生成成功，输出 34 条命令路径兜底；
- 参数 fixture 已兼容布尔参数的 `--flag=value` 规范形式，`include-versions → include-history=true` 不再被误判为丢值；
- 7 个新增 active 命令均补齐完整业务调用模板，10 个新增 active alias 均完成 canonical/alias 最终 transport payload 等价验证；
- `+access-grant`、`+history-revert`、`+update` 的 alias 回放仍由原命令返回 `confirmation_required`，没有绕过确认；
- media/resource 的 `file-id/url` 回放返回 `ambiguous_flag`，强文档身份 `document-id → node` 可正常进入目标命令；
- `+create-version`、`+save-version`、`+snapshot`、`+version-create` 均保留原始 `--node` 并改写到 `+history-save`，mock 回放实际调用 `save_doc_version`；
- `+export-pdf` 负向回放仍返回 `unknown_shortcut`，没有被路径兜底误导到缺少 `--export-format pdf` 的导出流程；
- 更新隔离副本的预期数量/覆盖 fixture 后，相关 `internal/cli`、`internal/app` 参数/命令兜底测试和 `internal/pipeline` 命令兜底测试通过。

尚未执行的正式门禁：候选尚未同步到 `internal/cli`，因此没有在工作分支运行全量 `go test ./internal/app`、generated drift 和 Schema policy。正式实施时还必须同步测试期望（fallback 总数由 26 变 34，并增加 8 条 Doc 覆盖），再执行 Spec 第 9 节的完整命令。

## 六、第一轮实施建议

1. 先同步候选参数表，并补 payload 等价、blocked/ambiguous、原生 flag 不受影响的回归测试。
2. 同步 4 条可由现框架表达的 Doc shortcut 名兜底及覆盖测试；不加入跨 shortcut/non-shortcut 自动改写。
3. 对 `content-file`、`include`、share URL、block role 保持 fail-closed。
4. 单独立项处理原生 hidden `--id`、export `--format`、非叶子命令及参数组合约束。
5. 完整运行生成漂移、Schema Catalog 和 app 集成门禁后，再决定是否合入正式文件。

## 七、可复用审计流程

1. 从当前 Cobra 树枚举真实命令、公开/隐藏 flags 和 runnable 状态；
2. 用运行时组装 Schema 对账每个公开 leaf；
3. 扫描 Skill 中真正可执行的代码片段，并剔除文档记法；
4. 按业务实体、角色、单复数和值域聚合参数，而不是按字符串相似度聚合；
5. 只对“值可原样传递”的映射使用 concept/override，其余进入 block/ambiguous/当前不支持；
6. 在隔离副本生成并测试，最后才同步正式输入文件；
7. 实验 badcase 单列附录，用来验证静态结论和发现命令名幻觉，不取代当前契约事实。
