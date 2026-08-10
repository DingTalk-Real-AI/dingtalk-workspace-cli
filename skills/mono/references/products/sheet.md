# 电子表格 (sheet) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断 + 全局约束），各命令的详细参数、示例和注意事项在 [sheet/](./sheet/) 目录下按需加载。

## 适用范围

`sheet` 仅支持钉钉在线电子表格（`contentType=ALIDOC`、`extension=axls`）。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令 |
| `xlsx` / `xls` / `xlsm` / `csv` | `dws drive download --node <ID> --output <路径>` 下载到本地处理，禁止调用任何 `sheet` 子命令 |
| 本地 xlsx/xls 导入为在线表格 | `dws sheet import create --file <路径>`（单命令一站式：创建会话→上传→确认→轮询，产出在线电子表格；`sheet import` 保留兼容） |
| 在线表格导出为 xlsx | `dws sheet export`（axls → xlsx 格式转换） |

用户贴原始 `alidocs` URL 时必须先 probe：`dws drive info --node <URL> --format json`，按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 校验：
- `extension=axls` → 继续走 `sheet`
- `extension=xlsx` / `xls` / `xlsm` / `csv` → 转 `dws drive download`，告知用户"这是本地表格文件，已为你下载到本地处理"

## URL → NODE_ID

| URL 格式 | 提取方式 |
|----------|---------|
| `.../i/nodes/{id}` 或 `.../i/nodes/{id}?query` | 取路径末段作 NODE_ID（忽略 query） |
| `.../spreadsheetv2/{key}/...` | **完整 URL 原样传 `--node`**，禁止提取 path segment |

参数不确定时先 `dws sheet <命令> --help`。

## Agent 编辑总则

1. **先定位再编辑**：未知 `sheet-id` 时必须先 `dws sheet list --node <ID> --format json`，禁止猜 `Sheet1` / `sheet1` / `0` / `default`。用户只给工作表名称时，也先用 `list` 确认真实名称或 ID。
2. **先读结构再动结构**：合并、冻结、行列分组、行高列宽、隐藏行列、最后非空边界都属于工作表结构信息，先读 `dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json`；分组回读加 `--include groups`，行高、列宽和隐藏行列按需加 `--include row_heights,col_widths,hidden_rows,hidden_cols`。
3. **按目的选择读取方式**：快速看值和大表分批用 `csv-get`；需要 `columns` / `data` / `dtypes` / `formats` 用 `table-get`；需要公式、样式、数据验证、超链接、富文本等 per-cell 元数据用 `range read`。
4. **写返回不等于完成**：任何写操作完成后都要用独立读命令回读确认。值写入用 `csv-get` / `range read` / `table-get`，结构变更用 `sheet info`，对象类操作用对应 `list` / `get`。
5. **大批量 CSV 值/公式不要拼大 JSON**：超过 5 行或 20 个单元格、且不需要富格式对象时优先 `csv-put`；字段值以 `=` 开头按公式解析，前加单引号写入以 `=` 开头的字面文本；需要 table/dataframe 语义时用 `table-put`。
6. **公式写入后分层校验**：写公式前读 [sheet-formula](./sheet/sheet-formula.md)。写后先用 `range read --value-render-option formula` 确认公式文本，再用 `formula-verify` 聚合扫描计算错误；需要确认业务数值时再用 `raw_value` 抽样对账。
7. **专用操作用专用命令**：搜索用 `find`、替换用 `replace`、清空用 `range clear`、排序用 `range sort`、复制/移动区域用 `range copy-to` / `range move-to`，不要用 `range read` + `range update` 客户端模拟。
8. **大整数按文本写**：超过 `9007199254740991` 的整数、长数字 ID、订单号、手机号等需要逐位精确的值，不要按 JSON number 或 `int64` / `uint64` 写入；用字符串值 + `object` dtype + 文本格式 `@`。

## 场景速查

