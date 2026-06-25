# 电子表格 (sheet) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断 + 全局约束），各命令的详细参数、示例和注意事项在 [sheet/](./sheet/) 目录下按需加载。

## 适用范围

`sheet` 仅支持钉钉在线电子表格（`contentType=ALIDOC`、`extension=axls`）。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令 |
| `xlsx` / `xls` / `xlsm` / `csv` | `dws drive download --node <ID> --output <路径>` 下载到本地处理，禁止调用任何 `sheet` 子命令 |
| 本地 xlsx 导入为在线表格 | `dws drive upload --file <路径> --convert`（上传并转换为在线电子表格，转换后可用 `sheet` 命令操作） |
| 在线表格导出为 xlsx | `dws sheet export`（axls → xlsx 格式转换） |

用户贴原始 `alidocs` URL 时必须先 probe：`dws doc info --node <URL> --format json`，按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 校验：
- `contentType=ALIDOC` + `extension=axls` → 继续走 `sheet`
- `extension=xlsx` / `xls` / `xlsm` / `csv` → 转 `dws drive download`，告知用户"这是本地表格文件，已为你下载到本地处理"

## URL → NODE_ID

| URL 格式 | 提取方式 |
|----------|---------|
| `.../i/nodes/{id}` 或 `.../i/nodes/{id}?query` | 取路径末段作 NODE_ID（忽略 query） |
| `.../spreadsheetv2/{key}/...` | **完整 URL 原样传 `--node`**，禁止提取 path segment |

参数不确定时先 `dws sheet <命令> --help`。

## Reference 索引

| Reference | 描述 |
|-----------|------|
| [sheet-workbook](./sheet/sheet-workbook.md) | 管理表格文档与工作表。当用户说"创建表格"、"有哪些工作表"、"新建/重命名/隐藏/冻结/复制/删除工作表"时使用。命令：`create`/`list`/`info`/`new`/`update`/`copy`/`delete-sheet` |
| [sheet-read-data](./sheet/sheet-read-data.md) | 读取工作表数据。当用户说"读数据"、"看表格内容"、"查看数据"时使用。推荐 `csv-get`（CSV 格式、token 低、防爆保护）；需 value + dataValidation / hyperlink / richText / cellStyles 等 per-cell 元数据时用 `range read`。大范围数据建议分页读取（单次 ≤5000 单元格）。命令：`csv-get`/`range read` |
| [sheet-write-data](./sheet/sheet-write-data.md) | 写入数据到工作表。当用户说"写数据"、"填表"、"更新单元格"、"写公式"、"超链接"、"写值同时设样式/数据验证"、"追加数据"、"导入CSV"时使用。大批量纯值（>5行或>20单元格）必须用 `csv-put` 而非 `range update`。命令：`range update`/`append`/`csv-put` |
| [sheet-search-replace](./sheet/sheet-search-replace.md) | 搜索和替换文本。当用户说"搜索"、"查找"、"替换"、"把A改成B"时使用。禁止用 `range read` 全量读取后客户端过滤代替 `find`，禁止用 `range update` 模拟 `replace`。命令：`find`/`replace` |
| [sheet-range-operations](./sheet/sheet-range-operations.md) | 区域结构性操作。当用户说"清空"、"排序"、"自动填充"、"复制区域到"、"移动数据到"时使用。均为服务端原子操作，禁止 `range read`+`range update` 组合模拟。排序前必须先读前几行判断表头。命令：`range clear`/`range sort`/`range fill`/`range copy-to`/`range move-to` |
| [sheet-dimension-operations](./sheet/sheet-dimension-operations.md) | 行列增删移动与属性设置。当用户说"插入行/列"、"删除行/列"、"隐藏/显示行列"、"设行高/列宽"、"移动行/列"、"追加空行/空列"时使用。命令：`insert-dimension`/`delete-dimension`/`update-dimension`/`move-dimension`/`add-dimension` |
| [sheet-style-format](./sheet/sheet-style-format.md) | 单元格样式与合并。当用户说"设样式"、"改颜色/字体/对齐"、"数字格式(百分比/货币/日期)"、"合并/取消合并"时使用。纯样式/批量样式走 `set-style`；写值同时设置少量 cell 样式可用 `range update` 的 `cellStyles`。命令：`range set-style`/`range batch-set-style`/`merge-cells`/`unmerge-cells` |
| [sheet-dropdown](./sheet/sheet-dropdown.md) | 下拉列表管理。当用户说"设置下拉"、"下拉选项"、"删除下拉"时使用。命令：`set-dropdown`/`get-dropdown`/`delete-dropdown` |
| [sheet-media-image](./sheet/sheet-media-image.md) | 附件上传与图片。当用户说"上传附件"、"写入图片到单元格"、"浮动图片"时使用。单元格图片用 `write-image`（禁止 `range update`）；浮动图片需先 `media-upload` 再 `create-float-image`。命令：`media-upload`/`write-image`/`create-float-image`/`get-float-image`/`list-float-images`/`update-float-image`/`delete-float-image` |
| [sheet-filter](./sheet/sheet-filter.md) | 全局筛选。当用户说"筛选"、"过滤"、"只看某些行"（未说"筛选视图"）时使用。禁止用"删除不符合条件的行"代替筛选。命令：`filter get`/`create`/`delete`/`update`/`clear-criteria`/`sort` |
| [sheet-filter-view](./sheet/sheet-filter-view.md) | 筛选视图（个人化，不影响协作者）。当用户明确说"筛选视图"时使用，与全局筛选相互独立。命令：`filter-view list`/`create`/`update`/`delete`/`info`/`update-criteria`/`delete-criteria`/`list-criteria`/`get-criteria` |
| [sheet-conditional-format](./sheet/sheet-conditional-format.md) | 条件格式规则。触发词：标红/标黄/高亮/突出/标记/数据条/色阶/颜色随数据变 → **强制**走条件格式，禁止 `range set-style` 静态样式替代。命令：`cond-format list`/`create`/`update`/`delete` |
| [sheet-export](./sheet/sheet-export.md) | 导出表格为 xlsx。当用户说"导出"、"下载xlsx"、"存为Excel"时使用。单命令一站式，CLI 内部自动轮询，禁止 Agent 侧重试。命令：`export` |

