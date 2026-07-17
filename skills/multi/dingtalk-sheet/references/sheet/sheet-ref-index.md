# Reference 索引

## Reference 索引

| Reference | 描述 |
|-----------|------|
| [sheet-workbook](./sheet-workbook.md) | 管理表格文档与工作表。当用户说"创建表格"、"有哪些工作表"、"新建/重命名/隐藏/冻结/复制/删除工作表"、"显示/隐藏网格线"时使用；`info` 可回读工作表结构。命令：`create`/`list`/`info`/`new`/`update`/`copy`/`delete-sheet`/`show-gridline`/`hide-gridline` |
| [sheet-read-data](./sheet-read-data.md) | 读取工作表数据。当用户说"读数据"、"看表格内容"、"结构化/DataFrame 读取"时使用。推荐 `csv-get`（CSV 格式、token 低）；需 columns / data / dtypes / formats 时用 `table-get`；需 per-cell 元数据时用 `range read`。大范围数据建议分页读取（单次 ≤5000 单元格）。命令：`table-get`/`csv-get`/`range read` |
| [sheet-write-data](./sheet-write-data.md) | 写入数据到工作表。当用户说"结构化/DataFrame 写入"、"写数据"、"填表"、"写公式"、"超链接"、"追加数据"、"导入CSV"时使用。多工作表结构化写入用 `table-put`；大批量纯值（>5行或>20单元格）必须用 `csv-put` 而非 `range update`。命令：`table-put`/`range update`/`append`/`csv-put` |
| [sheet-formula](./sheet-formula.md) | 公式写入与有限回读校验。当用户说"写公式"、"计算列"、"辅助列"、"总计/占比/增长率/查找计算"时使用。当前无聚合式公式校验工具，写后必须分别回读公式文本和计算结果。命令：`range update` + `range read --value-render-option formula/raw_value` |
| [sheet-search-replace](./sheet-search-replace.md) | 搜索和替换文本。当用户说"搜索"、"查找"、"替换"、"把A改成B"时使用。禁止用 `range read` 全量读取后客户端过滤代替 `find`，禁止用 `range update` 模拟 `replace`。命令：`find`/`replace` |
| [sheet-range-operations](./sheet-range-operations.md) | 区域结构性操作。当用户说"清空"、"排序"、"自动填充"、"复制区域到"、"移动数据到"时使用。均为服务端原子操作，禁止 `range read`+`range update` 组合模拟。排序前必须先读前几行判断表头。命令：`range clear`/`range sort`/`range fill`/`range copy-to`/`range move-to` |
| [sheet-batch-operations](./sheet-batch-operations.md) | 批量操作。当用户说"批量清除多个区域"、"组合多个写操作"、"先清除再写入"、"插行列再写数据"、"批量创建/取消分组"时使用。`batch-update` 只支持已列明的原子写操作。命令：`range batch-clear`/`batch-update` |
| [sheet-dimension-operations](./sheet-dimension-operations.md) | 行列增删移动、属性设置与分组。当用户说"插入行/列"、"删除行/列"、"隐藏/显示行列"、"设行高/列宽"、"移动行/列"、"追加空行/空列"、"创建/取消行列分组"、"新建分组并设为折叠/展开"时使用。命令：`insert-dimension`/`delete-dimension`/`update-dimension`/`move-dimension`/`add-dimension`/`group-dimension`/`ungroup-dimension` |
| [sheet-style-format](./sheet-style-format.md) | 单元格样式与合并。当用户说"设样式"、"改颜色/字体/对齐"、"数字格式(百分比/货币/日期)"、"合并/取消合并"时使用。纯样式/批量样式走 `set-style`；写值同时设置少量 cell 样式可用 `range update` 的 `cellStyles`。命令：`range set-style`/`range batch-set-style`/`merge-cells`/`unmerge-cells` |
| [sheet-dropdown](./sheet-dropdown.md) | 下拉列表管理。当用户说"设置下拉"、"下拉选项"、"删除下拉"时使用。命令：`set-dropdown`/`get-dropdown`/`delete-dropdown` |
| [sheet-media-image](./sheet-media-image.md) | 附件上传与图片。当用户说"上传附件"、"写入图片到单元格"、"浮动图片"时使用。单元格图片用 `write-image`（禁止 `range update`）；浮动图片需先 `media-upload` 再 `create-float-image`。命令：`media-upload`/`write-image`/`create-float-image`/`get-float-image`/`list-float-images`/`update-float-image`/`delete-float-image` |
| [sheet-filter](./sheet-filter.md) | 全局筛选。当用户说"筛选"、"过滤"、"只看某些行"（未说"筛选视图"）时使用。禁止用"删除不符合条件的行"代替筛选。命令：`filter get`/`create`/`delete`/`update`/`clear-criteria`/`sort` |
| [sheet-filter-view](./sheet-filter-view.md) | 筛选视图（个人化，不影响协作者）。当用户明确说"筛选视图"时使用，与全局筛选相互独立。命令：`filter-view list`/`create`/`update`/`delete`/`info`/`update-criteria`/`delete-criteria`/`list-criteria`/`get-criteria` |
| [sheet-conditional-format](./sheet-conditional-format.md) | 条件格式规则。触发词：标红/标黄/高亮/突出/标记/数据条/色阶/颜色随数据变 → **强制**走条件格式，禁止 `range set-style` 静态样式替代。命令：`cond-format list`/`create`/`update`/`delete` |
| [sheet-export](./sheet-export.md) | 导出表格为 xlsx。当用户说“导出”、“下载xlsx”、“存为Excel”时使用。单命令一站式，CLI 内部自动轮询，禁止 Agent 侧重试。命令：`export` |
| [sheet-chart](./sheet-chart.md) | 浮动图表管理。当用户说“画图/数据可视化/趋势图/对比图/占比图/柱形图/折线图/饼图”时使用。禁止用本地脚本生成静态图片替代。命令：`chart list`/`create`/`update`/`delete` |
| [sheet-pivot-table](./sheet-pivot-table.md) | 透视表管理。当用户说“透视表/分组汇总/交叉分析/按X统计Y”时使用。禁止用公式拼汇总表替代。命令：`pivot-table list`/`create`/`update`/`delete` |
| sheet-template | 表格模板管理。当用户说“用模板创建表格”、“搜索模板”、“模板列表”时使用。命令：`template list`/`template search`/`template apply` |
