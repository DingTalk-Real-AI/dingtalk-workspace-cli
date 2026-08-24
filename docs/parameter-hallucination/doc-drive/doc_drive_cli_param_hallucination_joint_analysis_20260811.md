# DWS Doc × Drive 参数幻觉联合分析

> 分析日期：2026-08-11
> 冻结基线：`origin/main@dde50494548f0ed4f6295b82e5cc214c074b0e30`
> 分析对象：官方 `doc`、`drive` 命令树、运行时组装 Schema、仓库内置 Skill、Shortcut 实现、正式参数概念表
> 数据边界：未使用历史 badcase、`dws-eval`、用户自定义 Shortcut、插件或固定 Schema 快照。

## 结论摘要

Doc 与 Drive 不能再完全分开治理。两个产品的命令路径不同，但共同操作文档空间中的节点、目录、工作区、权限和本地文件；其中 copy、move、delete 以及 permission add/list/update/remove 共 **7 组同名命令实际落到相同的 Doc RPC**。如果分别维护两套参数概念，最容易出现“同一个实体在 Doc 可兜底、在 Drive 不可兜底”，或把 Drive 的数字存储空间 ID 错当成 Doc 的知识库工作区 ID。

本轮在最新线上 `main` 的官方命令树中盘点 **131 个 Agent-visible 工具**，其中 Doc 90 个、Drive 41 个；合计出现 **521 个参数位**，Doc 367 个、Drive 154 个。Doc 有 94 个不同公开 flag，Drive 有 52 个，两边共有 **19 个同名 flag**。19 个同名 flag 的类型全部一致，但只有 10 个的底层 property 集合完全一致；其余 9 个受 Shortcut、本地文件处理或不同接口映射影响，说明“名字相同”不等于可以无条件跨产品合并。

联合分析识别出 **11 类需要治理的问题**。最重要的三项是：

1. `workspace` 与 `space-id` 不是同一实体。前者是知识库/文档空间工作区 ID 或 URL，后者是数字 DingDrive 存储空间 ID。正式概念表当前的 `space_id` 把两者放在同一概念中，存在跨值域错误归一风险。本轮候选将其改为严格的 `workspace_id`，并明确 block `space-id` 及其同义名称。
2. `dentryId` 与 `dentryUuid/fileId/nodeId` 不是同一值域。公开 `--node`/`--folder` 应使用后者；数字 `dentryId` 不能直接串到下一条 Doc/Drive 命令。候选允许 `dentry-uuid → node`，同时在相关 Drive 命令中阻断 `dentry-id`。
3. `drive +list`、`drive +find-file` 的 Shortcut 输出仍把多个候选 ID 投影为名为 `dentryId` 的稳定字段，而 Drive Skill 又明确要求后续命令使用 `fileId/dentryUuid`。这是输出契约问题，输入别名表无法修复，必须修改 Shortcut 投影和相应 Schema/Skill 说明。

基于正式表与收敛后的 Drive 草稿，本轮生成联合候选 `param_concepts.json`：共 **41 个 concept、165 个 command override、263 条 validation fixture**。生成后 Doc 命令得到 563 条 alias、1063 条 block、27 条 ambiguous；Drive 命令得到 384 条 alias、242 条 block、39 条 ambiguous。数量增加主要来自跨产品值域保护，而不是放宽通用 `id` 映射。

候选已在隔离的最新 `main` 副本完成生成、构建、263 条 fixture、全部 guard runtime contract、代表性最终 payload 等价、相关 Go 包、Schema policy、生成物漂移和组装确定性验证。正式 `internal/cli/param_concepts.json` 与生成文件没有被修改。正式落地时还需要同步维护最终 payload 测试模板；只替换 JSON 会因新增 Doc/Drive alias 缺少完整命令模板而触发门禁。

## 一、联合参数面现状

| 指标 | Doc | Drive | 合计/交集 |
|---|---:|---:|---:|
| Agent-visible 工具 | 90 | 41 | 131 |
| 参数位（命令 × flag） | 367 | 154 | 521 |
| 不同公开 flag | 94 | 52 | 19 个同名 |
| 同名 flag 类型一致 | — | — | 19/19 |
| 同名 flag property 集合一致 | — | — | 10/19 |