## 全局硬约束

1. **`--sheet-id` 禁止臆测**：未知时必须 `dws sheet list --node <ID> --format json` 查询，禁止编造 `Sheet1`/`sheet1`/`0`/`default`
2. **合并单元格是结构信息**：`dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json` 返回 `mergedRanges`（如 `["C7:D11"]`）；不要在 `range read` / `csv-get` 里寻找合并信息
3. **`range update` 维度校验**：`--values` 行列数必须与 `--range` 完全一致；只接 `--values` 一个数据参数，cell `type` 仅支持 `text` / `richText`；整格超链接通过 cell-level `hyperlink` 表达，富文本片段链接才使用 `richText.texts[].type="link"`
4. **dataValidation 三语义**：不传 `dataValidation` 字段=保留原 DV；`dataValidation:{type:"none"}`=显式清除；`dataValidation:{type:"dropdown"/"checkbox",...}`=覆盖。`{}` 跳过亦保留原 DV
5. **hyperlink 三语义**：不传 `hyperlink` 字段=保留原整格超链接；`hyperlink:{type:"none"}`=显式清除；`hyperlink:{type:"path"/"sheet"/"range",link,...}`=覆盖。Agent 调用不要用 `hyperlink:null`，避免网关/Schema 过滤 null 字段
6. **样式写法**：cell-level 样式用 `cellStyles` 或 `range set-style`；richText 片段级样式才用子项 `style`。不要在 `type:"text"` 顶层使用旧 `style` 字段
7. **用专用命令不用组合模拟**：搜索→`find`、替换→`replace`、清空→`range clear`、排序→`range sort`、填充→`range fill`、复制区域→`range copy-to`、移动区域→`range move-to`、移动行列→`move-dimension`
8. **大批量纯值用 `csv-put`**（>5 行或 >20 单元格），不用 `range update`
9. **单元格图片用 `write-image`**（`range update` 不支持图片参数）
10. **`export` 禁止自行轮询**（CLI 内部已完成渐进式退避，最多 30 次约 5 分钟）
11. **单次调用上限**：`range update` / `set-style` 行数 ≤ 1000，单元格总数建议 ≤ 5000（硬限 30000）
12. **关键区分**：sheet（电子表格/单元格读写）vs aitable（AI多维表/结构化记录）vs doc（文档）

## URL 粘贴场景

用户直接粘贴表格 URL（无其他指令）:
- 先 probe：`dws doc info --node <URL> --format json`
- `extension=axls` → `list` + `range read`（读取第一个工作表数据）
- `extension=xlsx`/`xls`/`xlsm`/`csv` → 转 `dws drive download --node <URL> --output ./`

用户粘贴 URL + 附加指令:
- probe 为 `axls` → 按 Reference 索引路由到对应命令
- probe 为 xlsx/csv → 先 `dws drive download` 下载到本地，严禁调用 sheet 命令

## 导入本地表格

用户说"导入Excel/把xlsx转为在线表格/上传表格并在线编辑"时：
```bash
# 上传并转换为在线电子表格（转换后返回 nodeId，可用 sheet 命令操作）
dws drive upload --file ./data.xlsx --convert

# 指定上传到某个文件夹
dws drive upload --file ./data.xlsx --folder <FOLDER_ID> --convert

# 指定上传到知识库
dws drive upload --file ./data.xlsx --workspace <WS_ID> --convert
```
- `--convert` 是关键参数，不加则仅上传为附件，不会转换为在线电子表格
- 转换后的文档为 `axls` 格式，可用 `sheet` 全部命令操作
- 支持 `.xlsx` / `.xls` / `.csv` 等格式

## nodeId 说明

`--node` 同时支持文档 ID、URL、分享链接。`drive list` 返回中必须用 `fileId`（UUID 格式），禁止用 `dentryId`（纯数字）。
