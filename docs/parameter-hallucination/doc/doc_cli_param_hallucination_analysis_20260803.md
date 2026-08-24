# DWS Doc 产品参数问题与参数幻觉现状分析

> 分析日期：2026-08-03
> 代码基线：`main@187787040b0e1339cb7fb3fe3b1229f3746efce2`
> 分析范围：`dws doc` 产品；不把 `drive`、`wiki`、`markdown`、`devdoc` 的命令纳入数量统计，仅在 Doc 的产品边界和参数来源发生交叉时说明。
> 数据边界：仅使用当前代码、真实 Cobra/`--help`、内嵌 Schema 和 Doc Skill；未使用 `dws-eval`、历史 badcase 或实验扫描结果。

## 结论摘要

本轮共盘点 **65 个可执行业务入口**：63 个可执行叶子命令，以及同时可执行又包含 `get` 子命令的 `doc export`、`doc import` 两个父命令。其中 **49 个可从正常帮助路径发现，16 个属于隐藏或迁移兼容入口**。这些命令合计有 **250 个公开参数位、72 个不同的公开参数名**；代码还注册了 **279 个隐藏兼容参数位、31 个不同的隐藏兼容名称**。这里的“参数位”按“命令 × 参数”计数，同一个 `--node` 出现在 40 个命令中会计为 40 个参数位。

整体判断不是“Doc 参数完全失控”，而是已经存在一套较强的原生兼容能力，但治理分布不均：稳定命令通常通过隐藏 flag 接受 `--doc-id`、`--node-id`、`--file-id`、`--parent-folder-id`、`--workspace-id`、`--page-size` 等常见名称；`+shortcut` 大多没有复用这些兼容参数。当前中央 `param_concepts.json` 在 Doc 上只覆盖 **5 个命令、8 个 alias 映射和 11 个保护性名称**，因此模型在稳定命令与快捷命令之间迁移参数名时，仍会出现“同一业务含义，在一个入口可用、另一个入口 unknown flag”的情况。

Schema 的基础参数契约本身较健康：当前 60 个 Schema 工具与对应 Cobra 命令的公开参数名、参数类型逐项比较，**结构差异为 0**。风险主要集中在 Schema 合同之外或更高层语义上：

1. `--node` 是 Doc 的主标识，但 10 个 `+shortcut` 没有稳定命令已有的节点别名；`doc +doc-append` 又单独使用 `--doc`。
2. `--folder`、`--workspace`、`--limit`、`--cursor`、`--query` 在稳定命令上已有部分原生兜底，4～7 个同类快捷入口仍未覆盖。
3. 33 个稳定命令把隐藏的通用 `--id` 固定解释为文档节点；其中 19 个命令同时还有块、评论、附件、用户或位置等其他角色参数。对 `block insert`、`media insert` 一类写命令，错误的 `--id` 可能被忽略为节点别名，无法表达模型原本想指定的插入锚点。
4. `doc export` 的隐藏本地 `--format` 与全局输出 `--format` 同名并发生遮蔽；传 `doc export ... --format json` 并不能得到 JSON 输出，而会进入旧版导出格式兼容逻辑。
5. `doc export`、`doc import` 是真实可执行主入口，但不是叶子命令，当前 Schema 和中央 alias 生成器都不能把它们作为一个完整工具/命令范围处理。
6. Skill 对 17 个公开 `+shortcut` 没有参数段，对 style/template/version 共 11 个公开稳定命令也没有参数段；另外存在 `--user`/`--users`、`--max-results`/`--limit`、导出格式、`--parent-block`、角色大小写等事实漂移。

因此，第一轮标准化应优先做三件事：补齐低风险、只改参数名的中央 concept/override；给块定位、任务 ID 等高风险近义词增加 block/ambiguous；把真实 flag 冲突、非叶子 Schema、参数组合约束和 Skill 漂移留给命令代码或 Schema/Skill 修复，不用 alias 表强行解决。

## 一、分析口径与现状量化

