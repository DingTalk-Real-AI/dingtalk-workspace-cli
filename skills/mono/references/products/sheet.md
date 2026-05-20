# 在线电子表格 (sheet) 命令参考

> ⚠️ **本文档已对齐开源 dws v1.0.30 实际暴露的 cobra 子命令**。文档中描述的所有命令在该版本下都能 dispatch 成功（PASS），仅业务返回值取决于真实数据 / 权限。

## 适用范围（重要）

`sheet` 产品**仅支持钉钉在线电子表格**（`contentType=ALIDOC`、`extension=axls`），**不支持**上传的 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令（读/写/筛选/合并等服务端原子操作） |
| `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件 | 必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再用本地工具解析，**禁止**调用任何 `sheet` 子命令 |

> 用户直接粘贴 `alidocs` URL 时，先用 `dws doc info --node <URL> --format json` 确认 `contentType=ALIDOC` 且 `extension=axls` 后再走 `sheet`；否则转 `dws doc download`。

## 命令总览（按功能分组）

### 工作表 (Worksheet) 级

| 命令 | 用途 |
|------|------|
| `dws sheet create` | 在知识库中创建一个新的钉钉表格文档 |
| `dws sheet new` | 在已有钉钉表格文档中新建一张工作表 |
| `dws sheet list` | 列出指定文档的所有工作表 |
| `dws sheet info` | 获取指定工作表详情 |

### 区域 (Range) 读写

| 命令 | 用途 |
|------|------|
| `dws sheet range read` | 读取指定区域的单元格内容 |
| `dws sheet range update` | 写入/更新指定区域的单元格 |
| `dws sheet append` | 在工作表末尾追加若干行 |

### 行列 (Dimension)

| 命令 | 用途 |
|------|------|
| `dws sheet add-dimension` | 在末尾追加空行或空列 |
| `dws sheet insert-dimension` | 在指定位置插入空行/空列 |
| `dws sheet delete-dimension` | 删除指定位置起的若干行/列 |
| `dws sheet move-dimension` | 移动行/列到指定位置 |
| `dws sheet update-dimension` | 更新行/列属性（显隐、行高/列宽） |

### 单元格合并

| 命令 | 用途 |
|------|------|
| `dws sheet merge-cells` | 合并指定范围的单元格（`mergeAll`/`mergeRows`/`mergeColumns`） |
| `dws sheet unmerge-cells` | 取消指定范围的合并 |

### 查找/替换

| 命令 | 用途 |
|------|------|
| `dws sheet find` | 在工作表中搜索单元格内容（支持正则/整格匹配/隐藏） |
| `dws sheet replace` | 全局查找替换 |

### 筛选视图 (Filter View) — 命名视图、按列条件、不影响表本身

| 命令 | 用途 |
|------|------|
| `dws sheet filter-view create` | 创建筛选视图 |
| `dws sheet filter-view list` | 列出工作表的所有筛选视图 |
| `dws sheet filter-view update` | 更新筛选视图（名称/范围/条件） |
| `dws sheet filter-view delete` | 删除整个筛选视图 |
| `dws sheet filter-view update-criteria` | 设置/更新视图内某列的筛选条件 |
| `dws sheet filter-view delete-criteria` | 清除视图内某列的筛选条件 |

### 图片

| 命令 | 用途 |
|------|------|
| `dws sheet write-image` | 将已上传的图片资源写入指定单元格（需要 `--resource-id` 或 `--resource-url`） |

## 通用必填参数

绝大多数 `sheet` 命令都需要：

- `--node <NODE_ID>` —— 钉钉表格文档的 nodeId 或 `https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>` URL
- `--sheet-id <SHEET_ID>` —— 工作表 ID，从 `dws sheet list` 拿

例外：
- `create` 只需要 `--name`（在知识库创建文档时不需要 nodeId）
- `list` / `info` / `range read` 只需要 `--node`

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
  --find "TODO" --match-case false --format json

# 全局替换
dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> \
  --find "TODO" --replacement "DONE" --format json
```

### 筛选视图

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

### 写入图片

```bash
# 已有图片资源 ID 和 URL 后写入单元格
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "B2:B2" --resource-id <RES_ID> --resource-url <RES_URL> \
  --width 200 --height 100 --format json
```

## 易混淆点

| 区分 | 说明 |
|---|---|
| `dws sheet create` vs `dws sheet new` | `create` 在知识库**新建一个文档**（返回新 nodeId）；`new` 在**已有文档中新建一张工作表**（需 nodeId） |
| `filter-view update-criteria` vs `filter-view delete-criteria` | update 是设置/覆盖列条件；delete 是清除列条件（视图本身保留）；要删整个视图用 `filter-view delete` |
| `dws sheet write-image` vs `range update` | write-image 写入图片（需 resourceId + resourceUrl）；range update 写入文本/数字/公式 |
| `range update` vs `append` | range update 指定区域覆盖；append 在末尾追加行 |
| online axls vs 本地 xlsx | sheet 全部命令只认 axls；本地 xlsx 必须先 `doc download` 再用本地工具解析 |

## 危险操作（必须先向用户确认）

| 命令 | 风险 |
|---|---|
| `delete-dimension` | 删除行/列（含数据），不可恢复 |
| `filter-view delete` | 删除整个筛选视图 |
| `replace` | 全局替换可能影响大量单元格 |
| `unmerge-cells` | 取消合并可能丢失部分单元格内容 |

执行前先 `--dry-run` 预览，并向用户展示操作摘要 + 拿到明确同意，再加 `--yes` 提交。

## 何时**不要**用 sheet

- 用户给的是 `xlsx` / `xls` / `xlsm` / `csv` 本地文件 → 用 `dws doc download` 下载后本地解析
- 用户给的是 AI 表格（不是在线电子表格）→ 用 `dws aitable record query` 等
- 用户给的是富文本/普通文档 → 用 `dws doc read`

## 不支持的能力（开源 dws v1.0.30）

以下命令在内部 wukong 版本可能可用，但**开源 v1.0.30 未暴露**，请勿尝试调用：

- 复制工作表 / 更新工作表元信息（`copy_sheet` / `update_sheet`）
- 表级筛选器（`create_filter / get_filter / update_filter / delete_filter / set_filter_criteria / clear_filter_criteria / sort_filter`）—— 改用筛选视图（`filter-view *`）实现等价功能
- 导出 xlsx（`submit_export_job / query_export_job`）—— 改在钉钉客户端导出
- 浮动图片（float-image 系列）/ 下拉框（dropdown 系列）/ 单元格样式（range set-style 系列）

## 权威参考

- 看某个命令的完整 help：`dws sheet <cmd> --help`
- 看 schema：`dws schema sheet.<tool_name>`