19 个共同 flag 是：`convert`、`created-from`、`created-to`、`creator-uids`、`cursor`、`extensions`、`file`、`filter-role`、`folder`、`limit`、`mime-type`、`name`、`node`、`output`、`query`、`role`、`users`、`version`、`workspace`。

这些共同名称可分成三类：

- 可安全共用概念：搜索起止时间、创建者列表、分页 cursor/limit、查询词、知识库 workspace、权限 role 等，前提是值原样传递且命令范围精确。
- 名字相同但业务角色不同：`file` 可能是本地输入文件或某个 Shortcut 本地参数；`output` 是本地下载/导出目标；`folder` 在不同接口中可能映射父目录或目标目录。
- 名字相同但必须保留边界：`node` 是远端文档空间节点；`version` 是历史版本号，不能与编辑 revision、导出 job ID、导入 task ID 或回收项 ID 合并。

## 二、Doc 与 Drive 的重叠命令

| Doc 命令 | Drive 命令 | 接口关系 | 参数关系 | 联合治理判断 |
|---|---|---|---|---|
| `doc copy` | `drive copy` | 同为 `doc.copy_document` | `node/folder/workspace` 完全一致 | 同一实体概念；优先引导使用 Drive 主入口 |
| `doc move` | `drive move` | 同为 `doc.move_document` | `node/folder/workspace` 完全一致 | 同上，保留 source/destination 角色保护 |
| `doc delete` | `drive delete` | 同为 `doc.delete_document` | `node` 一致 | 同一节点概念，写操作继续保留确认 |
| `doc permission add` | `drive permission add` | 同为 `doc.add_permission` | `node/workspace/users/role` 一致 | 用户列表、角色、节点和工作区可共同治理 |
| `doc permission list` | `drive permission list` | 同为 `doc.list_permission` | `node/workspace/limit/filter-role` 一致 | filter role 不能与授权 role 合并 |
| `doc permission update` | `drive permission update` | 同为 `doc.update_permission` | `node/workspace/users/role` 一致 | 同一概念，保持用户列表 cardinality |
| `doc permission remove` | `drive permission remove` | 同为 `doc.remove_permission` | `node/workspace/users` 一致 | 同一概念，不能把单用户值自动包装成列表 |
| `doc download` | `drive download` | 不同接口 | 共享 `node/output`；Drive 另有 storage/version/分片参数 | 只共用节点和本地输出，其他保持 Drive 独立 |
| `doc upload` | `drive upload` | 复合实现，无单一同源 RPC | 共享 `file/folder/workspace/convert`；远端名称不同 | 本地输入与远端显示名必须分开 |
| `doc info` | `drive info` | 不同接口 | 只共享 `node`；Drive 另有数字 `space-id` | 节点可共用，workspace 与 space-id 不可共用 |

Doc 中这些稳定兼容入口虽然仍被 Schema 发布，但部分属于隐藏/迁移性质的兼容命令。当前 alias 生成器只接受它定义的 runnable leaf，不能为部分隐藏 Doc compatibility 路径增加中央 alias。这个边界不会影响 Drive 主入口或公开 Doc Shortcut 的兜底，但意味着中央 JSON 暂时不能覆盖每一个历史兼容入口。

## 三、主要参数问题与处理方案

### 3.1 workspace 与数字 space-id 混用（P0）

Doc/Drive 的 `--workspace` 表示知识库或文档空间工作区；Drive 的 `--space-id` 表示数字存储空间。`drive list` 和 `drive upload` 同时拥有这两个参数，因此宽泛的 `--space` 无法安全决定目标。

联合候选用 `workspace_id` 管理 `workspace/workspace-id/knowledge-base-id/wiki-workspace-id`，并排除 `space-id/drive-space-id/storage-space-id/dingdrive-space-id`。数字空间别名继续只在精确 Drive 命令 override 中映射到 `--space-id`。这修正了正式表把两种值域放在 `space_id` 概念中的问题。

### 3.2 dentryId 与 dentryUuid/fileId/nodeId 混用（P0）

Drive Help 和 Skill 均要求公开 `--node`/`--folder` 使用 `dentryUuid` 或等价的 `fileId/nodeId`；数字 `dentryId` 是另一个标识符。候选将 `dentry-uuid` 纳入 `doc_node_id`，但将 `dentry-id` 加入排除项，并在 Drive 精确命令上 block。