| 用户要做 | 正确命令 | 动手前读 | 禁止写法 |
|---------|---------|----------|----------|
| 快速查看数据 / 大表分批读取 | `csv-get` | [sheet-read-data](sheet/sheet-read-data.md) | 不传 `--range` 全量 `range read` 大表 |
| 按表格结构读写列名、类型、格式 | `table-get` / `table-put` | [sheet-read-data](sheet/sheet-read-data.md)、[sheet-write-data](sheet/sheet-write-data.md) | 把 table spec 塞进 `batch-update` |
| 少量精确写入、公式对象、超链接、富文本、数据验证 | `range update` | [sheet-write-data](sheet/sheet-write-data.md)、[sheet-formula](sheet/sheet-formula.md) | 用 `append` 写公式，或用 `csv-put` 写富格式 |
| 校验公式错误 / 全表公式检查 | `formula-verify` | [sheet-formula](sheet/sheet-formula.md) | 全表 `range read` 后由 Agent 自行枚举错误值 |
| 批量 CSV 值/公式写入 / CSV 粘贴 | `csv-put` | [sheet-write-data](sheet/sheet-write-data.md)、[sheet-formula](sheet/sheet-formula.md) | 为大块 CSV 数据手写巨大 `--values` JSON |
| 追加记录到末尾 | `append` | [sheet-write-data](sheet/sheet-write-data.md) | 手算最后一行后 `range update` |
| 查找 / 替换 | `find` / `replace` | [sheet-search-replace](sheet/sheet-search-replace.md) | 读全表后本地过滤或手写替换 |
| 清空 / 排序 / 填充 / 复制移动区域 | `range clear` / `range sort` / `range fill` / `range copy-to` / `range move-to` | [sheet-range-operations](sheet/sheet-range-operations.md) | 用读写组合模拟服务端原子操作 |
| 合并、冻结、分组、行高列宽 | `sheet info` + 结构命令 | [sheet-workbook](sheet/sheet-workbook.md)、[sheet-dimension-operations](sheet/sheet-dimension-operations.md) | 从 `range read` / CSV 空值推断结构 |
| 多个原子写操作组合 | `batch-update` | [sheet-batch-operations](sheet/sheet-batch-operations.md) | 多次独立调用导致半成品 |
| 图片写入单元格 / 浮动图片 | `write-image` / `media-upload` + `create-float-image` | [sheet-media-image](sheet/sheet-media-image.md) | 用 `range update` 写图片 |
| 条件高亮 / 标红 / 数据条 / 色阶 | `cond-format` | [sheet-conditional-format](sheet/sheet-conditional-format.md) | 用静态 `set-style` 冒充条件格式 |
| 单元格评论 / 批注 / @人讨论 | `comment list/create/reply/update/delete` | [sheet-comment](sheet/sheet-comment.md) | 把评论内容写进单元格值 |

## 触发必读

- 涉及公式、计算列、辅助列、占比、增长率、查找计算、公式校验、公式错误扫描：先读 [sheet-formula](./sheet/sheet-formula.md)。
- 涉及 `range update --values`、超链接、富文本、数据验证、少量样式随值写入：先读 [sheet-write-data](./sheet/sheet-write-data.md)。
- 涉及公式文本 / 原始值 / 格式化值回读、per-cell 元数据、大表分页：先读 [sheet-read-data](./sheet/sheet-read-data.md)。
- 涉及合并、冻结、工作表增删改、最后非空边界：先读 [sheet-workbook](./sheet/sheet-workbook.md)。
- 涉及行列插删、隐藏、尺寸、移动、分组：先读 [sheet-dimension-operations](./sheet/sheet-dimension-operations.md)。
- 涉及多个写操作一次提交：先读 [sheet-batch-operations](./sheet/sheet-batch-operations.md)。

## Reference 索引

