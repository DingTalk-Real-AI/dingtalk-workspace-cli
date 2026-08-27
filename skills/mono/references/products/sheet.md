# 电子表格（Sheet）

> 本文件是 Sheet 常见任务的唯一必读 reference，已经覆盖创建、定位、读写、验证、导出和清理。复杂任务按执行阶段加载精确子 reference：每个阶段最多一份，真正进入下一阶段时才允许继续加载；不要批量预读 `sheet/`、`dingtalk-shared` 或 Drive 文档。

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

Sheet 操作先读必要范围、做最小修改，再用匹配读命令回读；已有原生命令时不要用本地脚本或多次客户端读写模拟。

## 产品边界

| 用户或资源 | 路由 |
|---|---|
| 明确说“在线电子表格/工作表/单元格/A1/公式/图表/透视表/版本” | `dws sheet`，即使用户同时说“结构化整理”“表头”“记录”也不要改走 AITable |
| 明确说 Base/多维表/字段类型/记录视图，且没有 Sheet 原生操作 | `dws aitable` |
| 在线富文本文档 | `dws doc` |
| 本地 xlsx/xls，用户要转换为在线表格 | `dws sheet import create`；当前只支持 xlsx/xls |
| Drive 中的 xlsx/xls，用户要转换为在线表格 | 先用 `dws drive download` 下载到本地相对路径，再执行 `dws sheet import create`；不要把二进制节点传给工作表命令 |
| 只做本地分析，或文件是 xlsm/csv | 留在本地处理；当前 `sheet import` 不支持 xlsm/csv |

`sheet` 仅支持在线电子表格（`contentType=ALIDOC`、`extension=axls`）。只有用户给出未知类型 URL/ID 时才调用一次 `dws drive info --node <URL_OR_ID> --format json` 探测；刚由 `sheet create` 返回的资源无需再次 probe。`spreadsheetv2` URL 原样传入 `--node`，不要截短。

只有运行时仍无法识别 URL 形态时才按需查看 [链接规范](../url-patterns.md)；只有上表无法判定的低频产品歧义才查看 [局部意图消歧](../intent-guide.md)。这两份都不是冷启动必读项。

## 常用闭环

### 1. 创建

| 目标 | 命令 |
|---|---|
| 只建空表格 | `dws sheet create --name <NAME> --format json` |
| 新建并写入初始二维数据 | `dws sheet create-with-data --name <NAME> --values '<2D_JSON>' --format json` |
| 新建多个 typed 工作表 | `dws sheet create-with-data --name <NAME> --sheets '<SPECS_JSON>' --format json` |
| 浏览当前可用模板 | `dws sheet template list --format json` |
| 按关键词搜索模板 | `dws sheet template search --query <TEXT> --format json` |
| 用模板创建在线表格 | `dws sheet template apply --template-id <TEMPLATE_ID> --name <NAME> --format json` |

有初始数据时优先 `create-with-data`，它会创建、定位默认工作表、写入并读回，减少独立调用。空表才用 `create`。后续始终复用返回的真实 `nodeId`；不要从 URL 文本或历史会话猜 ID。

模板意图先用 `list` 或 `search` 取得唯一的真实 `templateId`；零个或多个候选时停止并消歧，不能把模板名称猜成 ID。`apply` 会新建在线表格，后续复用其返回的节点信息；不要再执行一次普通 `sheet create`。

### 2. 定位工作表

