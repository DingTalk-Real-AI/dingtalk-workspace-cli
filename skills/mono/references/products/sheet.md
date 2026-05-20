# 在线电子表格 (sheet) 命令参考

## 适用范围（重要）

`sheet` 产品**仅支持钉钉在线电子表格**（`contentType=ALIDOC`、`extension=axls`），**不支持**上传的 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令（读/写/筛选/合并/导出等服务端原子操作） |
| `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件 | 必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再用本地工具解析，**禁止**调用任何 `sheet` 子命令 |
| 想把在线表格导出为 xlsx | 用 `dws sheet submit_export_job` 提交导出任务 → 拿到 `jobId` → `dws sheet query_export_job` 轮询直到 `finished` → 用返回的 `downloadUrl` 下载 |

> 用户直接粘贴 `alidocs` URL 时，先用 `dws doc info --node <URL> --format json` 确认 `contentType=ALIDOC` 且 `extension=axls` 后再走 `sheet`；否则转 `dws doc download`。

## 命名风格说明（v1.0.25 envelope 现状）

`sheet` 产品的命令 cli_name **当前同时存在两种风格**——这是 envelope schema 还在演进中、`CLIAliases` (#246) 重命名尚未完成的过渡态：

- **kebab-case** (~17 个)：`add-dimension`、`merge-cells`、`filter-view update-criteria` 等，与 dws 其它产品风格一致
- **snake_case** (~12 个)：`copy_sheet`、`submit_export_job`、`set_filter_criteria` 等，envelope 原始名直透出来

**调用时以 `dws sheet --help` 输出为准**——本文档与 envelope schema 同步，未来命名收敛后会同步更新。所有命令的最终参数名以 `dws sheet <cmd> --help` 和 `dws schema sheet.<canonical_path>` 为准。

## 命令总览（按功能分组）

### 工作表 (Worksheet) 级

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet create` | `sheet.create_workspace_sheet` | 在知识库中创建一个新的钉钉表格文档 |
| `dws sheet new` | `sheet.create_sheet` | 在已有钉钉表格文档中新建一张工作表 |
| `dws sheet list` | `sheet.get_all_sheets` | 列出指定文档的所有工作表 |
| `dws sheet info` | `sheet.get_sheet` | 获取指定工作表详情 |
| `dws sheet copy_sheet` | `sheet.copy_sheet` | 复制工作表（同文档内） |
| `dws sheet update_sheet` | `sheet.update_sheet` | 更新工作表元信息（如改名） |

### 区域 (Range) 读写

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet range read` | `sheet.get_range` | 读取指定区域的单元格内容 |
| `dws sheet range update` | `sheet.update_range` | 写入/更新指定区域的单元格 |
| `dws sheet append` | `sheet.append_rows` | 在工作表末尾追加若干行 |

### 行列 (Dimension)

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet add-dimension` | `sheet.add_dimension` | 在末尾追加空行或空列 |
| `dws sheet insert-dimension` | `sheet.insert_dimension` | 在指定位置插入空行/空列 |
| `dws sheet delete-dimension` | `sheet.delete_dimension` | 删除指定位置起的若干行/列 |
| `dws sheet move-dimension` | `sheet.move_dimension` | 移动行/列到指定位置 |
| `dws sheet update-dimension` | `sheet.update_dimension` | 更新行/列属性（显隐、行高/列宽） |

### 单元格合并

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet merge-cells` | `sheet.merge_cells` | 合并指定范围的单元格（`mergeAll`/`mergeRows`/`mergeColumns`） |
| `dws sheet unmerge-cells` | `sheet.unmerge_range` | 取消指定范围的合并 |

### 查找/替换

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet find` | `sheet.find_cells` | 在工作表中搜索单元格内容（支持正则/整格匹配/隐藏） |
| `dws sheet replace` | `sheet.replace_all` | 全局查找替换 |

