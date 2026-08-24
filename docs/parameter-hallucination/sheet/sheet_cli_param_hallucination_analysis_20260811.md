# DWS Sheet 产品 CLI 参数幻觉分析

## 1. 结论摘要

本报告以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，按照 `specs/product-cli-param-hallucination-analysis-spec.md` 对 Sheet 产品做全量分析。分析只使用同一提交重新构建的 DWS、运行时组装的 Schema、逐命令 `--help`、Sheet Skill、Shortcut 和必要实现代码；没有使用历史 badcase、`dws-eval`、`merged_scan.json` 或旧工作簿。

冻结基线共有 92 个可执行 Sheet 工具，包含 2 个公开 Shortcut；共出现 409 次业务参数，形成 112 个不同的真实 flag 名。逐命令对账显示，92 个命令的公开 Help 参数与运行时 Schema 参数没有差异，说明当前主要问题不是 Schema 快照过期，而是同一产品内存在大量“名字相近但业务角色不同”以及“同一角色使用不同名字”的情况。

本轮归纳为 7 类参数问题，覆盖全部 92 个命令。问题明细有 212 行，因为同一个命令可以同时存在标识符、区域、结构化值等多类风险，不能把 212 当成命令数。最需要优先处理的是：

- 文档节点、工作表、源/目标工作表、文件夹和知识库标识符容易混用；
- `range`、`ranges`、`source-range`、`target-range` 的单复数、结构和读写方向不同；
- `source`、`name`、`type` 等宽泛名字在不同命令中不是同一个概念；
- chart create/update 的同提交 Help/Schema 只承诺工作表 ID，但 Skill 写成“ID 或名称”，属于真实契约口径漂移；
- JSON、CSV、列表、像素和坐标参数不能通过参数改名安全互转。

基于上述分析，已生成一份完整候选 `param_concepts.json`，但没有替换正式文件。候选在正式表基础上新增 12 个 Sheet concept、扩展 9 个既有 concept 的 Sheet 命令范围，并新增 14 个 Sheet command override。生成后，91 个 Sheet 命令获得参数治理入口，包含 963 条 alias、1672 条 block 和 20 条 ambiguous 规则。

候选已经通过结构、生成、PreParse、最终 payload 等价、歧义保护、包测试和仓库政策门禁。它仍被标记为“待审核草稿”，原因是本轮没有把新增 Sheet 规则写入正式 `validation_fixture` 和完整命令 payload 模板；在补齐这些正式审核资产前，不应直接替换仓库正式别名表。

## 2. 产品参数现状

| 量化项 | 结果 | 说明 |
|---|---:|---|
| 可执行 Sheet 工具 | 92 | 来自冻结提交运行时组装 Schema，无 Sheet exact exclusion |
| 参数出现次数 | 409 | 按 92 个工具的业务参数累计 |
| 不同真实 flag 名 | 112 | 不含 root 全局输出类参数 |
| 使用 `--node` 的命令 | 85 | 表示文档/文件节点，不等同于工作表 ID |
| 使用 `--sheet-id` 的命令 | 69 | 多数承诺“ID 或名称”，chart 两个命令当前只承诺 ID |
| 使用 `--range` 的命令 | 27 | 单个 A1 区域或坐标语义 |
| 使用 `--ranges` 的命令 | 4 | 列表或 JSON 结构，不能与 `range` 直接互换 |
| 使用 `--source-range` / `--target-range` 的命令 | 3 / 3 | 源与目标角色必须保留 |
| 使用 `--target-sheet-id` 的命令 | 3 | 跨工作表操作中的目标工作表 |
| Help/Schema 公开参数差异 | 0 | 逐 92 个 leaf 使用同一冻结二进制核对 |

## 3. 七类参数问题

### 3.1 文档、工作表和目标位置标识符分层不一致（89 个命令）

Sheet 同时存在 `node`、`sheet-id`、`target-sheet-id`、`folder`、`folder-token` 和 `workspace`。它们都可能被模型概括为“表格 ID”或“URL”，但分别指向文档节点、工作表页签、目标工作表、文件夹和知识库空间。