- 后续命令不要求 `sheet-id` 时不要为了“保险”调用 `list`。
- 需要 `sheet-id` 且当前结果没有返回时，用 `dws sheet +list-sheets --node <NODE_ID> --format json`；按完整标题唯一匹配，不猜 `Sheet1`、`0`、`default`。
- 合并、冻结、行列尺寸/隐藏/分组等结构信息用 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json`，不要从 CSV 空值推断。

### 3. 读写选择

| 目标 | 首选命令 | 说明 |
|---|---|---|
| Agent 快速读值 | `dws sheet csv-get --node <NODE_ID> --sheet-id <SHEET_ID> --range <A1> --format json` | token 最低；关注 `hasMore`、`returnedRange`、`truncationReasons` |
| 严格完整读取范围 | `dws sheet +read --node <NODE_ID> --sheet-id <SHEET_ID> --range <A1> --format json` | 截断失败关闭 |
| typed table/dataframe | `dws sheet table-get` / `table-put` | 用于 columns/data/dtypes/formats；不塞进 `batch-update` |
| 少量值、富文本、链接、数据验证 | `dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range <A1> --values '<2D_JSON>' --format json` | `--values` 维度必须与范围一致 |
| 超过 5 行或 20 单元格的纯值/公式 | `dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 --csv - --format json` | stdin 优先；覆盖已有数据时显式 `--allow-overwrite` |
| 末尾追加记录 | `dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> --values '<2D_JSON>' --format json` | 不手算最后一行 |

长数字 ID、订单号、手机号及超过 `9007199254740991` 的整数按文本写入，避免 JSON number 精度损失。公式以 `=` 开头；需要字面量 `=` 时前加单引号。

### 4. 最小验证

- 值写入：只回读受影响范围，优先 `csv-get`；需要公式文本用 `--value-render-option formula`，需要真实计算值用 `raw_value`。
- 公式任务：写后先确认公式文本，再执行 `formula-verify`；只有 `status=success`、`hasMore=false`、`totalErrors=0` 才能说目标范围未发现公式错误，这仍不等于业务数值一定正确。
- 结构修改：用 `sheet info` 或对应对象 `list/get`；图表、透视表、筛选、评论、条件格式等不能只凭写响应断言完成。
- 多个相互依赖的修改尽量使用服务端原子 `batch-update`；不支持的对象按依赖顺序执行，并在最后合并验证，避免每一步都全表回读。

### 5. 本地交付与清理

| 用户要的文件 | 正确命令 | 不要做 |
|---|---|---|
| 单个工作表的纯 RFC4180 CSV | `dws sheet export-csv --node <NODE_ID> --sheet-id <SHEET_ID> --output ./data.csv` | 不要给 `sheet export` 猜 `--format csv`；不要把带 `[row=N]` 的 `csv-get` 输出冒充纯 CSV |
| 整个工作簿 xlsx | `dws sheet export --node <NODE_ID> --output ./result.xlsx` | 不要自行重复提交或轮询导出任务 |
| 只供 Agent 阅读 | `dws sheet csv-get ...` | 不需要落盘 |

`export-csv` 默认遇到截断就失败且不覆盖已有文件；只有用户明确接受不完整 CSV 时才加 `--allow-truncated`。先根据命令回执确认目标文件成功落盘且非空，再做清理。

清理不是 `finally`：只有导出命令成功，且已验证本地文件存在、非空、可交付后，才进入清理步骤。导出失败或本地文件不可验证时保留在线节点并报告失败。

用户要求清理时，只能针对本任务创建且 ID 已确认的在线节点。是否需要确认以 `drive +delete` 的 Runtime gate/Schema 为准；需要确认时先向用户说明节点、动作和不可见影响，取得明确确认后，才在下面这条已核对命令上追加 `--yes`：

```bash
dws drive +delete --node <NODE_ID> --format json
```

不要把原请求中的“导出后删除”自动等同于 Runtime 确认，也不要在存储示例中预置 `--yes`。命令和参数已明确时无需读取 Drive reference 或 Help；只有安全语义仍不确定时读取一次该 leaf 的 compact Schema。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "sheet +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws sheet <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service sheet --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws sheet +list-sheets` | read | 严格列出在线电子表格的工作表，并可按完整标题精确筛选 |
| `dws sheet +read` | read | 完整读取并严格校验在线电子表格范围；截断结果失败关闭 |
<!-- VISIBLE_SHORTCUTS_END -->

## 复杂操作：按阶段加载 reference

先用本文件完成路由。只有执行即将进入一个复杂阶段时，才读取该阶段对应的一份子 reference；完成阶段后保留 `nodeId`、`sheetId`、对象 ID、已验证范围和 revision，再进入下一阶段。一个任务可以顺序读取多份，但禁止冷启动并行预读、重复读取已经加载的文件，或因“可能用到”提前加载。常规任务最多三个复杂阶段；超过时应先合并同类操作并复用已加载契约。

