# DWS Drive 产品 CLI 参数幻觉分析

## 1. 结论摘要

本轮以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，从该提交重新构建二进制，并使用同一官方命令树对 Drive 的真实 Help、运行时组装 Schema、仓库内置 Skill、命令实现和正式参数概念表进行对账。分析未使用历史 badcase、`dws-eval`、历史工作簿、固定 Catalog、用户自定义 Shortcut 或插件。

Drive 当前共有 **41 个 Agent-visible 工具**，其中 **7 个仓库内置 Shortcut**；全部 41 个命令都有业务参数，共出现 **154 次业务参数**、形成 **52 个不同公开 flag 名**。初版盘点把 `quiet` 按常见全局输出参数排除了，但 `drive list --quiet` 实际是该 leaf 的本地业务/输出行为参数，用于关闭递归进度输出，因此本版已补回。逐命令对账后，真实 Help 与运行时 Schema 的公开业务 flag 集合差异为 **0**，说明本轮参数问题不是 Schema 快照过期，而主要来自同一产品内部的命名、值域、业务角色和可发现性差异。

分析聚合为 **7 类参数问题**，在 Excel 中形成 **89 条“问题—命令”明细**，覆盖全部 41 个有业务参数的命令。最需要优先处理的是：

- `--node`、`--folder`、`--space-id`、`--workspace` 和回收站 `--id` 都像资源标识符，但分别属于节点、父目录、数字存储空间、知识库工作区和回收项，不能全局互换；
- Drive Skill 明确区分数字型 `dentryId` 与 `dentryUuid/fileId`：公开 `--node` 接收的是后者，因此 `dentry-uuid → node` 可以归一，而 `dentry-id → node` 必须在相关命令中拦截；
- copy/move/upload/commit 等写操作同时存在源对象与目标位置，参数实体相近但业务角色相反；
- permission 命令同时出现用户列表、授权角色、筛选角色、新所有者和保留角色，单复数与操作角色都必须区分；
- 文件上传、下载和版本工作流中的本地路径、远端名称、总字节数、分片大小、版本号和上传会话 ID 不能因为名称相近而互转；
- 仓库已有大量隐藏兼容 flag，但 `drive permission list --page-size` 和 `drive upload --file-path` 存在“Cobra 接受、最终实现未读取”的问题，不能把解析成功误判为业务等价。

冻结正式别名表源码中有 2 个 Drive override，生成结果实际触达 **3 个 Drive 命令、4 条 alias、1 条 block、2 条 ambiguous**。收敛后的候选草稿扩展到 **41 个 Drive 命令、376 条 alias、206 条 block、39 条 ambiguous**；相对冻结正式表新增 8 个 Drive concept、扩展 6 个既有 concept 的 Drive 命令范围、新增 36 个 Drive override，并修正 1 条已不符合业务语义的 Drive validation fixture。文件名、显示名、下载输出路径、排序字段、上传会话 ID 和新所有者这 6 类只在少量命令中成立，因此改为精确 command override，不再创建产品级 concept。所有非 Drive concept、override、保护规则和 fixture 均保持不变。

收敛后的候选已在隔离副本通过生成、构建、4 组代表性 alias/canonical 最终 dry-run payload 等价、2 组保护行为以及 `internal/cli`、`internal/pipeline` 回归。因为 fixture 从 alias 改为 block，正式落地还必须同步删除 `param_alias_payload_equivalence_test.go` 中已经没有 active alias 对应的 `drive info` complete-command 模板；候选 JSON 单独替换会被 stale-template 门禁拦住。候选仍是待评审草稿，不会直接替换当前工作区的正式 `internal/cli/param_concepts.json`。

Skill 审核也在本次复查中收紧：仓库内置 Drive Skill 只显式提到 41 个可见工具中的 **25 个**，另有 **16 个**没有命令级说明；共有 **13 个真实公开 flag** 未在 Skill 中出现。已提及的命令示例没有发现错误 flag，并不等于 Skill 对全部 Drive 命令和参数完整覆盖。

## 2. 参数问题

### 2.1 节点、目录、存储空间、知识库与回收项标识符混杂

Drive 最常见的参数是 `--node`，用于文件或目录节点；`--folder` 表示父目录或目标目录；`--space-id` 表示数字 DingDrive 存储空间；`--workspace` 表示知识库工作区；`drive recycle restore --id` 则只接受回收站列表返回的回收项 ID。