| 指标 | 结果 | 说明 |
|---|---:|---|
| 可执行业务入口 | 65 | 63 个叶子命令 + `doc export` + `doc import` |
| 正常帮助路径可发现 | 49 | 不经过隐藏祖先命令即可发现 |
| 隐藏/迁移兼容入口 | 16 | 仍可执行，其中 13 个进入 Schema，3 个未进入 Schema |
| Schema 工具 | 60 | 当前 `dws schema doc` 的工具数 |
| Help/Schema 参数名和类型差异 | 0 | 对 60 个 Schema 工具逐项比较公开 flag |
| 公开参数位 | 250 | 命令 × 公开参数 |
| 不同公开参数名 | 72 | 去重后的 canonical flag 名 |
| 隐藏兼容参数位 | 279 | 命令 × hidden flag |
| 不同隐藏兼容名 | 31 | 去重后的 hidden flag 名 |
| 有原生 hidden alias 的业务入口 | 43 | 主要是稳定命令和迁移兼容命令 |
| Doc 中央 alias 覆盖 | 5 个命令 | 2 个 concept + 3 个 command override |
| Doc 中央 alias/保护输出 | 8 / 11 | 8 个映射、11 个 blocked 名称 |
| Doc 校验 fixture | 6 | 3 个 alias case + 3 个 guard case |

当前中央能力覆盖的 Doc 行为只有：

- `doc +template-search`：`keyword/keywords/q/search-word → query`；保护 `name/subject/text/title`。
- `doc block insert`：`body/content → text`；保护 `before-block-id/name/title`。
- `doc block update`：`body/content → text`；保护 `name/title`。
- `doc +export-get`：保护 `node`，避免把文档节点当导出任务 ID。
- `doc block delete`：保护 `index`，避免把块位置当块 ID。

## 二、集中参数问题

### 2.1 文档节点标识命名没有贯穿稳定命令与快捷入口

Doc 的业务 canonical 是 `--node`。43 个业务入口公开使用 `--node`，其中 33 个稳定/兼容入口原生接受 `--doc-id`、`--node-id`、`--file-id`，大部分还接受 `--id`、`--url`。以下 10 个快捷入口只有 `--node`，没有同等兜底：

`doc +comment-create`、`doc +comment-create-inline`、`doc +comment-list`、`doc +comment-reply`、`doc +copy`、`doc +export-submit`、`doc +move`、`doc +version-list`、`doc +version-revert`、`doc +version-save`。

`doc +doc-append` 进一步把同一文档目标命名为 `--doc`，这会让从 `doc update --node ... --content ...` 迁移来的调用同时猜错目标参数和内容参数。

可由 alias 表解决的部分：新增命令范围严格限定的 `doc_node_id` concept，在上述快捷入口中将 `doc-id/node-id/file-id/document-id/url` 归一到 `node`，并为 `doc +doc-append` 用 bind 将真实 `doc` 归入同一概念。不要把 `doc list` 纳入该 concept：它的隐藏 `--node` 实际表示“要列子节点的文件夹”，不是文档操作目标。

不能直接归一的边界：`doc +share-doc --url` 要求的是可点击文档链接，不能把只有 nodeId 的 `--node` 仅靠改名变成 URL。这里应 block `node/doc-id/node-id` 并提示先取得文档 URL，而不是做 alias。

### 2.2 目标文件夹和知识库参数在快捷入口上缺少同等兼容

- 13 个命令公开使用 `--folder`；9 个稳定命令已经原生接受 `parent-folder/parent-folder-id/parent-node-id/parent-id`，4 个快捷入口 `doc +copy/+list/+move/+template-apply` 没有。
- 17 个命令公开使用 `--workspace`；13 个稳定命令已经原生接受 `--workspace-id`，同样是上述 4 个快捷入口没有。

`workspace-id → workspace` 可以直接复用现有 `space_id` concept，值域仍是同一个知识库 ID/URL。文件夹建议建立独立的 `doc_folder_node_id`，因为 Doc 的 folder 值必须是文档文件夹 nodeId/dentryUuid/URL，不能复用当前表示 drive folder id 的 `folder_id` 概念。