| 当前执行阶段 | 本阶段唯一子 reference | 原生命令族 |
|---|---|---|
| 浏览、搜索或应用模板 | 本文件创建闭环（无需子 reference） | `template list/search/apply` |
| 工作表增删改、冻结、合并边界、网格线 | [sheet-workbook](sheet/sheet-workbook.md) | `new` / `update` / `copy` / `delete-sheet` / `info` |
| 读取元数据、分页、大范围值 | [sheet-read-data](sheet/sheet-read-data.md) | `csv-get` / `table-get` / `range read` |
| 富格式值、超链接、数据验证、typed 写入 | [sheet-write-data](sheet/sheet-write-data.md) | `range update` / `csv-put` / `table-put` / `append` |
| 公式写入、文本回读、错误扫描 | [sheet-formula](sheet/sheet-formula.md) | `range update` / `formula-verify` |
| 查找或替换 | [sheet-search-replace](sheet/sheet-search-replace.md) | `find` / `replace` |
| 清空、排序、填充、复制/移动区域 | [sheet-range-operations](sheet/sheet-range-operations.md) | `range clear/sort/fill/copy-to/move-to` |
| 多个原子写组合 | [sheet-batch-operations](sheet/sheet-batch-operations.md) | `batch-update` / `range batch-clear` |
| 行列插删、尺寸、隐藏、移动、分组 | [sheet-dimension-operations](sheet/sheet-dimension-operations.md) | dimension 命令族 |
| 样式、数字格式、合并 | [sheet-style-format](sheet/sheet-style-format.md) | `range set-style` / `merge-cells` |
| 下拉选项 | [sheet-dropdown](sheet/sheet-dropdown.md) | dropdown 命令族 |
| 筛选 | [sheet-filter](sheet/sheet-filter.md) | `filter` 命令族 |
| 个人筛选视图 | [sheet-filter-view](sheet/sheet-filter-view.md) | `filter-view` 命令族 |
| 条件高亮、色阶、数据条 | [sheet-conditional-format](sheet/sheet-conditional-format.md) | `cond-format` 命令族 |
| 图表 | [sheet-chart](sheet/sheet-chart.md) | `chart` 命令族 |
| 透视表 | [sheet-pivot-table](sheet/sheet-pivot-table.md) | `pivot-table` 命令族 |
| 评论、回复、更新、删除评论 | [sheet-comment](sheet/sheet-comment.md) | `comment` 命令族 |
| 图片与附件 | [sheet-media-image](sheet/sheet-media-image.md) | `write-image` / float-image 命令族 |
| 在线历史版本保存、列表、恢复 | [sheet-version](sheet/sheet-version.md) | `version save/list/revert` |
| 当前 revision 与编辑审计 | [sheet-revision-changeset](sheet/sheet-revision-changeset.md) | `revision-get` / `changeset-get` |
| 导入或导出边界/失败恢复 | [sheet-export](sheet/sheet-export.md) 或 [sheet-import](sheet/sheet-import.md) 中与意图匹配的一份 | `export` / `export-csv` / `import create/get` |

例如“写公式 → 设置样式 → 创建图表”可在进入三个阶段时依次加载 `sheet-formula`、`sheet-style-format`、`sheet-chart`，但每一阶段只读一份，且下一份必须等上一阶段写入/验证完成后再读。若当前 reference 已能完成后续动作，不再加载；契约错误恢复仍只补读与报错 leaf 精确对应的一份。

## 版本与 revision 边界

- 在线 Sheet 历史版本只用 `dws sheet version save/list/revert`。恢复是破坏性操作，必须确认精确目标版本；不要用 Doc 的 version 命令。
- `revision-get` / `changeset-get` 是编辑审计和前向语义变化，不是可恢复的历史快照，也不保证是当前最终值。
- 不要用 AITable schema snapshot 冒充 Sheet 历史版本。用户明确要求在线电子表格版本时，产品边界优先于“结构化”措辞。
- 恢复或审计后仍需回读当前目标范围；changeset 不能代替最终值。

## 完成检查

在最终答复前只做一次紧凑检查：

- 资源类型和 profile 正确，所有 ID 来自本任务真实返回。
- 创建/修改、对象存在性与关键值已有匹配读回；公式没有被普通值回读误判。
- 用户要求的本地 CSV/xlsx 已成功导出，格式与扩展名一致。
- 若要求清理，本任务创建的精确在线节点已移入回收站；本地交付物仍保留。
- 最终答复附上真实在线链接或本地路径，只报告证据支持的行数、对象数、公式状态和清理状态。

## 最短错误恢复

- 参数校验失败：只修正错误指出的 leaf 参数后重试一次；不要改读父级 Help。
- `hasMore=true` / 截断：按 `returnedRange` 分块续读；不得把部分结果声称为完整。CSV 落盘默认失败关闭。
- 导出 xlsx 失败或超时：不要重复提交 `sheet export`；保留在线文档并报告。
- `create-with-data` 在 create / write / style 间不是原子事务。若错误 `details.status` 为 `unknown` 或 `partial_success`，复用 `details.nodeId` / `details.sheetId` 读回现状，只补失败步骤；不得整体重跑 `create-with-data`，也不得自动删除已写数据。
- 写入部分成功：先读回确定真实状态，再续做缺失部分；不要盲目重放非幂等创建。
- 权限或认证失败：停止业务重试并报告；不要切换产品绕过权限。