仍需代码修复：`drive +list` 与 `drive +find-file` 当前输出投影用名为 `dentryId` 的字段承载 `dentryId/dentryUuid/id/fileId/nodeId` 中第一个存在的值。该字段不能作为可靠的后续输入契约；建议稳定输出改为 `fileId` 或 `dentryUuid`，数字 dentryId 如需保留则单列。

### 3.3 源节点、目标目录和目标工作区角色混用（P0）

copy/move/shortcut 同时接受源 `node` 与目标 `folder/workspace`。候选只接受角色明确的 `source-node-id`、`target-folder-id`、`destination-workspace-id`；`target-id`、`destination-id` 等无法确定目标实体的名称返回 ambiguous，`destination-node-id` 等明显错误角色直接 block。

### 3.4 搜索、分页、时间和创建者过滤不一致（P1）

Doc 与 Drive 都存在 `query/limit/cursor/created-from/created-to/creator-uids`，模型常生成 `keyword/page-size/page-token/created-after/created-before/creator-user-ids`。候选共用 `search_query`、`pagination_size`、`page_cursor`，并新增/扩展 `created_time_start`、`created_time_end`、`creator_user_ids`。所有映射只改名称，不转换时间单位、游标或列表格式。

### 3.5 权限用户和角色混用（P0/P1）

`users` 是目标协作者列表；`role` 是直接授权角色；`filter-role` 是查询筛选；`new-owner` 是单一新所有者；`reserve-role` 是原所有者保留角色；publish `permission` 是公开访问级别。候选共用 `document_permission_role`，但对 filter/new-owner/reserve/public permission 继续使用独立 scoped alias、block 或 ambiguous。

### 3.6 本地输入、输出、远端名称和正文混用（P1）

`file` 是本地输入，`output` 是本地下载/导出目标，`file-name/name` 是远端显示名，Doc `content` 是正文。候选新增 `local_output_path`，仅在安全的下载/导出命令中接受 `output-path/destination-path/save-path`；不把输入 file 与输出 output 互换，也不把裸文件路径自动当正文。

### 3.7 历史 version 与其他流程 ID 混用（P1）

Doc/Drive 的历史版本号可共同纳入 `doc_version_number`，但编辑 `revision`、导出 `job-id`、导入 `task-id`、回收 `id` 都必须独立。候选只扩大同一整数版本语义的命令范围，不做跨实体映射。

### 3.8 同名命令导致产品选路幻觉（P1）

Doc 与 Drive 的 10 组重叠命令会让模型在命令路径上犹豫。参数别名表只能在已经选定的 leaf 内改参数名，不能把 `doc copy` 自动切成 `drive copy`。该问题应通过 Schema selection、Skill 路由说明和主入口策略治理，而不是继续扩大全局参数同义词。

### 3.9 原生隐藏 flag 被接受但实现未读取（P1）

`RegisterCrossProductAliases` 已注册一些隐藏真实 flag。已确认两项问题：

- `drive permission list --page-size` 可被 Cobra 接受，但最终实现未读取该值；
- `drive upload --file-path` 可被 Cobra 接受，但处理器仍要求 canonical `--file`。

真实 flag 会先于中央别名生效，因此 JSON 无法接管。应修改命令实现读取正确 flag，或删除无效隐藏 flag，并补最终 payload 测试。

### 3.10 Shortcut 输出字段不能安全串联（P0）

`drive +list/+find-file` 的 `dentryId` 输出标签与真实值域不稳定，容易诱导模型把数字 ID 传给下一条 `--node`。这是“出参导致下一步入参幻觉”，必须在 Shortcut 投影、Schema 描述与 Skill 示例中统一修复。

### 3.11 隐藏 Doc compatibility 路径不在中央 alias 可治理面（P1）

生成器对 runnable leaf 有严格限制，部分隐藏兼容路径无法加入 concept/override。若业务要求这些旧路径也获得中央 alias，需要先调整命令可用性/生成器契约；当前更合理的做法是让 Agent 使用公开的 Drive 主入口或 Doc Shortcut。

## 四、联合候选别名表改动

候选文件：`docs/parameter-hallucination/doc-drive/param_concepts.json`。它是完整候选，不是补丁；正式文件未改。

主要变化：