这些值都可能表现为字符串，名称也都可能被模型概括成 `id`、`file-id`、`folder-id`、`workspace-id` 或 `space-id`。但字符串形态相同不等于值域相同。例如：

- `drive info --space-id` 不能用知识库 `workspace` 替代；
- `drive permission add --workspace` 不能用数字 `space-id` 替代；
- `drive recycle restore --id` 不能接收普通 `node-id`；
- `drive list` 和 `drive upload` 同时有 `--space-id` 与 `--workspace`，因此宽泛 `--space` 不能安全选出唯一目标。
- `dentryId` 是数字型旧标识，`dentryUuid/fileId` 才能作为公开 `--node` 的值；两者不能因为都带有 `dentry` 前缀而互换。

候选通过精确 command scope 绑定同一实体别名，并对跨值域名称进行 block 或 ambiguous：`dentry-uuid` 可归一到 `--node`，`dentry-id` 在 28 个相关命令中统一 block。它不会查询 ID、不会把 URL 解析为 node，也不会在 storage space 与 knowledge-base workspace 之间转换。

### 2.2 源对象、目标目录与创建位置角色容易互换

`drive copy`、`drive move`、`drive shortcut` 以及对应 Shortcut 同时接受源 `--node` 与目标 `--folder`/`--workspace`；`drive upload` 还同时存在本地 `--file`、覆盖目标 `--node`、目标目录和空间；`drive commit`/`mkdir` 则表达创建位置。

这类命令不能使用全局 `file-id → node`、`folder-id → folder` 就宣称治理完成，因为来源参数还必须保留 source/destination 角色。候选只在精确命令中接受 `source-node-id`、`target-folder-id`、`destination-workspace-id` 等角色明确的名称，并对 `target-id`、`destination-id` 等仍有多个合理目标的名称提示歧义。

### 2.3 检索、过滤、排序和分页参数命名分散

Drive 的搜索命令使用 `--query`，分页主要使用 `--limit`/`--cursor`，但筛选还包含创建时间、修改时间、文件类型、资源类型、操作类型、组织范围、目标范围和排序字段/方向。常见幻觉包括 `--keyword`、`--page-size`、`--page-token`、`--created-after`、`--modified-before` 和宽泛 `--types`。

候选扩展 `search_query`、`pagination_size`、`page_cursor`，新增四个时间端点 concept 和一个排序方向 concept；排序字段只在 `drive list`、`drive star list` 中用 scoped alias 归一。它只改参数名并原样传值，不做游标换算、时间格式补全、枚举翻译或多条件合并。

### 2.4 权限用户、角色、新所有者与发布权限角色不同

`drive permission add/apply/remove/update/list/transfer-owner` 使用 `users`、`role`、`filter-role`、`new-owner`、`reserve-role` 等参数；`drive publish set --permission` 又是公开发布权限，而不是协作者角色。

候选把人员列表、新所有者单值、授权角色、筛选角色和发布权限拆开：同一角色同一卡数时可归一；`new-owner-id`、`new-owner-user-id` 仅在 transfer-owner 中映射到 `new-owner`，含义不明确的 `owner-user-id` 改为 ambiguous；`user-id` 与 `users`、`role` 与 `filter-role`、协作者 role 与 publish permission 之间则拦截或提示歧义。它不把用户名解析成 userId，也不把单值包装成列表。

### 2.5 文件传输路径、名称、大小、版本和分片单位易混淆

上传和下载链路同时包含 `--file`、`--file-name`、`--output`、`--file-size`、`--part-size`、`--version`、`--upload-id`、`--parallel`、`--mime-type`。这些字段处在同一工作流中，但分别代表本地输入路径、远端显示名、本地输出路径、总字节数、分片单位、历史版本、上传会话和并发度。

候选为值可原样传递的名称增加 scoped alias，例如 `save-path → output`、`upload-session-id → upload-id`；`size-bytes → file-size` 由单位明确的 concept 支持，`version-number → version` 复用既有 concept。同时阻断 path/name、file-size/part-size、node/version 之间的错误互换。路径解析、单位换算、MIME 推断和分片计算不属于当前别名表能力。

### 2.6 宽泛名称、类型和布尔开关依赖命令上下文

`--name` 在 mkdir 与 rename 中表示目标显示名称；`file-types`、`content-types`、`resource-types`、`creator-type`、`operate-type` 分别属于不同枚举或列表；`no-resume`、`convert`、`latest`、`thumbnail`、`versions` 是不同布尔行为。