可安全处理的是同一实体、同一角色、同一值格式的参数名变体，例如 `document-id → node`、`worksheet-id → sheet-id`、`destination-sheet-id → target-sheet-id`。不能安全处理的是 `id → 任一对象`、工作表名称查询成 ID，或者在同一命令中把一个通用 `url` 自动判断成 node、图片 src、folder 或 workspace。

### 3.2 A1 区域的单复数和源/目标角色不同（36 个命令）

`range` 是单个区域，`ranges` 通常是数组或 JSON 列表；`source-range`、`target-range` 分别承担读写方向；透视表创建还用 `source` 表示源 A1 区域。它们看起来相似，但 cardinality、结构和写入落点不同。

候选只在同角色内归一，如 `from-range → source-range`、`destination-range → target-range`。`range ↔ ranges`、无角色 `worksheet-id` 在同时存在源/目标的命令中会被 block 或 ambiguous，而不是静默转换。

### 3.3 检索词、评论正文和显示名称的常用名字不同（17 个命令）

查找使用 `query` 并保留原生隐藏 `find` 兼容参数，替换命令的真实参数是 `find`；评论正文使用 `content`；对象显示名称主要使用 `name`，部分命令还同时存在真实 `title`。

这类问题可以按精确命令扩展已有 `search_query` 和 `content_text` concept，并新增显示名称 concept，但不能全局把 `text`、`title` 或 `name` 互相替换。`sheet find --find` 已是原生命令兼容能力，候选不会重复建立中央 alias。

### 3.4 分页和子对象 ID 命名多样（29 个命令）

列表命令使用 `limit/cursor`，而图表、筛选视图、条件格式、浮动图片、透视表、评论、导入任务、模板和版本分别使用专用 ID 或 Key。分页的 `page-size/next-cursor` 可以原值归一；`comment-id → comment-key`、`version-no → version` 也可以在精确命令内处理。

泛化 `--id` 不安全，因为同一命令可能同时包含 node、sheet-id 和子对象 ID。候选不建立全局 `id` 概念，也不做跨对象 ID 查询或转换。

### 3.5 同一宽泛参数名在不同命令中含义不同（16 个命令）

`source` 在透视表创建中是源区域，在模板 list/search 中是模板来源枚举；`type` 在清除命令中表示清除内容/格式；`name` 覆盖工作表、筛选视图、图片、透视表等多种显示名称。

这些参数必须使用命令范围、bind 或 scoped alias 表达，不能仅根据同名建立全局 concept。候选只对 `pivot-table create` 的 source 做命令级绑定，模板 source 和 clear type 保持原生。

### 3.6 Help/Schema 与 Skill 的工作表名称能力口径冲突（2 个命令）

`sheet chart create` 和 `sheet chart update` 的冻结提交 Help 与运行时 Schema 都写“工作表 ID”，而 Sheet Skill 写“工作表 ID 或名称”。别名表无法证明后端是否真的接受名称。

当前候选采用同提交可执行契约的安全解释：允许 `worksheet-id` 等 ID 同义名，阻断 `sheet-name/worksheet-name` 等名称导向别名。后续需要先确认实现或后端能力，再统一 Help、Schema 和 Skill 的声明。

### 3.7 结构化值不能靠改参数名互转（23 个命令）

`values`、`sheets`、`styles`、`properties`、`criteria`、`condition`、`operations`、`ranges`、`sort-keys` 等参数要求不同 JSON 结构；CSV、起始单元格、像素值、行列索引又属于不同输入协议。

现有参数归一链路只修改 argv flag 名并原样保留值，不能构造 JSON、包装数组、拆分字段、修改嵌套 key 或换算单位。报告将这些场景列为当前不能解决，而不是配置一个表面上“能通过解析”的错误 alias。

## 4. 候选别名表怎么解决

候选草稿的核心策略如下：