需要额外约束：`parent-id` 在其他产品经常表示纯数字 dentryId。稳定命令会通过 `validateDocFolderID` 拒绝纯数字值，但快捷入口直接把值发给接口，没有同等校验。alias 表只看名称，无法保证值域；第一轮不应把 `parent-id` 广泛新增到快捷入口，除非先补齐相同的值校验。

### 2.3 分页和搜索命名是最适合直接复用已有 concept 的一组

- 14 个命令公开使用 `--limit`。7 个稳定命令已有 `--page-size` 或 `--max-results` 原生兼容；以下 7 个快捷入口没有：`doc +comment-list`、`doc +find-doc`、`doc +list`、`doc +search`、`doc +template-list`、`doc +template-search`、`doc +version-list`。
- 12 个命令公开使用 `--cursor`。6 个稳定命令已有 `--page-token/--next-token`；以下 6 个快捷入口没有：`doc +comment-list`、`doc +list`、`doc +search`、`doc +template-list`、`doc +template-search`、`doc +version-list`。
- 5 个命令公开使用 `--query`。稳定 `doc search`、`doc template search` 原生接受 `--keyword`，`doc +template-search` 由中央 concept 接受；`doc +find-doc`、`doc +search` 仍不接受 `--keyword`。

这组参数都保持原值、类型和分页角色不变，可分别扩展现有 `pagination_size`、`page_cursor`、`search_query` 的 Doc 命令范围。`count/page/offset` 不应顺手加入：它们分别可能表示总数、页码或偏移量，现有 concept 的 excludes 应继续生效。

### 2.4 评论和权限参数存在“名称可兼容、参数形态不一致”两类情况

权限命令的真实 canonical 是 `--users`：`doc permission add/update/remove` 都接受逗号分隔 userId 列表。Skill 的权限子文档和部分意图说明仍教 `--user`；运行时因为原生 hidden alias 已接受 `user/user-id/user-ids/uid/user-list/user-id-list`，不会直接报 unknown flag，但会形成两个事实来源。这里应修改 Skill 统一教 `--users`，无需再向中央表重复添加。

评论命令统一公开 `--mention`，但稳定命令使用普通 string（要求 CSV），快捷命令使用 stringSlice（支持重复或 CSV）。名称相同、出现次数语义却不同：同一参数重复传入时，稳定命令最后一个值覆盖，快捷命令会累积。中央 alias 只改名，无法把两种出现次数语义变成硬等价；在统一 Cobra 类型前，不建议把通用 `--user-ids` concept 直接绑定到 `--mention`。

`doc comment create/reply/update` 的 `--mentioned-open-conversation-id` 实际是 stringSlice，可重复或 CSV，flag 名却是单数。可以增加严格限定到这 3 个命令的 `mentioned_open_conversation_ids` concept，接受复数 `mentioned-open-conversation-ids` 以及同角色的 `open-conversation-id(s)`；必须 block `group-id/group-ids`，因为数字群号不是 openConversationId。

稳定评论的群 @能力没有出现在 `doc +comment-create` 和 `doc +comment-reply` 快捷入口中。alias 表不能凭空给快捷命令新增底层能力；要么补快捷命令实现，要么明确路由稳定命令。

### 2.5 块标识与位置参数必须保留角色，不能做“看起来像”的同义归一

Doc 同时存在：

- `--block-id`：要读取、修改、删除或锚定评论的目标块。
- `--ref-block`：插入操作的同级参考块，通常还要结合 `--where before/after`。
- `--parent-block`：容器内插入的父容器。
- `--start-block-id/--end-block-id`：局部读取的区间边界。
- `--index/start-index/end-index/start/end`：分别表示块位置或块内字符偏移，不能互换。