候选只在明确命令中把 `folder-name`、`display-name` 或 `file-name` 归一到 `--name`，不建立产品级 display-name concept，也不建立宽泛全局 name/type 规则；布尔和枚举值保持 Cobra 原生语义，不做值格式转换。`drive list --quiet` 已纳入参数盘点，但无需新增 alias。

### 2.7 原生隐藏兼容参数与中央别名的可发现性和实现一致性

`internal/helpers/cross_product_aliases.go` 已为 Drive 注册多组隐藏 flag，例如 `node-id`、`parent-folder-id`、`workspace-id`、`keyword`、`page-size`、`page-token`、`user` 和 `file-path`。这些参数不出现在公开 Help/Schema 中，但可能被真实 Cobra 接受。

审核候选时已移除所有与真实隐藏 flag 重复的 alias 来源，避免双重治理。最终 payload 验证还发现两项实现边界：

- `drive permission list --page-size 20` 可以解析并进入 dry-run，但最终 `maxResults` 不存在，说明实现没有读取该隐藏 flag；
- `drive upload --file-path README.md` 可以被 Cobra 接受，但实现仍报 `flag --file is required`，说明上传逻辑只读取 canonical `--file`。

这两项必须修改命令实现或移除无效隐藏 flag；`param_concepts.json` 不能接管一个已经存在的真实 Cobra flag。

此外，Skill 当前只显式覆盖 25/41 个可见工具，16 个工具没有命令级说明，`content-types`、`latest`、`new-owner`、`notify-mode`、`pattern`、`quiet`、`reason`、`recursive`、`reserve-role`、`resource-types`、`sort`、`version`、`versions` 共 13 个真实 flag 未被提到。因此 Skill 只能作为已写内容的证据，不能作为 Drive 参数面的完整清单。

## 3. 当前别名表可以实施的方案

候选草稿位于同目录 `param_concepts.json`，是冻结正式表的完整副本加 Drive 改动，而不是增量片段。

第一轮建议落地以下治理：

1. 扩展既有 `search_query`、`pagination_size`、`page_cursor`、`folder_id`、`doc_version_number` 和 `space_id` 的 Drive 精确命令范围；
2. 只新增 8 个跨命令稳定的 Drive concept：回收项 ID、创建时间起止、修改时间起止、字节数、排序方向和直接授权角色；
3. 文件名、显示名、下载输出路径、排序字段、上传会话 ID 和新所有者只配置命令级 scoped alias，不提升为产品级 concept；
4. 为 copy/move/permission/upload/download/search 等 38 个 Drive 命令配置精确 override，其中 36 个为新增；
5. 对 `dentry-id/dentry-uuid`、space/workspace、node/folder、source/destination、single/list、permission role/filter role、file-size/part-size 等冲突配置 block 或 ambiguous；
6. 保留正确的原生隐藏 compatibility flag，不在中央表重复声明；
7. 将冻结表中 `drive info --workspace → --space-id` 的旧 fixture 改为 `did-you-mean:blocked`，明确知识库工作区与数字存储空间不是同一值域。

候选生成后的 Drive 影响面为 41 个命令、376 条 alias、206 条 block、39 条 ambiguous。相较收敛前，alias 减少 30 条；`dentry-id → node` 的 28 条错误 alias 全部消失，并转为 28 条保护规则；`owner-user-id` 增加 1 条 ambiguous。数量较大主要来自 identifier/value-domain 交叉保护，仍应在正式合入时由 Drive 业务 owner 复核概念名称和枚举口径。

## 4. 当前能力支持不了或不应该做的事项

- 知识库 workspace、数字 storage space、node、folder、recycle item 之间的查询和转换；
- 根据宽泛 `--space`、`--id`、`--target-id` 自动选择多个合理目标；
- 把单个用户转换为用户列表，或把用户名解析为 userId；
- 把 page/offset/cursor 等不同分页模型互相换算；
- 修改时间格式、补全时区、推导缺失的范围端点；
- JSON、枚举、MIME、布尔值和单位转换；
- 自动读取文件、推断输出目录或计算分片参数；
- 修复已经存在但实现未读取的 `--page-size`、`--file-path` 原生隐藏 flag。

上述场景应继续使用 leaf Help/Schema 的 canonical 参数。存在多个目标时，候选会在 dispatch 前停止，不为了扩大覆盖率强行改名。

## 5. 候选草稿审核结论

候选相对冻结正式文件的结构化审核结论如下：