### 筛选视图 (Filter View) — 命名视图、按列条件、不影响表本身

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet filter-view create` | `sheet.create_filter_view` | 创建筛选视图 |
| `dws sheet filter-view list` | `sheet.get_filter_views` | 列出工作表的所有筛选视图 |
| `dws sheet filter-view update` | `sheet.update_filter_view` | 更新筛选视图（名称/范围/条件） |
| `dws sheet filter-view delete` | `sheet.delete_filter_view` | 删除整个筛选视图 |
| `dws sheet filter-view update-criteria` | `sheet.set_filter_view_criteria` | 设置/更新视图内某列的筛选条件 |
| `dws sheet filter-view delete-criteria` | `sheet.clear_filter_view_criteria` | 清除视图内某列的筛选条件 |

### 表级筛选 (Filter) — 直接作用于工作表本身的临时筛选

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet create_filter` | `sheet.create_filter` | 在工作表上创建筛选器 |
| `dws sheet get_filter` | `sheet.get_filter` | 获取当前筛选器配置 |
| `dws sheet update_filter` | `sheet.update_filter` | 更新筛选器条件 |
| `dws sheet delete_filter` | `sheet.delete_filter` | 删除筛选器 |
| `dws sheet set_filter_criteria` | `sheet.set_filter_criteria` | 设置某列的筛选条件 |
| `dws sheet clear_filter_criteria` | `sheet.clear_filter_criteria` | 清除某列的筛选条件 |
| `dws sheet sort_filter` | `sheet.sort_filter` | 对筛选范围按指定列排序 |

### 图片

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet write-image` | `sheet.write_image` | 将已上传的图片资源写入指定单元格 |

### 导出（两步原子，**v1.0.25 没有合并的 `export` 命令**）

| 命令 | canonical | 用途 |
|------|-----------|------|
| `dws sheet submit_export_job` | `sheet.submit_export_job` | 提交导出任务，返回 `jobId` |
| `dws sheet query_export_job` | `sheet.query_export_job` | 轮询导出任务状态，完成后返回 `downloadUrl` |

> v1.0.25 envelope 暴露的是这两个**原子动作**。要完整完成"导出 → 下载"流程需要 client 侧轮询 + 调 `downloadUrl`。`CLIToolOverride.Pipeline` (#247) 提供了底层编排能力，但**当前 envelope 还没把这两个动作 Pipeline 化成一条 `dws sheet export` 总命令**。

## 通用必填参数

绝大多数 `sheet` 命令都需要：

- `--node <NODE_ID>` —— 钉钉表格文档的 nodeId 或 `https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>` URL（alias 来自 `nodeId`）
- `--sheet-id <SHEET_ID>` —— 工作表 ID（alias 来自 `sheetId`），从 `dws sheet list` 拿

例外：
- `create` 只需要 `--name`（在知识库创建文档时不需要 nodeId）
- `list` / `info` / `range read` 只需要 `--node`
- `submit_export_job` 只需要 `--node` + `--export-format`（无 sheet-id）
- `query_export_job` 只需要 `--job-id`

## 常用命令示例

### 创建文档 + 新建工作表

```bash
# 在知识库下创建一个钉钉表格文档
dws sheet create --name "销售数据" --workspace <WS_ID> --format json
# 返回的 nodeId 用于后续操作

# 在已有文档中新建一张工作表
dws sheet new --node <NODE_ID> --name "Q1 数据" --format json
```

### 读写区域

```bash
# 读 A1:D10
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" --format json

# 写入 5x4 区域（values 是二维 JSON 数组）
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D5" \
  --values '[["姓名","岗位","入职","薪资"],["张三","研发","2024-01","30000"]]' \
  --format json

# 追加行
dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
  --values '[["李四","产品","2025-03","28000"]]' \
  --format json
```

### 行列操作

```bash
# 在第 5 行处插入 2 个空行
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension rows --position 4 --length 2 --format json

# 末尾追加 3 列
dws sheet add-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension columns --length 3 --format json

# 删除第 10-12 行
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension rows --position 9 --length 3 --format json

# 隐藏 B 列（startIndex=1, length=1）
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension columns --start-index 1 --length 1 --hidden true --format json
```

### 合并/取消合并

```bash
# 合并 A1:C1
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "A1:C1" --merge-type mergeAll --format json

# 取消合并
dws sheet unmerge-cells --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "A1:C1" --format json
```

### 查找/替换

```bash
# 查找
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> \
  --find "TODO" --use-regexp false --match-case false --format json

# 全局替换
dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> \
  --find "TODO" --replacement "DONE" --format json
```

### 筛选视图（推荐：可命名、不破坏原表）

```bash
# 创建筛选视图（范围必须包含表头行）
dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> \
  --name "未完成项" --range "A1:E100" --format json
# 返回的 filterViewId 用于后续 update/delete/criteria 操作

# 列出所有筛选视图
dws sheet filter-view list --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 给视图的某一列设置筛选条件（column 是相对视图范围首列的 0-based 偏移）
dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> \
  --filter-view-id <FV_ID> --column 2 \
  --filter-criteria '{"conditions":[{"type":"TEXT_CONTAINS","values":["pending"]}]}' \
  --format json

# 清除某列的筛选条件（不删除视图本身）
dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> \
  --filter-view-id <FV_ID> --column 2 --format json