现有 `before-block-id` block 是正确的：它需要同时生成 `--ref-block <id>` 和 `--where before`，仅改参数名会默认为 after。应将相同保护补到 `doc media insert`；为 `parent-block-id → parent-block`、`reference-block-id → ref-block` 添加角色保持的 scoped alias；把 `doc block insert --block-id` 标记为 ambiguous，因为无法判断是 ref 还是 parent。`doc read --block-id` 也不能简单映射，它需要同时确定 `scope=section` 和 `start-block-id`。

### 2.6 评论、任务、模板和版本标识需要小范围 concept/guard

- `--comment-key`：出现在 `doc comment reply/update/delete` 和 `doc +comment-reply`。`comment-id → comment-key` 可以保持相同 opaque 值直接归一；通用 `--id` 不可以。
- `--job-id`：只用于导出任务查询；`--task-id`：只用于导入任务查询。`doc +export-get` 已 block `node`，但稳定 `doc export get` 未保护，且两条导出查询都还应保护 node 的其他拼写和 `task-id`；`doc import get` 应反向保护文档节点名和 `job-id`。
- `--template-id`：稳定 `doc template apply` 原生接受 `template/tpl-id`，隐藏快捷 `doc +template-apply` 没有，可补 scoped alias。
- `--version` 与 `doc update --revision` 不是同一字段。前者是历史版本号，后者是并发检查版本。建议互相 block，而不是互设 alias。

### 2.7 原生通用 `--id` 过宽，中央 alias 无法覆盖真实 flag

33 个命令把隐藏 `--id` 注册为 `--node` 的真实兼容 flag；19 个命令同时还有其他角色参数。由于 `--id` 已经是真实 Cobra flag，中央 alias/blocked/ambiguous 不会接管它。

高风险例子是 `doc block insert --node DOC --id BLOCK --text ...`：`--node` 已经提供后，`--id` 仍会被解析为另一个节点别名并在 fallback 中被忽略，`--ref-block` 为空，命令可能按默认位置插入，而不是在 BLOCK 附近插入。类似风险也存在于 `doc media insert`。评论 update/delete 多数会因为缺 `--comment-key` 而失败，风险较低，但错误提示仍会偏离模型意图。

这类问题需要收窄或删除多标识命令上的原生 `--id`，或让原生 alias 注册支持按命令保护；不能靠 `param_concepts.json` 覆盖一个已存在的真实 flag。

### 2.8 `doc export --format` 与全局输出参数发生真实名称冲突

`doc export` 为兼容旧版，在本地注册了 hidden `--format` 作为 `--export-format` 的别名。这会遮蔽根命令的全局 `--format`。实测：

```text
dws doc export --node demo --output /tmp/demo.docx --dry-run --format table
```

命令仍按 `docx` 导出格式执行预览，`table` 没有控制输出呈现；同理 `--format json` 也不能把预览切成 JSON。这个冲突发生在真实 flag 解析层，而且 `doc export` 又不是叶子命令，中央 alias 不能修复。建议移除旧本地 `--format`，只保留 `--export-format`，全局 `--format` 恢复统一含义。

### 2.9 参数组合约束仍有缺口

当前 Schema 只为 3 个 Doc 命令发布组合约束：`block insert/update` 的 `text|heading|element` 至少一个，以及 `doc update` 的 `content|content-file` 二选一。仍需注意：

- `block insert/update` 只声明“至少一个”，未声明三者互斥；运行时代码按 `element > heading > text` 静默选一个。
- `block insert` 的 `ref-block`、`parent-block`、`index` 和 `where` 角色关系没有完整 Schema 约束。
- `media insert --where` 只有与 `--ref-block` 同时出现才有意义，Schema 未表达。
- `comment reply --emoji` 与 `--mentioned-open-conversation-id` 运行时互斥，Schema 未表达。
- `doc create` 同时给 `content` 和 `content-file` 时运行时优先文件，但 Schema 未声明互斥。

这些都是参数组合问题，不是参数名问题；应补 Runtime Schema constraints 和对应运行时校验，不应通过 alias 伪装成单参数修复。