- 新增 8 个 concept，全部只包含 `drive ...` 命令；
- 修改 6 个既有 concept，仅增加或移除 Drive 命令范围，成员、排除项、含义和风险没有变化；
- 新增 36 个 override，全部是 Drive 精确路径；候选共有 38 个 Drive override；
- `dentry-uuid → node` 保留，`dentry-id → node` 为 0，并在 28 个相关命令中 block；
- transfer-owner 只接受语义明确的 `new-owner-id`、`new-owner-user-id → new-owner`，`owner-user-id` 为 ambiguous；
- 非 Drive override 和保护规则变化为 0；
- validation fixture 只修改 `drive info/workspace` 一条，并由错误自动映射改为安全拦截；
- 与该 active fixture 配套的 `drive info` complete-command 模板已在隔离副本移除；候选 JSON 单独替换而不做这项测试维护会触发 stale-template 门禁；
- 所有命令路径都来自同提交官方命令树；
- 与 `cross_product_aliases.go` 中真实隐藏 flag 重复的来源已移除；
- 自动 alias 都满足同实体、同角色、同值域、同单位、同 cardinality 且值可原样传递；原先不满足该条件的 `dentry-id` 已改为保护规则；
- 不能确认的映射均转为 block、ambiguous 或“当前能力不支持”。

因此收敛后的候选在语义和作用域上更合理，可进入产品评审；它没有直接修改正式工作区别名表。Skill 缺失的命令和参数不影响本轮以 Help/Schema 为主的盘点，但说明后续不能仅凭 Skill 判断覆盖完整性。

## 6. 验证结果

收敛后的候选在隔离副本中临时替换正式输入并重新生成、构建和测试，当前已验证：

- JSON 解析、生成器读取和二次生成确定性；
- 4 组代表性 alias/canonical 最终 dry-run payload 等价：commit 文件元数据、download 输出路径、list 排序、transfer-owner 新所有者；
- 2 组关键保护在 dispatch 前停止：`dentry-id`、`owner-user-id`；
- 生成结果为 41 个 Drive 命令、376 alias、206 block、39 ambiguous；
- `internal/cli`、`internal/pipeline` 包回归通过；
- 2 个 accepted-but-ignored 原生边界仍可复现：`permission list --page-size` 与 `upload --file-path`。

完整 `internal/app` 与政策门禁不能只替换候选 JSON 直接通过，因为旧的 `drive info` complete-command 模板会成为 stale template；正式落地时必须先同步删除该模板，再执行全量 app、generated drift 与 Schema policy。初版候选曾完成更大范围的 21 组 payload/14 组 guard 验证，但其规则集合已经被本次收敛替代，因此本报告不再把该数字作为当前候选的通过结论。

写命令行为验证全部使用 `--dry-run`，未发起真实业务写调用。当前工作区正式 `internal/cli/param_concepts.json` 和 `internal/cli/param_aliases_generated.go` 在验证前后均无差异。

## 7. 第一轮改造建议

1. 先合入值域明确的 identifier、search/pagination、permission role 和命令级 transfer metadata 规则，以及对应 fixture；同时删除失去 active fixture 的 `drive info` complete-command 模板；
2. 将 space-id/workspace、source/destination 和 single/list 保护作为 P0 门禁一起落地，避免 alias 只增收益而缺少风险控制；
3. 将 `dentryId` 与 `dentryUuid/fileId` 的差异作为标识符硬边界，保留 28 个命令的 `dentry-id` block 回归；
4. 单独修复 `permission list --page-size` 与 `upload --file-path` 的实现读取问题，并为它们补最终 payload 测试；
5. 正式替换前由 Drive owner 复核 `role`、`permission`、`target` 等枚举和值域说明；
6. 补齐 Skill 未覆盖的 16 个工具和 13 个真实 flag，但不要让 Skill 反向覆盖 Help/Schema；
7. 保留候选的完整行为测试矩阵，避免后续命令新增时中央规则静默扩散。

## 8. 可复用分析流程

后续产品继续使用同一流程：冻结提交并构建官方二进制 → 盘点 runtime Schema、真实 Help、Skill 和内置 Shortcut → 按业务实体、值域、角色、cardinality、单位归并问题 → 基于冻结正式表生成完整候选 → 审核真实 flag 冲突和原生 compatibility → 在隔离副本执行生成、PreParse、payload、保护、包回归和政策门禁 → 只交付产品 Markdown、五页中文 Excel 和候选草稿，不直接改正式别名表。