1. 复用已有概念：把安全的 Sheet 命令加入 `doc_node_id`、`search_query`、`pagination_size`、`page_cursor`、`content_text`、`folder_id`、`space_id`、`doc_comment_key` 和 `doc_version_number`。
2. 新增 Sheet 专用概念：工作表、源/目标工作表、单区域、多区域、源/目标区域、本地输入文件、输出文件、维度轴、维度长度和显示名称。
3. 对跨角色命令使用 override：复制、移动、透视表、导入、chart、创建、模板、浮动图片等 14 个命令使用 bind、scoped alias、block 或 ambiguous。
4. 明确保留原生能力：真实 flag、Cobra 命令 alias、隐藏兼容 flag 不重复加入中央表。
5. 对 URL 和通用工作表名保护：6 个 URL 多角色命令，以及含源/目标工作表的命令，在无法判断角色时停止并提示，而不是猜测。

候选生成影响为：91 个 Sheet 命令、963 条 alias、1672 条 block、20 条 ambiguous。规模较大的主要原因是 concept 的成员与 excludes 会在每个精确命令上展开组合；这不是 2655 个独立人工决策。人工审核重点放在 command range、实体值域、源/目标角色、单复数结构、URL 多角色和 chart 契约漂移上。

## 5. 当前能力无法解决或不应该解决的事项

- 工作表名称查询并转换为 ID；
- 泛化 `--id` 自动选择文档、工作表或子对象；
- 单个 `range` 与 `ranges` JSON 列表互转；
- values、CSV、sheets、properties、criteria 等结构互转；
- 行列数量、索引、坐标和像素单位转换；
- folder/workspace 名称或 URL 自动判断业务角色；
- 修改 JSON 参数内部字段名或枚举值。

这些场景并非“发现错误也无法提示”。block/ambiguous 可以在执行前停止并给出 canonical 建议；无法完成的是自动选择业务实体、自动查询或改变参数值结构。

## 6. 候选审查结果

候选与冻结基线正式表的结构化差异审计结果：

- 新增 12 个 concept，全部只包含 `sheet ...` 命令；
- 扩展 9 个既有 concept，只增加 Sheet command range，没有修改 canonical、members 或 excludes；
- 新增 14 个 command override，全部属于 Sheet；
- 没有删除或修改既有非 Sheet override；
- `validation_fixture` 完全未变；
- 当前工作区 `internal/cli/param_concepts.json` 和生成文件没有被修改。

生成器在候选完善过程中实际阻止了三类不合理配置：同一个 `sheet-id` 同时归属两个 concept、同一参数既 alias 又 ambiguous、把真实隐藏 flag 重复声明成中央 alias。最终草稿已消除这些冲突。

## 7. 验证结果

验证全部在冻结 main 的隔离副本中进行，不调用真实业务 API：

| 验证项 | 结果 |
|---|---|
| JSON 结构校验 | 通过 |
| `go generate ./internal/cli` | 通过 |
| 代表性 PreParse | 29 条通过，覆盖 alias、block、ambiguous 和原生兼容 |
| alias/canonical 最终 payload 等价 | 11 组通过 |
| URL 多角色保护 | 6 条 ambiguous 专项及 document-id 正向用例通过 |
| `go test ./internal/cli ./internal/pipeline` | 通过 |
| `go test ./internal/app -count=1` | 通过，214.298 秒 |
| `check-generated-drift.sh` | 通过 |
| `check-schema-catalog.sh` | 通过，27 个产品、1018 个工具 |
| 非目标范围审计 | 通过，没有非 Sheet 语义变化 |

本轮草稿还没有为新增规则补入正式 `validation_fixture` 和 complete-command payload 模板。因此测试证明了候选逻辑可生成、可进入真实 PreParse 链路且代表性最终 payload 等价，但还不能替代正式合入时的全量审核 fixture。正式落地前应把本轮代表性用例转为仓库长期测试，并由产品/接口负责人确认 chart 的名称支持口径。

## 8. 交付物

- 本报告：`docs/parameter-hallucination/sheet/sheet_cli_param_hallucination_analysis_20260811.md`
- 汇报工作簿：`docs/parameter-hallucination/sheet/sheet_cli_param_hallucination_analysis_20260811.xlsx`
- 完整候选别名表：`docs/parameter-hallucination/sheet/param_concepts.json`

工作簿固定包含“汇报总览、参数问题明细、兜底解决方案、当前无法解决、分析依据”五个中文页面，可用于管理汇报、逐命令审核和后续落地跟踪。