| Reference | 描述 |
|-----------|------|
| [sheet-workbook](sheet/sheet-workbook.md) | 管理表格文档与工作表。当用户说"创建表格"、"有哪些工作表"、"新建/重命名/隐藏/冻结/复制/删除工作表"、"显示/隐藏网格线"时使用；`info` 可回读冻结行列，`--include groups` 回读分组，其他行列结构按需使用对应 `--include`。命令：`create`/`list`/`info`/`new`/`update`/`copy`/`delete-sheet`/`show-gridline`/`hide-gridline` |
| [sheet-read-data](sheet/sheet-read-data.md) | 读取工作表数据。当用户说"读数据"、"看表格内容"、"查看数据"时使用。推荐 `csv-get`（CSV 格式、token 低、防爆保护）；需按 table/dataframe 结构读取列名、data、dtypes、formats 时用 `table-get`；需 value + dataValidation / hyperlink / richText / cellStyles 等 per-cell 元数据时用 `range read`。大范围数据建议分页读取（单次 ≤5000 单元格）。命令：`csv-get`/`table-get`/`range read` |
| [sheet-write-data](sheet/sheet-write-data.md) | 写入数据到工作表。当用户说"写数据"、"填表"、"更新单元格"、"写公式"、"超链接"、"写值同时设样式/数据验证"、"追加数据"、"导入CSV"、"写入结构化 table"时使用。大批量 CSV 值/公式（>5行或>20单元格，且无需富格式）必须用 `csv-put` 而非 `range update`；结构化 table/dataframe 输入用 `table-put`。命令：`range update`/`append`/`csv-put`/`table-put` |
| [sheet-formula](sheet/sheet-formula.md) | 公式写入、文本回读、错误聚合与结果抽样。当用户说"写公式"、"计算列"、"辅助列"、"总计/占比/增长率/查找计算"、"校验公式"、"检查公式错误"时使用。命令：`range update` / `csv-put` / `range read --value-render-option formula/raw_value` / `formula-verify` |
| [sheet-search-replace](sheet/sheet-search-replace.md) | 搜索和替换文本。当用户说"搜索"、"查找"、"替换"、"把A改成B"时使用。禁止用 `range read` 全量读取后客户端过滤代替 `find`，禁止用 `range update` 模拟 `replace`。命令：`find`/`replace` |
| [sheet-range-operations](sheet/sheet-range-operations.md) | 区域结构性操作。当用户说"清空"、"排序"、"自动填充"、"复制区域到"、"移动数据到"时使用。均为服务端原子操作，禁止 `range read`+`range update` 组合模拟。排序前必须先读前几行判断表头。命令：`range clear`/`range sort`/`range fill`/`range copy-to`/`range move-to` |
| [sheet-batch-operations](sheet/sheet-batch-operations.md) | 批量操作。当用户说"批量清除多个区域"、"组合多个写操作"、"先清除再写入"、"插行列再写数据"、"批量创建/取消分组"时使用。`batch-update` 只支持已列明的原子写操作；`table-put` / `table-get` 不放进 batch，结构化 table 请用独立 `table-put`。命令：`range batch-clear`/`batch-update` |
| [sheet-dimension-operations](sheet/sheet-dimension-operations.md) | 行列增删移动、属性设置与分组。当用户说"插入行/列"、"删除行/列"、"隐藏/显示行列"、"设行高/列宽"、"移动行/列"、"追加空行/空列"、"创建/取消行列分组"、"新建分组并设为折叠/展开"时使用。命令：`insert-dimension`/`delete-dimension`/`update-dimension`/`move-dimension`/`add-dimension`/`group-dimension`/`ungroup-dimension` |
| [sheet-style-format](sheet/sheet-style-format.md) | 单元格样式与合并。当用户说"设样式"、"改颜色/字体/对齐"、"数字格式(百分比/货币/日期)"、"合并/取消合并"时使用。纯样式/批量样式走 `set-style`；写值同时设置少量 cell 样式可用 `range update` 的 `cellStyles`。命令：`range set-style`/`range batch-set-style`/`merge-cells`/`unmerge-cells` |
| [sheet-dropdown](sheet/sheet-dropdown.md) | 下拉列表管理。当用户说"设置下拉"、"下拉选项"、"删除下拉"时使用。命令：`set-dropdown`/`get-dropdown`/`delete-dropdown` |
| [sheet-media-image](sheet/sheet-media-image.md) | 附件上传与图片。当用户说"上传附件"、"写入图片到单元格"、"浮动图片"时使用。单元格图片用 `write-image`（禁止 `range update`）；浮动图片需先 `media-upload` 再 `create-float-image`。命令：`media-upload`/`write-image`/`create-float-image`/`get-float-image`/`list-float-images`/`update-float-image`/`delete-float-image` |
| [sheet-filter](sheet/sheet-filter.md) | 全局筛选。当用户说"筛选"、"过滤"、"只看某些行"（未说"筛选视图"）时使用。禁止用"删除不符合条件的行"代替筛选。命令：`filter get`/`create`/`delete`/`update`/`clear-criteria`/`sort` |
| [sheet-filter-view](sheet/sheet-filter-view.md) | 筛选视图（个人化，不影响协作者）。当用户明确说"筛选视图"时使用，与全局筛选相互独立。命令：`filter-view list`/`create`/`update`/`delete`/`info`/`update-criteria`/`delete-criteria`/`list-criteria`/`get-criteria` |
| [sheet-conditional-format](sheet/sheet-conditional-format.md) | 条件格式规则。触发词：标红/标黄/高亮/突出/标记/数据条/色阶/颜色随数据变 → **强制**走条件格式，禁止 `range set-style` 静态样式替代。命令：`cond-format list`/`create`/`update`/`delete` |
| [sheet-export](sheet/sheet-export.md) | 导出表格为 xlsx。当用户说“导出”、“下载xlsx”、“存为Excel”时使用。单命令一站式，CLI 内部自动轮询，禁止 Agent 侧重试。命令：`export` |
| [sheet-import](sheet/sheet-import.md) | 导入本地表格文件为在线电子表格。当用户说“导入表格”、“把xlsx传成在线表格”、“上传Excel变在线表格”时使用。Agent 入口为 `import create`，`import` 保留兼容；超时后只用 `import get` 续查，禁止重新创建或用 `drive upload` 代替。命令：`import create`/`import get` |
| [sheet-chart](sheet/sheet-chart.md) | 浮动图表管理。当用户说“画图/数据可视化/趋势图/对比图/占比图/柱形图/折线图/饼图”时使用。禁止用本地脚本生成静态图片替代。命令：`chart list`/`create`/`update`/`delete` |
| [sheet-pivot-table](sheet/sheet-pivot-table.md) | 透视表管理。当用户说“透视表/分组汇总/交叉分析/按X统计Y”时使用。禁止用公式拼汇总表替代。命令：`pivot-table list`/`create`/`update`/`delete` |
| [sheet-comment](sheet/sheet-comment.md) | 单元格评论管理。当用户说“给单元格加评论/批注”、“看某格的评论”、“回复/更新/删除评论”、“@某人讨论这个数据”时使用。评论锚定单元格（`--sheet-id`+`--range`），禁止把评论写进单元格值。命令：`comment list`/`create`/`reply`/`update`/`delete` |
| sheet-template | 表格模板管理。当用户说“用模板创建表格”、“搜索模板”、“模板列表”时使用。命令：`template list`/`template search`/`template apply` |
| [sheet-version](./sheet/sheet-version.md) | 表格历史版本管理。当用户说“保存版本/存个快照/看历史版本/版本列表/回滚到某个版本/恢复到之前的表格”时使用。回滚为危险操作，默认二次确认；版本号从 `version list` 获取。命令：`version save`/`version list`/`version revert` |