# 删除整个筛选视图
dws sheet filter-view delete --node <NODE_ID> --sheet-id <SHEET_ID> \
  --filter-view-id <FV_ID> --format json
```

### 表级筛选（snake_case 系列，直接作用于工作表本身）

```bash
# 创建筛选器（一张表只有一个，再次 create 会替换）
dws sheet create_filter --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "A1:E100" --format json

# 给某列加筛选条件
dws sheet set_filter_criteria --node <NODE_ID> --sheet-id <SHEET_ID> \
  --column 0 --filter-criteria '{...}' --format json

# 按指定列排序
dws sheet sort_filter --node <NODE_ID> --sheet-id <SHEET_ID> --field 0 --format json

# 删除筛选器
dws sheet delete_filter --node <NODE_ID> --sheet-id <SHEET_ID> --format json
```

### 复制工作表

```bash
dws sheet copy_sheet --node <NODE_ID> --sheet-id <SHEET_ID> --format json
# 返回新工作表 ID
```

### 写入图片

```bash
# 已有图片资源 ID 和 URL（通过 drive 或 doc 上传得到）后写入单元格
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "B2:B2" --resource-id <RES_ID> --resource-url <RES_URL> \
  --width 200 --height 100 --format json
```

### 导出 xlsx（两步流程）

```bash
# Step 1: 提交导出任务
JOB=$(dws sheet submit_export_job --node <NODE_ID> --export-format xlsx --format json --jq '.result.jobId')

# Step 2: 轮询任务状态（建议 sleep + 重试）
dws sheet query_export_job --job-id "$JOB" --format json
# 直到返回 status=finished + downloadUrl

# Step 3: 下载（用 curl / dws doc download / 其它工具拉 downloadUrl）
```

## 易混淆点

| 区分 | 说明 |
|---|---|
| `dws sheet create` vs `dws sheet new` | `create` 在知识库**新建一个文档**（返回新 nodeId）；`new` 在**已有文档中新建一张工作表**（需 nodeId） |
| `dws sheet filter-view *` vs `dws sheet create_filter`/`set_filter_criteria` 等 | filter-view 是**命名视图**，多个并存、不影响表本身；filter 是**表级唯一**筛选器，直接作用于工作表显示 |
| `filter-view update-criteria` vs `filter-view delete-criteria` | update 是设置/覆盖列条件；delete 是清除列条件（视图本身保留）；要删整个视图用 `filter-view delete` |
| `dws sheet submit_export_job` + `query_export_job` vs `dws sheet export` | 后者**不存在**于 v1.0.25 envelope。需 client 端自己轮询，或基于 Pipeline (#247) 在 envelope 侧 PR 一条总命令 |
| `dws sheet write-image` vs `range update` | write-image 写入图片（需 resourceId + resourceUrl）；range update 写入文本/数字/公式 |
| `range update` vs `append` | range update 指定区域覆盖；append 在末尾追加行 |
| online axls vs 本地 xlsx | sheet 全部命令只认 axls；本地 xlsx 必须先 `doc download` 再用本地工具解析 |

## 危险操作（必须先向用户确认）

| 命令 | 风险 |
|---|---|
| `delete-dimension` | 删除行/列（含数据），不可恢复 |
| `filter-view delete` | 删除整个筛选视图 |
| `delete_filter` | 删除表级筛选器 |
| `replace` | 全局替换可能影响大量单元格 |
| `unmerge-cells` | 取消合并可能丢失部分单元格内容（钉钉行为依赖合并模式） |
| `update_sheet` | 更新工作表元信息（如改名） |

执行前先 `--dry-run` 预览，并向用户展示操作摘要 + 拿到明确同意，再加 `--yes` 提交。

## 何时**不要**用 sheet

- 用户给的是 `xlsx` / `xls` / `xlsm` / `csv` 本地文件 → 用 `dws doc download` 下载后本地解析
- 用户给的是 AI 表格（不是在线电子表格）→ 用 `dws aitable record query` 等
- 用户给的是富文本/普通文档 → 用 `dws doc read`

## 权威参考

- 列出所有 sheet 工具：`dws schema | jq '.products[] | select(.id=="sheet") | .tools[] | "\(.group) \(.cli_name)"' -r`
- 看某个命令的完整 JSON Schema：`dws schema sheet.<canonical_path>`（如 `dws schema sheet.update_range`）
- 看某个命令的 flag 别名映射：`dws schema sheet.<canonical_path> --jq '.tool.flag_overlay'`
- 看必填字段：`dws schema sheet.<canonical_path> --jq '.tool.required'`
- 命令的人读视图：`dws sheet <cmd> --help`