### 2.10 Schema 与 Skill 的参数覆盖边界

60 个 Schema 叶子工具与 Cobra 公开参数名/类型一致，这是本轮最重要的正向结论。但存在两个结构边界：

1. `doc export`、`doc import` 是可直接执行的主命令，同时又含 `get` 子命令，因此不属于当前 Schema 的叶子工具。`dws schema --cli-path "doc export"` 只返回 `doc export get`，不会展示主命令的 `node/output/export-format`；`doc import` 同理只返回 `import get`。
2. 当前中央 alias 生成器也要求命令范围匹配 runnable leaf，无法给这两个主入口新增中央 alias。

Skill 的参数信息存在以下事实漂移或覆盖不足：

- 17 个公开 `+shortcut` 没有独立参数段，只能依赖 `--help`/Schema。
- style 5 条、template 3 条、version 3 条共 11 个公开稳定命令没有参数段；隐藏兼容的 `permission remove` 也未覆盖。
- 权限子文档使用 `--user`，真实 canonical/Schema 是 `--users`。
- 权限列表子文档使用 `--max-results` 且写默认 50，真实 canonical 是 `--limit`，默认 30；`--max-results` 只是 hidden alias。
- Skill 声明 role 必须大写且大小写敏感，运行时代码实际会 trim 并转成大写。
- Skill 声明 export 仅支持 docx，当前 `--help` 已支持 docx/markdown/pdf。
- `block insert` 文档示例使用 `--parent-block`，但参数清单没有列出它。
- `doc-info.md` 声称 `--parent-id` 不是 Doc 参数，实际 9 个命令把它注册为 hidden folder alias；真正需要强调的是值必须是 Doc 文件夹 nodeId/URL，不能是纯数字 dentryId。
- `doc import --help` 文案说 folder/workspace 至少传一个，但运行时 `docImportFlowConfig.requireTarget=false`，两者都不传可以导入到默认位置；Skill 在这一点反而与运行时一致。

这些问题不能由别名表替代修复：Skill 应统一教 canonical 参数，Schema 应解决非叶子入口暴露，Help 应与真实运行约束一致。

## 三、建议的别名表实现

以下是第一轮建议，目标是只接受“同一业务实体、同一值域、同一类型、同一角色、值原样透传”的映射。

| 建议项 | 实现方式 | 主要命令范围 | 结论 |
|---|---|---|---|
| `doc_node_id` | 新 concept；`node/node-id/doc-id/file-id/document-id/url/doc`，按命令实际 canonical 归一 | 10 个缺兼容的 node 快捷入口 + `doc +doc-append` | 可实现；排除 `doc list`、`doc +share-doc`、任务查询命令 |
| `doc_folder_node_id` | 新 concept；优先 `folder/folder-id/parent-folder/parent-folder-id/parent-node-id` | `doc +copy/+list/+move/+template-apply` | 可实现；`parent-id` 先不扩散，避免数字 dentryId 值域问题 |
| `space_id` | 扩展现有 concept 的命令范围 | 上述 4 个快捷入口 | 可实现；`workspace-id → workspace` |
| `pagination_size` | 扩展现有 concept | 7 个缺兼容的 limit 快捷入口 | 可实现；继续排除 count/page/cursor |
| `page_cursor` | 扩展现有 concept | 6 个缺兼容的 cursor 快捷入口 | 可实现 |
| `search_query` | 扩展现有 concept | `doc +find-doc`、`doc +search`，并可补全稳定 search/template search 的 q 等拼写 | 可实现；保持 name/title/text 排除 |
| `comment_key` | 新 concept；`comment-key/comment-id` | comment reply/update/delete 与 `+comment-reply` | 可实现；通用 id 不纳入 |
| `mentioned_open_conversation_ids` | 新 concept；保留 mention 角色 | comment create/reply/update | 可实现；block group-id(s) |
| Block role alias | scoped alias | `parent-block-id → parent-block`、`reference-block-id → ref-block` | 可实现 |
| Block role guard | block/ambiguous | insert/media 的 before-block-id，insert 的 block-id，read 的 block-id | 必须保护，不能猜 |
| 任务 ID guard | command override | export get、import get | 可实现；job/task/node 分域 |
| Template/version | scoped alias + block | template/tpl-id；version-number；version vs revision | 可实现，优先级较低 |
| `doc +share-doc` | command override block | node/doc-id/node-id | 只提示需要 URL，不做映射 |