## 全局硬约束

1. **`--sheet-id` 禁止臆测**：未知时必须 `dws sheet list --node <ID> --format json` 查询，禁止编造 `Sheet1`/`sheet1`/`0`/`default`
2. **合并单元格是结构信息**：`dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json` 返回 `mergedRanges`（如 `["C7:D11"]`）；不要在 `range read` / `csv-get` 里寻找合并信息
3. **冻结行列是工作表元数据**：`sheet info` 顶层返回 `frozenRowCount` / `frozenColumnCount`，分别表示从顶部第 1 行、左侧第 A 列开始冻结的数量；`0` 表示未冻结。不要在 `range read` / `csv-get` 里寻找冻结信息
4. **结构变更后按需 include 回读**：创建/取消分组后，用 `dws sheet info --node <ID> --sheet-id <SHEET_ID> --include groups --format json` 回读 `rowGroups` / `columnGroups`；行高、列宽、隐藏行列分别使用 `--include row_heights` / `col_widths` / `hidden_rows` / `hidden_cols`。裸 `sheet info` 不承诺返回这些可选结构字段。
5. **最后非空坐标只用 A1 语义**：`sheet info` 通过 `nonEmptyRange` 返回 A1/UI 边界；优先使用 `nonEmptyRange.range`，需要追加行/列时使用 `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn`。不要使用旧的 0-based 字段 `lastNonEmptyRow` / `lastNonEmptyColumn`
6. **`range update` 维度校验**：`--values` 行列数必须与 `--range` 完全一致；只接 `--values` 一个数据参数，cell `type` 仅支持 `text` / `richText`；整格超链接通过 cell-level `hyperlink` 表达，富文本片段链接才使用 `richText.texts[].type="link"`
7. **dataValidation 三语义**：不传 `dataValidation` 字段=保留原 DV；`dataValidation:{type:"none"}`=显式清除；`dataValidation:{type:"dropdown"/"checkbox",...}`=覆盖。`{}` 跳过亦保留原 DV
8. **hyperlink 三语义**：不传 `hyperlink` 字段=保留原整格超链接；`hyperlink:{type:"none"}`=显式清除；`hyperlink:{type:"path"/"sheet"/"range",link,...}`=覆盖。Agent 调用不要用 `hyperlink:null`，避免网关/Schema 过滤 null 字段
9. **样式写法**：cell-level 样式用 `cellStyles` 或 `range set-style`；richText 片段级样式才用子项 `style`。不要在 `type:"text"` 顶层使用旧 `style` 字段
10. **用专用命令不用组合模拟**：搜索→`find`、替换→`replace`、清空→`range clear`、排序→`range sort`、填充→`range fill`、复制区域→`range copy-to`、移动区域→`range move-to`、移动行列→`move-dimension`
11. **大批量 CSV 值/公式用 `csv-put`**（>5 行或 >20 单元格，且无需富格式），不用 `range update`；需要 dataframe/table 语义（列名、dtypes、formats、跨 sheet specs）时用 `table-get` / `table-put`
12. **单元格图片用 `write-image`**（`range update` 不支持图片参数）
13. **`export` / `import create` 禁止自行轮询**（CLI 内部已完成渐进式退避，最多 30 次约 5 分钟）；导入超时只按返回的 `next_command` 调用 `import get`，导出失败或超时直接转述，不自动重调
14. **单次调用上限**：`range update` / `set-style` 行数 ≤ 1000，单元格总数建议 ≤ 5000（硬限 30000）
15. **大整数精度保护**：超过 `9007199254740991` 的整数和长数字标识符必须按文本写入；`table-put` 中不要使用 JSON number 或 `int64` / `uint64` dtype，改用字符串值 + `object` dtype + `formats`/`cellStyles` 的 `@`
16. **批量写操作推荐用 `batch-update`**：对多个区域重复调用同一写入工具时，推荐合并为单次原子请求（详见 [sheet-batch-operations](./sheet/sheet-batch-operations.md)）；逐个调用非原子，中途失败会留下半成品。结构化 table 例外，`table-put` 不进 `batch-update`
17. **关键区分**：sheet（电子表格/单元格读写）vs aitable（AI多维表/结构化记录）vs doc（文档）