1. 将 `space_id` 收敛为 `workspace_id`，只表示知识库/文档空间工作区，并严格排除数字 storage space。
2. `doc_node_id` 增加 `dentry-uuid`，排除 `dentry-id` 和 `space-id`。
3. `doc_version_number` 扩展到 Drive 的 download/download-version/revert，但保持 revision/job/task/recycle ID 分域。
4. 将创建时间上下界扩展到 Doc/Drive 搜索，统一为 `created_time_start/end`。
5. 将直接文档权限角色统一为 `document_permission_role`，不包含筛选角色、保留角色和公开 permission。
6. 新增 `creator_user_ids`，只用于 Doc/Drive 搜索中的创建者列表。
7. 新增 `local_output_path`，只用于下载/导出的本地目标路径。
8. 删除已经由 concept 统一拥有的知识库 workspace 和 output scoped alias，避免同一映射有两个来源。
9. 增加 10 条 Doc×Drive 联合 fixture，覆盖跨值域映射与保护边界。

候选结构审计通过：41 concepts、165 overrides、263 fixtures；非 Doc/Drive 的 override、fixture 和概念语义没有被改变。

## 五、当前能力无法解决或不应解决

| 问题 | 为什么别名表不能处理 | 建议位置 |
|---|---|---|
| numeric dentryId 转 dentryUuid/fileId | 需要查询或读取正确出参，不是改名 | Shortcut 输出/接口响应处理 |
| storage space 与 workspace 转换 | 两套值域，需要业务查询 | 命令编排或 resolver |
| Doc/Drive 同名命令选路 | alias 只处理已选 leaf 的参数 | Schema selection、Skill |
| `+list/+find-file` 输出标签错误 | 属于出参契约 | Shortcut 投影、Schema、Skill |
| hidden `page-size/file-path` 未被实现读取 | 已是真实 Cobra flag，中央 alias 不接管 | 命令实现/原生 alias 注册 |
| 隐藏 Doc compatibility 命令不被生成器接受 | 当前生成器只处理其 runnable leaf 集合 | 命令可用性或生成器契约 |
| content-file、URL、列表包装、时间/单位转换 | 涉及值转换或外部读取 | 类型化转换器或保持 block |
| `target-id/space/id` 等多目标名称 | 无法仅凭名称唯一选定 canonical | ambiguous，要求补充上下文 |

## 六、验证范围与结果

验证在 `/private/tmp` 的最新 `origin/main` 隔离副本中进行，候选只临时替换正式输入，未触发真实业务写调用。

已通过：

- `go generate ./internal/cli`，生成 319 个命令的参数规则；连续两次生成 hash 一致；
- 263 条 validation fixture 的最终 delivery path 测试；
- 全部 reviewed block/ambiguous guard 到 runtime contract 的测试；
- 代表性 alias/canonical 最终 payload 等价，包括 `dentry-uuid → node`、`knowledge-base-id → workspace`、`created-after → created-from`、`creator-user-ids → creator-uids`、`permission-role → role`；
- `internal/app`、`internal/cli`、`internal/helpers`、`internal/pipeline`、`internal/shortcut/doc`、`internal/shortcut/drive` 和 alias generator 包测试；
- Schema Catalog policy、runtime confirmation truth、generated drift、Schema assembly determinism；
- 正式 `internal/cli/param_concepts.json` 与 `internal/cli/param_aliases_generated.go` 保持无差异。

正式合入还需同步：为新增 active alias 补齐完整命令模板，并删除失去 active fixture 的旧模板。隔离验证已临时完成这些测试维护并证明可通过，但本轮交付只保存候选 JSON 和分析材料。

## 七、建议实施顺序

1. P0：先合入 `workspace_id` 与 storage `space-id` 分域、`dentry-id` block、source/destination 角色保护。
2. P0：单独修复 `drive +list/+find-file` 的稳定输出字段，确保后续命令拿到 `fileId/dentryUuid`。
3. P1：合入搜索、分页、创建时间、创建者、权限角色和本地输出路径的共用概念。
4. P1：修复 `page-size/file-path` 两个 accepted-but-ignored 原生 flag。
5. 补齐 payload 测试模板后，在正式分支运行全量生成、app、Schema、漂移和策略门禁，再替换正式别名表。
6. 在 Schema selection 与 Skill 中明确：Drive 是文件/目录管理主入口，Doc 是文档内容与结构操作主入口；参数表不承担跨产品命令选路。