不建议把所有原生 hidden alias 再复制到中央表。稳定命令已经具备并经过代码读取的兼容 flag，中央表应优先补“快捷入口缺口”和“需要明确 guard 的角色冲突”，以免形成两套重复事实。

## 四、当前能力无法单独解决的事项

| 事项 | 为什么 alias 表解决不了 | 应修改的位置 |
|---|---|---|
| `doc export`/`doc import` 主入口缺 Schema/中央 alias | 两者是可执行父命令，不是 runnable leaf | Schema 命令模型/命令树结构，或提供真正的叶子主入口 |
| hidden `--id` 过宽 | 它已经是真实 Cobra flag，中央 alias 不会覆盖 | `RegisterCrossProductAliases` 或 Doc 命令注册策略 |
| export 本地 `--format` 冲突 | 与全局 flag 同名且已进入 pflag | 删除旧 alias，仅保留 `--export-format` |
| `before-block-id` 等一变二参数 | 需要同时生成 ref-block + where，超出名称归一 | 命令级转换器或保留 block 提示 |
| share-doc 的 nodeId 变 URL | 需要查询/构造链接，是值转换和业务调用 | shortcut 编排逻辑 |
| 快捷评论缺群 @ | 真实 Flag/Execute 都不存在该能力 | shortcut 参数和 Execute 实现 |
| comment mention string/stringSlice 不一致 | 重复出现次数语义不同，改名不能保证等价 | 统一 Cobra flag 类型和 payload 测试 |
| 参数组合约束不完整 | 涉及互斥、依赖、优先级，不是单个参数名 | Runtime Schema constraints + RunE 校验 |
| folder 数字值域在快捷入口未校验 | alias 不检查值内容 | 快捷入口复用 `validateDocFolderID` |
| Skill/Help 事实漂移 | 文档事实错误不会被运行时 alias 自动纠正 | Doc Skill、Cobra usage、Schema 暴露 |

## 五、验证结果

- 使用当前 `main` 重新构建 `./dws`，从 `app.NewSchemaSourceRootCommand()` 枚举 Doc 命令树，避免用户 shortcut/plugin 影响分析结果。
- 对 60 个 Schema 工具逐条比较对应 Cobra 命令的公开 flag 名称和类型，差异为 0。
- 确认 3 个可执行 hidden leaf 未进入 Schema：`doc +comment-create-inline`、`doc +template-apply`、`doc file search`；这是当前公开 Schema 边界的一部分。
- 确认 `doc export`、`doc import` 为可执行非叶子命令，Schema 路径查询只返回各自的 `get` 子命令。
- 读取并核对 `param_concepts.json`、生成后的 alias 表、Doc helpers、Doc shortcuts、Doc Skill 主文档及全部 13 个命令子文档。
- 通过 `doc export --dry-run` 复现本地 `--format` 对全局输出参数的遮蔽。

## 六、建议实施顺序

1. 先补 `search_query`、`pagination_size`、`page_cursor`、`space_id` 和缺失快捷入口的 node/folder alias，并为每个映射补 canonical payload 等价测试。
2. 同步补 `comment-key`、任务 ID、块角色的 block/ambiguous；这些保护比扩大通用同义词更重要。
3. 修复 Skill/Help 的 canonical 参数、默认值和枚举，补 style/template/version 与 shortcut 的参数索引。
4. 单独处理 native `--id`、export `--format`、非叶子 Schema 和组合约束；这些属于命令/Schema 设计，不要塞进 JSON 兜底。