## 当前能力边界

- **公式**：支持通过 `range update` 写入、通过 `range read --value-render-option formula` 回读公式文本、通过 `formula-verify` 按错误类型聚合扫描、通过 `raw_value` / `formatted_value` 回读计算结果。只有 `formula-verify` 返回 `status=success`、`hasMore=false`、`totalErrors=0` 时，才能声称本次目标范围未发现公式错误；这仍不等价于业务数值一定正确。
- **视觉规范**：当前不维护独立的表格视觉方案文档；样式、条件格式、图表分别按对应子文档执行。
- **结构化 table**：`table-get` / `table-put` 是 table/dataframe 语义入口，不嵌入 `batch-update`；需要原子组合时只组合已支持的单元格/结构写操作。
- **历史版本**：支持通过 `version save`/`list`/`revert` 手动保存快照、查看版本列表、回滚到指定版本（底层复用 doc 域版本能力）；回滚为危险操作，需二次确认。
- **单元格评论**：支持通过 `comment create`/`list`/`reply`/`update`/`delete` 管理锚定在 `--sheet-id` + `--range` 的评论；评论与单元格值相互独立，详见 [sheet-comment](sheet/sheet-comment.md)。
- **未暴露或未确认能力**：迷你图、历史 changeset 等能力未在本入口承诺；只有出现稳定 DWS 命令和可回读语义后再补充。

## URL 粘贴场景

用户直接粘贴表格 URL（无其他指令）:
- 先 probe：`dws drive info --node <URL> --format json`
- `extension=axls` → `list` + `range read`（读取第一个工作表数据）
- `extension=xlsx`/`xls`/`xlsm`/`csv` → 转 `dws drive download --node <URL> --output ./`

用户粘贴 URL + 附加指令:
- probe 为 `axls` → 按 Reference 索引路由到对应命令
- probe 为 xlsx/csv → 先 `dws drive download` 下载到本地，严禁调用 sheet 命令

## 导入本地表格

用户说"导入Excel/把xlsx转为在线表格/上传表格并在线编辑"时，用 `dws sheet import create`（单命令一站式；`sheet import` 保留兼容，详见 [sheet-import](./sheet/sheet-import.md)）：
```bash
# 导入本地表格文件为在线电子表格（默认表格名取文件名）
dws sheet import create --file ./data.xlsx

# 指定目标文件夹与导入后表格名
dws sheet import create --file ./report.xls --folder-token <FOLDER_TOKEN> --name "月度报表"

# 导入到指定知识库
dws sheet import create --file ./data.xls --workspace <WORKSPACE_ID>
```
- 支持 `.xlsx` / `.xls`，固定产出**在线电子表格**；只新建、不覆盖已有表格
- CLI 内部自动完成「创建会话→上传→确认→渐进式退避轮询」（最多 30 次约 5 分钟），**禁止 Agent 侧再实现轮询或重试**
- 导入结果返回 `documentUrl` / `nodeId`，`nodeId` 可直接作为后续 `sheet` 命令的 `--node`
- **禁止用 `dws drive upload --convert` 代替**：drive upload 只上传文件、不在表格导入闭环内，产出不保证为可编辑在线电子表格

## nodeId 说明

`--node` 同时支持文档 ID、URL、分享链接。`drive list` 返回中必须用 `fileId`（UUID 格式），禁止用 `dentryId`（纯数字）。

## 模板管理

当用户说“用模板创建表格”、“搜索表格模板”、“有哪些表格模板”时使用。

### 获取表格模板列表

```
Usage:
  dws sheet template list [flags]
Example:
  dws sheet template list
  dws sheet template list --source MY
  dws sheet template list --source PUBLIC
  dws sheet template list --limit 20
Flags:
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

### 搜索表格模板

```
Usage:
  dws sheet template search [flags]
Example:
  dws sheet template search --query "预算"
  dws sheet template search --query "排班表" --limit 10
  dws sheet template search --query "财务" --source PUBLIC
Flags:
      --query string     搜索关键词 (必填)
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

### 应用表格模板

```
Usage:
  dws sheet template apply [flags]
Example:
  dws sheet template apply --template-id TPL_ID --name "月度预算表"
  dws sheet template apply --template-id TPL_ID --name "排班表" --folder FOLDER_ID
Flags:
      --template-id string  模板 ID (必填，从 template list/search 获取)
      --name string         新表格文档名称 (可选)
      --folder string       目标文件夹 ID (可选)
      --workspace string    知识库 ID (可选)
```

---

## SKILL 摘要（原 dingtalk-sheet/SKILL.md 正文）

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "sheet +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws sheet <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service sheet --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws sheet +list-sheets` | read | 获取表格文档中全部工作表列表 |
| `dws sheet +read` | read | 读取工作表指定范围的结构化单元格数据 |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建电子表格" | `dws sheet create --name "<标题>"` |
| "新建工作表" | `dws sheet new --node <nodeId或URL> --name "<sheet名>"` |
| "读取单元格" | `dws sheet range read --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2` |
| "写入单元格" | `dws sheet range update --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2 --values '[[..]]'` |
| "写入超链接" | `dws sheet range update --node <nodeId或URL> --sheet-id <sheetId> --range A1 --values '[[{"type":"text","text":"显示文本","hyperlink":{"type":"path","link":"https://..."}}]]'` |
| "追加一行" | `dws sheet append --node <nodeId或URL> --sheet-id <sheetId> --values '[[..]]'` |
| "查找 / 替换" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<关键词>"` / `dws sheet replace --node <nodeId或URL> --sheet-id <sheetId> --find "<旧值>" --replacement "<新值>"` |
| "精确匹配搜索 / 完全等于" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<关键词>" --match-entire-cell` |
| "搜索公式文本" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<公式片段>" --match-formula` |
| "正则搜索 / 不区分大小写" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<regexp>" --use-regexp --match-case=false` |
| "插入图片到单元格" | `dws sheet write-image --node <nodeId或URL> --sheet-id <sheetId> --range A1 --file <图片路径>` |
| "创建浮动图片" | 先 `dws sheet media-upload --node <nodeId或URL> --file <图片路径>` 获取 `resourceUrl`，再 `dws sheet create-float-image --node <nodeId或URL> --sheet-id <sheetId> --src "<resourceUrl>" --range A1 --width <宽> --height <高>` |

## URL 与 ID 前置

- 用户直接粘贴 `alidocs.dingtalk.com` URL 且意图不明确时，先执行
  `dws drive info --node "<URL>" --format json` 探测；只有 `extension=axls`
  才继续走 `sheet`。
- `spreadsheetv2` 链接不要截取短 path segment，必须把完整 URL 原样传给 `--node`。
- 写入类命令必须先用 `dws sheet list --node <nodeId或URL> --format json` 取得真实 `sheetId`；不要猜 `Sheet1`、`sheet1`、`0`。
- 所有 sheet 子命令使用 `--node` 参数；不要写 `--node-id`、`--file-id` 或把 JSON 字段名 `id` 当成节点值传入。

## 高频硬约束

- `sheet create` 后必须从返回结果提取真实 `nodeId` 或文档 URL，后续 `range update/read` 原样传给 `--node`；如果返回里同时有 `nodeId` 和 `url`，优先用 `nodeId`。
- 写入区域前先 `sheet list --node <nodeId> --format json` 获取真实 `sheetId`；`range update` 成功后默认用独立 `range read` / `csv-get` 回读受影响区域，不能只凭 `success` / `updatedRows` / `updatedCells` / `message` 宣称完成。仅当用户明确要求“不要再读/信任写入返回”时，才跳过回读并说明验证边界。
- `range update` 的 `--values` 必须是二维 JSON，行列数量与 `--range` 完全一致；批量样式用 `range batch-set-style --batch <json文件>`。
- 整格超链接只能用 `range update` 的 cell-level `hyperlink` 字段：`{"type":"text","text":"显示文本","hyperlink":{"type":"path","link":"https://..."}}`。不要走 `doc`，不要用 `--hyperlinks`，不要把整格链接写成 richText 片段链接。
- 搜索必须用 `sheet find`，不要用 `range read` 后本地过滤；“精确/完全匹配/等于”必须加 `--match-entire-cell`，“搜公式”必须加 `--match-formula`，“正则且不区分大小写”必须同时加 `--use-regexp --match-case=false`。
- 同一参数错误最多纠正 2 次；若持续 `nodeId 格式不合法`，回到
  `drive info --node <URL>` 或 `sheet create` 返回重新取 ID，不要无意义重试。

## 跨产品协作

- 多维表 / 字段类型 → 切到 `dingtalk-aitable`
- 把表数据写进文档 → 切到 `dingtalk-doc`

## 局部意图与 Recipe

- [局部意图消歧](../intent-guide.md)。
