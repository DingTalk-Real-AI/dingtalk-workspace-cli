# 电子表格 (sheet) 命令参考

## 适用范围（重要）

`sheet` 产品线仅支持钉钉在线电子表格（`contentType=ALIDOC`、`extension=axls`），不支持上传的 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令（读/写/筛选/合并/导出等服务端原子操作） |
| `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件 | 必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再解析处理，禁止调用任何 `sheet` 子命令（sheet 底层 MCP 工具仅认 `axls`，传入 xlsx 节点会直接报错） |
| 想把在线表格导出为 xlsx | 用 `dws sheet export` ——输入是 `axls`，输出是 xlsx（axls → xlsx 的格式转换） |

> 用户直接粘贴原始 `alidocs` URL 时必须先 probe：先执行 `dws doc info --node <URL> --format json`，按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 校验 `contentType` 和 `extension`：
> - 仅当 `contentType=ALIDOC` 且 `extension=axls` 时，才继续走 `sheet`
> - 如果是 `xlsx` / `xls` / `xlsm` / `csv`，立即转向 `dws doc download`，并告知用户"这是本地表格文件，已为你下载到本地处理"

## URL 识别与 NODE_ID 提取

当用户输入包含钉钉文档 URL 时，必须先识别并提取 NODE_ID，再判断意图。

硬性规则：对用户直接给出的原始 `alidocs` URL，不允许直接走 `sheet` 命令，必须先执行：

```bash
dws doc info --node "<URL>" --format json
```

根据返回路由：
- `contentType=ALIDOC` + `extension=axls` → 继续走 `sheet`
- `contentType≠ALIDOC` + `extension=xlsx` / `xls` / `xlsm` / `csv` → 转向 `dws doc download --node <ID> --output <路径>`，禁止调用任何 sheet 子命令
- 其他类型 → 按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 路由

补充：如果这是用户直接提供的原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe 一次确认真实类型，再判断是否继续走 `sheet`。

### 支持的 URL 格式

| 格式 | 示例 | NODE_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段 |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段 |
| `alidocs.dingtalk.com/spreadsheetv2/{key}/...` | `https://alidocs.dingtalk.com/spreadsheetv2/vKJngl50tJN1v3a3/...?dentryKey=vKJngl50tJN1v3a3&type=s` | **不要提取 path segment**，将完整 URL 直接传给 `--node` 参数，由 MCP 服务端解析 |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. **判断 URL 路径格式**：
   - 路径包含 `/i/nodes/` → 取 URL path 的最后一段作为 NODE_ID（去掉 query string 和 fragment）
   - 路径包含 `/spreadsheetv2/` → **不要提取 path segment**，必须将完整 URL 原样传给 `--node` 参数（因为 path 中的短 ID 不是合法的 nodeId，MCP 服务端会自行解析完整 URL）
3. 对于 `/i/nodes/` 格式，提取出的 NODE_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）
4. 对用户直接提供的原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe；只有 probe 确认 `contentType=ALIDOC` 且 `extension=axls` 时，才继续留在 `sheet`；如果 `extension=xlsx` / `xls` / `xlsm` / `csv`，必须转向 `dws doc download`，不能走任何 sheet 命令

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 sheet 下所有子命令
dws sheet --help

# 查看具体命令的完整参数说明
dws sheet range update --help
dws sheet filter-view create --help
dws sheet insert-dimension --help

# 查看子命令组下的所有命令
dws sheet range --help
dws sheet filter-view --help
```

规则：
- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws sheet --help` 查看命令列表

## 命令速查表

| 命令 | 用途 |
|------|------|
| `sheet create` | 创建钉钉表格文档 |
| `sheet list` | 获取全部工作表列表 |
| `sheet info` | 获取指定工作表详情 |
| `sheet new` | 新建工作表 |
| `sheet update` | 更新工作表属性（标题/位置/隐藏/冻结） |
| `sheet copy` | 复制工作表 |
| `sheet range read` | 读取工作表数据（别名: range get） |
| `sheet range update` | 更新指定区域内容（值/公式/超链接） |
| `sheet range set-style` | 设置单元格样式 |
| `sheet range batch-set-style` | 按配置文件批量设置样式 |
| `sheet find` | 搜索单元格内容 |
| `sheet append` | 在末尾追加数据行 |
| `sheet csv-put` | 将 CSV 数据写入指定位置（纯值，自动扩容） |
| `sheet replace` | 全局查找替换文本 |
| `sheet merge-cells` | 合并单元格 |
| `sheet unmerge-cells` | 取消合并单元格 |
| `sheet set-dropdown` | 设置下拉列表 |
| `sheet get-dropdown` | 获取下拉列表配置 |
| `sheet delete-dropdown` | 删除下拉列表 |
| `sheet insert-dimension` | 在指定位置插入行或列 |
| `sheet delete-dimension` | 删除指定位置的行或列 |
| `sheet update-dimension` | 更新行/列属性（显隐/行高/列宽） |
| `sheet move-dimension` | 移动行或列到指定位置 |
| `sheet add-dimension` | 在末尾追加空行或空列 |
| `sheet media-upload` | 上传附件到表格 |
| `sheet write-image` | 上传图片并写入单元格 |
| `sheet create-float-image` | 创建浮动图片 |
| `sheet get-float-image` | 获取浮动图片详情 |
| `sheet list-float-images` | 列出工作表所有浮动图片 |
| `sheet update-float-image` | 更新浮动图片属性 |
| `sheet delete-float-image` | 删除浮动图片 |
| `sheet export` | 导出表格为 xlsx |
| `sheet filter get` | 获取全局筛选信息 |
| `sheet filter create` | 创建全局筛选 |
| `sheet filter delete` | 删除全局筛选 |
| `sheet filter update` | 批量更新筛选条件 |
| `sheet filter clear-criteria` | 清除单列筛选条件 |
| `sheet filter sort` | 筛选排序 |
| `sheet filter-view list` | 获取所有筛选视图 |
| `sheet filter-view create` | 创建筛选视图 |
| `sheet filter-view update` | 更新筛选视图属性 |
| `sheet filter-view delete` | 删除筛选视图 |
| `sheet filter-view update-criteria` | 更新筛选视图列条件 |
| `sheet filter-view delete-criteria` | 删除筛选视图列条件 |
| `sheet filter-view info` | 获取单个筛选视图详情 |
| `sheet filter-view list-criteria` | 列出筛选视图所有列条件 |
| `sheet filter-view get-criteria` | 获取单列筛选条件详情 |

> 不确定参数？对任意命令执行 `dws sheet <命令> --help` 查看完整用法。

## 意图判断

### 表格与工作表管理

用户说"创建表格/新建电子表格":
- 创建表格文档 → `create`

用户说"看工作表/有哪些工作表/表格结构":
- 列出工作表 → `list`
- 工作表详情 → `info`

用户说"加工作表/新增Sheet":
- 新建工作表 → `new`

用户说"修改工作表名称/重命名工作表/移动工作表位置/隐藏工作表/显示工作表/冻结行/冻结列/取消冻结/更新工作表属性":
- 更新工作表属性 → `update`
- 重命名工作表 → `update --title "新名称"`
- 移动工作表位置 → `update --index N`
- 隐藏工作表 → `update --hidden`
- 显示工作表 → `update --hidden=false`
- 冻结行列 → `update --frozen-row-count N --frozen-column-count M`
- 取消冻结 → `update --frozen-row-count 0 --frozen-column-count 0`

用户说"复制工作表/拷贝工作表/克隆工作表/工作表副本":
- 复制工作表 → `copy`
- 复制并指定名称 → `copy --title "副本名称"`
- 复制并指定位置 → `copy --index N`

### 数据读写

用户说"读数据/看表格内容":
- 读取数据 → `range read`

用户说"写数据/填表/更新单元格/写入公式":
- 更新数据 → `range update`
- 【强制】`--sheet-id` 必填：即使是单工作表也不能省略，不要参照 `range read` 的默认行为；未知时先执行 `dws sheet list --node <NODE_ID> --format json` 获取 `sheetId`，禁止凭空臆测为 `Sheet1`、`sheet1`、`0`、`default` 等
- 注意：如果用户的目的是替换文本、移动行列或追加空行空列，请勿使用 `range update`，必须使用对应的专用命令（`replace`/`move-dimension`/`add-dimension`）
- **批量纯值写入优先用 `csv-put`**：当写入场景同时满足以下条件时，必须优先使用 `csv-put` 而非 `range update`：(1) 写入的是纯值（不含公式、超链接）；(2) 数据量较大（超过 5 行或超过 20 个单元格）；(3) 数据来源为表格/CSV 文本/结构化文本。`csv-put` 无需手动构造二维 JSON 数组，直接传 CSV 文本即可，更简洁高效且支持自动扩容

用户说"追加数据/添加行/在末尾加数据/新增记录":
- 追加数据 → `append`

用户说"批量写入CSV/导入CSV/CSV写入表格/把CSV贴到表格里":
- 写入 CSV → `csv-put`
- 与 `range update` 的区别：`csv-put` 接受 CSV 文本直接写入，无需手动构造二维 JSON 数组；适合大批量纯值写入
- 与 `append` 的区别：`csv-put` 写入指定位置（--start-cell），`append` 在末尾追加

用户说"搜索/查找/找单元格/搜内容/精确搜索/精确匹配/完全匹配/全字匹配":
- 搜索单元格 → `find`
- 精确匹配（只匹配完全等于的，不匹配包含的） → `find --match-entire-cell`
- 正则搜索 → `find --use-regexp`
- 搜索公式 → `find --match-formula`
- 不要用 `range read` 读取全量数据后在客户端过滤来替代 `find`，必须使用 `find` 命令的服务端搜索能力

用户说"替换/查找替换/全局替换/批量替换/把A替换成B/把所有的X改成Y":
- 查找替换 → `replace`
- 精确匹配后替换（只替换内容完全等于的单元格） → `replace --match-entire-cell`
- 正则替换 → `replace --use-regexp`
- 删除匹配内容 → `replace --replacement ""`
- 请勿用 `find` + `range update`、`range read` + `range update` 等组合来模拟替换，`replace` 是服务端原子操作，效率更高且返回替换计数

### 行列操作

用户说"插入行/插入列/在某行前插入/在某列前插入":
- 插入行或列 → `insert-dimension`
- 在末尾追加 → `append`（insert-dimension 不支持末尾追加）

用户说"删除行/删除列/删掉第几行/删掉某列/移除行/移除列":
- 删除行或列 → `delete-dimension`
- 仅清空内容但保留行/列 → `range update --values` 写入空字符串 `""`

用户说"隐藏行/隐藏列/显示行/显示列/设置行高/设置列宽/调整行高/调整列宽/行列属性":
- 隐藏/显示行或列 → `update-dimension --hidden` / `--hidden=false`
- 设置行高/列宽 → `update-dimension --pixel-size`
- 同时修改尺寸与显隐 → `update-dimension --pixel-size --hidden`

用户说"移动行/移动列/调整行顺序/调整列顺序/行列拖拽/把第N行移到第M行":
- 移动行或列 → `move-dimension`
- 请勿用 `range read` + `range update` 读取再重写来模拟移动，`move-dimension` 是原子操作，能保留格式和合并状态

用户说"追加空行/追加空列/增加行数/增加列数/扩展表格/在末尾加空行":
- 追加空行/空列 → `add-dimension`
- 注意与 `append`（追加数据行）区分：`add-dimension` 追加的是空行/空列，`append` 追加的是带数据的行
- 请勿用 `range update` 写空数据来模拟追加，`add-dimension` 直接扩展表格维度

### 单元格格式

用户说"设置样式/改颜色/设背景色/加粗/居中/换行/字体颜色/字号":
- 设置单元格样式 → `range set-style`
- 批量设置不同 range 的样式 → `range batch-set-style --batch ./styles.json`（内部顺序循环调 `update_range`）
- 请勿用 `range update --values` 写空/重写来模拟样式变更；也请勿把样式变更混在 `range update` 里、再故意清空 `--values`

用户说"设置数字格式/改成百分比/用人民币显示/按日期显示/文本格式/保留几位小数":
- 设置数字格式 → `range set-style --number-format <格式代码>`（如 `0%` / `¥#,##0.00` / `yyyy/m/d` / `@`）
- 请勿用 `range update` 传递数字格式，`range update` 仅负责写入值与超链接；数字格式属于单元格样式，统一走 `range set-style`

用户说"合并单元格/合并/合并区域/按行合并/按列合并":
- 合并所有单元格 → `merge-cells`（默认 mergeAll）
- 按行合并 → `merge-cells --merge-type mergeRows`
- 按列合并 → `merge-cells --merge-type mergeColumns`

用户说"取消合并/拆分单元格/还原合并":
- 取消合并单元格 → `unmerge-cells`

### 下拉列表

用户说"设置下拉列表/下拉选项/下拉菜单/添加下拉/配置下拉":
- 设置下拉列表 → `set-dropdown`
- 设置多选下拉 → `set-dropdown --multi-select`

用户说"查看下拉列表/获取下拉配置/下拉列表有哪些选项":
- 获取下拉列表配置 → `get-dropdown`

用户说"删除下拉列表/移除下拉/取消下拉/清除下拉":
- 删除下拉列表 → `delete-dropdown`

### 媒体上传

用户说"上传附件/传文件到表格/上传文件到表格/上传到表格":
- 上传附件 → `media-upload`（需表格 ID 或 URL + 本地文件路径）
- 用户指定了上传后的名称 → `media-upload --name "自定义名称"`
- `media-upload` 的 `--name` 参数用于指定附件在表格中显示的名称（不改变本地文件名）；不传时默认使用本地文件名

用户说"写入图片/插入图片/加图片/放图片到单元格/嵌入图片到表格":
- 写入图片 → `write-image`（需表格 ID + 工作表 ID + 单元格范围 + 本地图片路径）
- 禁止使用 `range update` 写入图片，因为 `update_range` 的 MCP 工具不支持图片类型参数，调用必定失败。必须使用 `write-image` 命令
- 用户指定了图片尺寸 → `write-image --width N --height M`

### 浮动图片

用户说"浮动图片/悬浮图片/在表格上放一张图/加个浮动的图":
- 创建浮动图片 → 先 `media-upload` 上传图片获取 `resourceUrl`，再 `create-float-image`
- 浮动图片悬浮于单元格之上，不占用单元格内容，与 `write-image`（写入单元格内部的图片）不同

用户说"查看浮动图片/有哪些浮动图片/浮动图片列表":
- 列出所有浮动图片 → `list-float-images`
- 查看某个浮动图片详情 → `get-float-image`

用户说"移动浮动图片/调整浮动图片大小/修改浮动图片/更新浮动图片":
- 更新浮动图片属性 → `update-float-image`（可更新锚点位置、尺寸、偏移量、图片资源路径）

用户说"删除浮动图片/移除浮动图片":
- 删除浮动图片 → `delete-float-image`

关键区分：`write-image`（单元格内嵌图片，占据单元格内容）vs `create-float-image`（浮动图片，悬浮于单元格之上，不占内容）

### 筛选视图

用户说"筛选/过滤/只看某些值/只显示满足条件的行/筛选数据/创建筛选/删除筛选/设置筛选条件/清除筛选/排序":
- 查看当前筛选 → `filter get`
- 创建筛选 → `filter create`
- 删除筛选 → `filter delete`
- 批量设置多列条件 → `filter update`
- 清除某一列条件 → `filter clear-criteria`
- 按列排序 → `filter sort`
- **区分全局筛选与筛选视图**：如果用户说"筛选视图"则走 `filter-view` 系列；如果只说"筛选/过滤/只看"则默认走全局 `filter` 系列
- **禁止替代方案**：当用户要求"筛选/只看/仅保留某些行"时，必须通过 `filter create` / `filter update` 创建真实的筛选器。禁止用"删除不符合条件的行"或"新建工作表只放符合条件的行"来代替——这些做法会让原数据丢失或不可恢复

用户说"筛选视图/查看筛选视图/有哪些筛选视图/筛选视图列表":
- 获取所有筛选视图 → `filter-view list`

用户说"筛选视图详情/查看某个筛选视图/筛选视图信息/筛选视图配置":
- 获取单个筛选视图详情 → `filter-view info`

用户说"创建筛选视图/新建筛选视图/添加筛选视图":
- 创建筛选视图 → `filter-view create`

用户说"更新筛选视图/修改筛选视图/改筛选视图名称/改筛选视图范围":
- 更新筛选视图属性 → `filter-view update`

用户说"删除筛选视图/移除筛选视图":
- 删除筛选视图 → `filter-view delete`

用户说"设置筛选条件/添加筛选条件/配置筛选视图条件/按值筛选/按条件筛选/按颜色筛选":
- 设置筛选视图列条件 → `filter-view update-criteria`

用户说"查看筛选条件/有哪些筛选条件/筛选视图设了什么条件/列出筛选条件":
- 列出所有列条件 → `filter-view list-criteria`
- 查看某一列的条件 → `filter-view get-criteria --column N`

用户说"清除筛选条件/移除筛选条件/取消筛选条件":
- 清除筛选视图列条件 → `filter-view delete-criteria`
- 注意与 `filter-view delete`（删除整个筛选视图）区分：`delete-criteria` 仅清除指定列的条件，不删除筛选视图本身

### 导出

用户说"导出/下载xlsx/存为Excel/存成表格文件/把表格变成xlsx/导出表格/下载表格/导出为 excel":
- 导出表格 → `export`（单命令一站式，内部自动完成提交、轮询、可选下载）
- 仅需传 `--node`，可选 `--output` 指定本地文件/目录（不传则返回 downloadUrl）
- 需要落盘到本地 → `dws sheet export --node <NODE_ID> --output <path>`，命令自动下载 xlsx
- 禁止用 `range read` 全量读取后自行拼接 xlsx 来模拟导出，必须使用 `export` 命令（服务端原子导出，保留格式/合并/公式等属性）
- 禁止在 AI Agent 侧实现轮询或重试，CLI 内部已按渐进式退避策略完成（最多 30 次约 5 分钟）

### URL 粘贴场景

用户直接粘贴表格 URL（无其他指令）:
- 先 probe：`dws doc info --node <URL> --format json` 校验 `contentType` 和 `extension`
- `extension=axls` → `list`（列出工作表）+ `range read`（读取第一个工作表数据）
- `extension=xlsx` / `xls` / `xlsm` / `csv` → 转 `dws doc download --node <URL> --output ./`，告知用户"这是本地表格文件，已为你下载到本地"，然后基于本地文件继续后续处理

用户粘贴 URL + 附加指令:
- 已 probe 为 `axls` 时：
  - "帮我看看这个表格有什么数据" → `range read`
  - "这个表格有哪些工作表" → `list`
  - "往这个表格写入数据" → `range update`
  - "帮我找一下表格里的XXX" → `find`
- probe 为 xlsx/xls/xlsm/csv 时：无论用户说"读数据/查看/分析"，先走 `dws doc download` 下载到本地，由用户或后续步骤对本地 xlsx 进行解析，严禁调用 `sheet list` / `range read` 等命令

关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录) vs doc(文档编辑/阅读)

## 关键注意事项

以下是最容易出错的规则，**必须严格遵守**：

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ **`range update` 维度校验（强制）**：`--values` / `--hyperlinks` 的行列数必须与 `--range` 完全一致。例如 `--range "A1:C3"` → `--values` 必须是 3×3 数组
- ★ **`range update` 清空规范（强制）**：清空单元格用空字符串 `""`，禁止用 `null`（全 null 会被跳过无效果）
- ★ **单次调用上限（强制）**：`range update` / `set-style` 行数 ≤ 1000，单元格总数建议 ≤ 5000（硬限 30000）
- ★ **大批量纯值写入用 `csv-put` 不用 `range update`**：当写入纯值（无公式/超链接）且数据量较大时（>5 行或 >20 单元格），必须使用 `csv-put`。`csv-put` 接受 CSV 文本直接写入，无需构造二维 JSON 数组，支持自动扩容，更简洁高效。仅在需要写入公式、超链接、或仅更新少量单元格时才使用 `range update`
- ★ **搜索用 `find` 不用 `range read`**：`find` 是服务端搜索，禁止用 `range read` 全量读取后客户端过滤
- ★ **替换用 `replace` 不用 `range update`**：`replace` 是服务端原子操作，返回替换计数
- ★ **移动用 `move-dimension` 不用 `range update`**：原子操作，保留格式和合并状态
- ★ **单元格图片用 `write-image` 不用 `range update`**：`update_range` MCP 不支持图片参数，调用必失败
- ★ **浮动图片用 `create-float-image` 不用 `write-image`**：两者用途不同——`write-image` 写入单元格内部，`create-float-image` 创建悬浮于单元格之上的浮动图片；`--src` 必须来自 `media-upload` 的 `resourceUrl`
- ★ **`export` 禁止自行轮询/重试**：CLI 内部已完成渐进式退避轮询（最多 30 次约 5 分钟），失败时直接告知用户稍后再试
- ★ **关键区分**：sheet(在线电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)

> 完整注意事项请参见本文档末尾「注意事项（完整版）」章节。

## 命令详细参考

> 以下为各命令的完整 Usage、Flags 和示例。参数不确定时也可直接执行 `dws sheet <命令> --help` 在线查看。

### 创建钉钉表格文档
```
Usage:
  dws sheet create [flags]
Example:
  dws sheet create --name "销售数据"
  dws sheet create --name "Q1 数据" --folder <FOLDER_ID>
  dws sheet create --name "知识库表格" --workspace <WS_ID>
Flags:
      --name string        表格名称 (必填)
      --folder string      目标文件夹 ID (dentryUuid 格式) 或 URL；禁止传入纯数字 dentryId
      --workspace string   目标知识库 ID
```

> **ID 格式约束**：`--folder` 只接受 UUID 格式的 `fileId`（如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）或 alidocs 文件夹 URL。`drive list` 返回中有 `dentryId`（纯数字，如 `218595998810`）和 `fileId`（UUID 格式）两个字段，**必须使用 `fileId`，禁止使用 `dentryId`**，传入纯数字会导致命令失败。

### 获取全部工作表列表
```
Usage:
  dws sheet list [flags]
Example:
  dws sheet list --node <NODE_ID>
  dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   表格文档 ID 或 URL (必填)
```

### 获取指定工作表详情
```
Usage:
  dws sheet info [flags]
Example:
  dws sheet info --node <NODE_ID>
  dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet info --node <NODE_ID> --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则返回第一个工作表)
```

### 新建工作表
```
Usage:
  dws sheet new [flags]
Example:
  dws sheet new --node <NODE_ID> --name "Sheet2"
  dws sheet new --node <NODE_ID> --name "数据汇总"
Flags:
      --node string   表格文档 ID (必填)
      --name string   工作表名称 (必填)
```

### 更新工作表属性
```
Usage:
  dws sheet update [flags]
Example:
  # 改名 + 调整冻结
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --title "汇总表" --frozen-row-count 2 --frozen-column-count 1

  # 隐藏工作表
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --hidden=true

  # 显示工作表
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --hidden=false

  # 移动工作表到第一个位置
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --index 0

  # 取消冻结
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --frozen-row-count 0 --frozen-column-count 0
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --title string             新标题，最长 100 字符，不能包含 / \ ? * [ ] :
      --index int                新位置（从 0 开始）
      --hidden                   --hidden=true 隐藏，--hidden=false 取消隐藏
      --frozen-row-count int     冻结行数，0 表示取消冻结
      --frozen-column-count int  冻结列数，0 表示取消冻结
```

更新工作表标题、位置、隐藏状态、冻结行列。
`--title` / `--index` / `--hidden` / `--frozen-row-count` / `--frozen-column-count` 至少提供一个；多个属性可同时传入，将在同一次请求中更新。

注意：
- 至少需要保留一个可见的工作表，不能将所有工作表都隐藏
- 冻结行数/列数不能超过工作表的总行数/列数

### 复制工作表
```
Usage:
  dws sheet copy [flags]
Example:
  # 按默认位置复制
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID>

  # 指定副本名称和位置
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID> --title "销售副本" --index 2

  # 只指定名称
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID> --title "备份"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   源工作表 ID 或名称 (必填)
      --title string      副本名称，最长 100 字符，不能包含 / \ ? * [ ] : (不传则系统自动生成)
      --index int         副本位置（从 0 开始）(不传则放在源工作表之后)
```

复制指定工作表，在同一表格中创建一个副本。
复制操作会将源工作表的所有内容（包括数据、格式、公式等）完整复制到新工作表中。
传 `--index` 时，CLI 会先复制，再追加一次位置更新，把副本移动到目标索引。
名称与已有工作表重复时系统会自动重命名。

### 读取工作表数据
```
Usage:
  dws sheet range read [flags]     # 别名: dws sheet range get
Example:
  dws sheet range read --node <NODE_ID>
  dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet range read --node <NODE_ID> --sheet-id "Sheet1" --range "A1:D10"
  dws sheet range read --node <NODE_ID> --range "Sheet1!A1:D10"

  # 使用 get 别名，与 read 等价
  dws sheet range get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则默认第一个工作表)
      --range string      读取范围，A1 表示法 (如 A1:D10，不传则读取全部数据)
```

**超时处理建议**：读取大范围数据时若出现超时或响应过慢，请主动缩小 `--range` 查询范围，**建议单次读取的单元格数量控制在 5000 个以内**（例如 50 行 × 100 列、100 行 × 50 列）。对于大表可采用分页读取策略：
- 先通过 `info` 获取 `rowCount` / `lastNonEmptyRow` / `columnCount` 确定数据边界
- 按行分批读取，如 `A1:J500`、`A501:J1000`、`A1001:J1500` ……
- 避免不传 `--range` 直接读取整个大工作表

### 更新工作表指定区域内容
```
Usage:
  dws sheet range update [flags]
Example:
  # 写入值
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2" \
    --values '[["姓名","分数"],["张三",90]]'

  # 写入公式
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "C2" \
    --values '[["=A2&B2"]]'

  # 写入超链接
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"钉钉"}]]'

  # 清空区域（使用空字符串 ""）
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" \
    --values '[["",""],["",""],["",""]]'
Flags:
      --node string            表格文档 ID (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --range string           目标单元格区域地址，如 A1:B3 (必填)
      --values string          单元格值，二维 JSON 数组 (与 --hyperlinks 至少传一项)
      --hyperlinks string      超链接，二维 JSON 数组 (与 --values 至少传一项)
```

**单次调用建议**：行数 ≤ 1000，单元格总数（行×列）≤ 5000；超过时请拆分多次调用。

**何时该用 `csv-put` 替代**：如果你准备用 `range update` 写入纯值（不含公式和超链接），且数据量超过 5 行或 20 个单元格，应改用 `csv-put`——它接受 CSV 文本直接写入，无需手动拼装二维 JSON 数组，且支持自动扩容行列。仅在需要写入公式（`=SUM(...)`）、超链接（`--hyperlinks`）、或修改少量单元格时才使用 `range update`。

**范围职责**：`range update` 仅负责写入单元格的值与超链接，不接受任何样式参数。如需设置数字格式（百分比 / 货币 / 日期 / 文本等）请使用 `dws sheet range set-style --number-format <格式代码>`，可与其他样式参数同时传入。

### 设置单元格样式
```
Usage:
  dws sheet range set-style [flags]
Example:
  # 给 A1:B3 打上黄底粗体居中
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" \
    --bg-color "#FFF2CC" --font-weight bold --h-align center

  # 给 C1:C5 逐单元格设置不同背景色
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "C1:C5" \
    --bg-colors-json '[["#FF0000"],["#00FF00"],["#0000FF"],["#FFFF00"],["#FF00FF"]]'

  # 整片 range 启用自动换行
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E10" --word-wrap autoWrap
Flags:
      --node string                 表格文档 ID 或 URL (必填)
      --sheet-id string             工作表 ID 或名称 (必填)
      --range string                目标区域，如 A1:B3 (必填)
      --bg-color string             背景色（#RRGGBB），一键刷整个 range；与 --bg-colors-json 二选一
      --bg-colors-json string       背景色二维 JSON 数组，维度需与 --range 一致
      --font-size int               字号，一键刷整个 range；与 --font-sizes-json 二选一
      --font-sizes-json string      字号二维 JSON 数组
      --h-align string              水平对齐：left/center/right/general
      --h-aligns-json string        水平对齐二维 JSON 数组
      --v-align string              垂直对齐：top/middle/bottom
      --v-aligns-json string        垂直对齐二维 JSON 数组
      --font-color string           字体颜色（#RRGGBB）
      --font-colors-json string     字体颜色二维 JSON 数组
      --font-weight string          字体粗细：bold/normal
      --font-weights-json string    字体粗细二维 JSON 数组
      --word-wrap string            换行方式：overflow/clip/autoWrap（整个 range 共用）
      --number-format string        数字格式，如 General/@/#,##0/0%/yyyy/m/d
```

**特性说明**：
- 每个样式维度提供两种写法，二选一：`--xxx`（单值刷整个 range，CLI 本地展开为二维数组）vs `--xxx-json`（逐单元格指定，维度需与 `--range` 完全一致）
- 至少需传入一个样式参数。单次调用建议：行数 ≤ 1000，单元格总数 ≤ 5000
- 枚举值按驼峰书写：`autoWrap`、`bold`、`normal`、`center` 等

### 批量设置单元格样式
```
Usage:
  dws sheet range batch-set-style [flags]
Example:
  dws sheet range batch-set-style --node <NODE_ID> --batch ./styles.json
  dws sheet range batch-set-style --node <NODE_ID> --batch ./styles.json --continue-on-error
Flags:
      --node string               表格文档 ID 或 URL (必填)
      --batch string              批次配置 JSON 文件路径 (必填)
      --continue-on-error         遇到失败时继续执行后续条目（默认遇错即停）
```

配置文件格式（JSON 数组，每个元素一条批次项）：
```json
[
  {
    "sheetId": "Sheet1",
    "range":   "A1:B3",
    "bgColor":      "#FFF2CC",
    "fontSize":     12,
    "hAlign":       "center",
    "vAlign":       "middle",
    "fontColor":    "#333333",
    "fontWeight":   "bold",
    "wordWrap":     "autoWrap",
    "numberFormat": "General"
  },
  {
    "sheetId": "Sheet1",
    "range":   "C1:C5",
    "bgColorsJson": "[[\"#FF0000\"],[\"#00FF00\"],[\"#0000FF\"],[\"#FFFF00\"],[\"#FF00FF\"]]"
  }
]
```

**特性说明**：
- CLI 侧顺序循环逐条调用 `update_range`（非服务端批量），运行时输出 `[N/M]` 进度
- 每条记录执行与 `set-style` 一致的校验：至少一项样式字段 + rows ≤ 1000 + rows×cols ≤ 30000 + 枚举合法
- 默认遇错即停（返回非 0），`--continue-on-error` 时所有条目跑完再返回首个错误

### 在工作表中搜索单元格内容
```
Usage:
  dws sheet find [flags]
Example:
  # 基本搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "销售额"

  # 在指定范围内搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "合计" --range "A1:D100"

  # 正则表达式搜索（不区分大小写）
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "^total" --use-regexp --match-case=false

  # 精确匹配整个单元格内容
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "完成" --match-entire-cell

  # 搜索公式文本
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "SUM" --match-formula
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --find string         搜索文本 (必填)
      --range string        搜索范围，A1 表示法 (如 A1:D10)
      --match-case          区分大小写 (默认 true)
      --match-entire-cell   精确匹配整个单元格内容
      --use-regexp          启用正则表达式搜索
      --match-formula       搜索公式文本而非显示值
      --include-hidden      包含隐藏单元格
```

### 在工作表末尾追加数据
```
Usage:
  dws sheet append [flags]
Example:
  dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> --values '[["张三","销售部",50000]]'
  dws sheet append --node <NODE_ID> --sheet-id "Sheet1" \
    --values '[["李四","市场部",38000],["王五","销售部",62000]]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --values string     追加数据，二维 JSON 数组 (必填)
```

`--values` 为二维 JSON 数组，外层每个元素代表一行，内层每个元素代表一个单元格值。
追加的数据列数应与工作表已有数据的列数保持一致。

### 将 CSV 数据写入指定位置
```
Usage:
  dws sheet csv-put [flags]
Example:
  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --csv 'name,score\nAlice,95\nBob,87'

  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell B2 \
    --csv @data.csv --allow-overwrite

  cat data.csv | dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> \
    --start-cell A1 --csv -

  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --csv @data.csv --dry-run
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --csv string          CSV 文本、@文件路径 或 - 表示 stdin (必填)
      --start-cell string   起始单元格，A1 表示法 (必填)
      --allow-overwrite     允许覆盖已有数据 (默认 false)
```

将 RFC 4180 格式的 CSV 文本写入指定工作表的指定单元格位置。
- 只写纯值，不支持公式/样式/批注。`=` 开头的内容当文本处理，不会被解析为公式
- 数字/日期/百分数由表格引擎自动识别类型（如 `95` 存为数字，`2025-03-01` 存为日期）
- 自动扩容行列：CSV 数据超出当前工作表维度时自动追加行/列
- 目标区域如含合并单元格，合并将被打散，值正常写入
- `--allow-overwrite` 默认 false，目标区域有数据时需显式传 `--allow-overwrite` 才能覆盖
- `--csv` 支持三种输入：直接传文本、`@filepath` 从本地文件读取、`-` 从 stdin 管道读取
- CSV 文本上限 2M 字符，单元格总数上限 30000
- 特殊字符处理：CLI 会自动过滤 `\r`（Windows 换行符）和 BOM（UTF-8 文件头标记），Excel/Windows 导出的 CSV 可直接使用；如 CSV 数据中含零宽字符（U+200B 等）或 Bidi 控制符，CLI 会拒绝并报错

### 在指定位置插入行或列
```
Usage:
  dws sheet insert-dimension [flags]
Example:
  # 在第 3 行之前插入 2 行
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "3" --length 2

  # 在 A 列之前插入 1 列
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "A" --length 1

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "Sheet1!3" --length 5

  # 在 AB 列之前插入 3 列
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "AB" --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   插入维度: ROWS 或 COLUMNS (必填)
      --position string    插入位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string      插入数量，正整数 (必填)，最大 5000
```

在钉钉表格指定工作表的指定位置之前插入若干空行或空列。
`--dimension ROWS` 时，`--position` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--position` 为列字母。
支持在 `--position` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
若需要在末尾追加行/列，请使用 `append` 命令。

### 删除指定位置的行或列

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet delete-dimension [flags]
Example:
  # 从第 3 行开始删除 2 行
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "3" --length 2

  # 从 A 列开始删除 1 列
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "A" --length 1

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "Sheet1!3" --length 5

  # 从 AB 列开始删除 3 列
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "AB" --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   删除维度: ROWS 或 COLUMNS (必填)
      --position string    删除起始位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string      删除数量，正整数 (必填)，最大 5000
```

在钉钉表格指定工作表中，从指定位置起删除若干连续的行或列。
`--dimension ROWS` 时，`--position` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--position` 为列字母。
支持在 `--position` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
删除后后续的行/列会向前移动填补空位；若需要仅清空内容但保留行/列占位，请使用 `range update` 将目标区域写入空字符串 `""`。

### 更新指定范围行/列属性
```
Usage:
  dws sheet update-dimension [flags]
Example:
  # 隐藏第 3~4 行
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "3" --length 2 --hidden

  # 显示 A~B 列
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --start-index "A" --length 2 --hidden=false

  # 设置第 1~5 行行高为 40px
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "1" --length 5 --pixel-size 40

  # 设置 C 列列宽为 200px 并隐藏
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --start-index "C" --length 1 --pixel-size 200 --hidden

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "Sheet1!3" --length 2 --hidden
Flags:
      --node string          表格文档 ID 或 URL (必填)
      --sheet-id string      工作表 ID 或名称 (必填)
      --dimension string     更新维度: ROWS 或 COLUMNS (必填)
      --start-index string   起始位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string        更新数量，正整数 (必填)，最大 5000
      --hidden               是否隐藏 (true=隐藏, false=显示)，与 --pixel-size 至少填其一
      --pixel-size int       行高或列宽（像素），ROWS 时为行高，COLUMNS 时为列宽，与 --hidden 至少填其一
```

批量更新钉钉表格指定工作表中连续多行/多列的属性，支持设置显隐状态（hidden）与行高/列宽（pixelSize）。
`--dimension ROWS` 时，`--start-index` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--start-index` 为列字母。
支持在 `--start-index` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
`--hidden` 与 `--pixel-size` 至少必须提供一个。当同时提供时，将先应用尺寸再应用显隐，任一失败整体失败。
`--pixel-size` 单位为像素，`dimension=ROWS` 时表示行高、`dimension=COLUMNS` 时表示列宽。

### 合并单元格
```
Usage:
  dws sheet merge-cells [flags]
Example:
  # 合并所有单元格（默认）
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3"

  # 按行合并
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeRows

  # 按列合并
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeColumns

  # 使用带工作表前缀的范围（忽略 --sheet-id）
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "Sheet1!A1:B3"
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --range string        目标单元格区域地址，如 A1:B3 (必填)
      --merge-type string   合并方式: mergeAll(默认)/mergeRows/mergeColumns
```

支持三种合并方式：
- `mergeAll`（默认）：合并所有单元格，将选定区域内的所有单元格合并成一个
- `mergeRows`：按行合并，在选定区域内将同一行相邻的单元格合并
- `mergeColumns`：按列合并，在选定区域内将同一列相邻的单元格合并

注意：合并时只保留左上角单元格的值，其他单元格的值会被丢弃。
`--range` 支持带工作表前缀的写法（如 `Sheet1!A1:B3`），此时将优先使用前缀解析出的工作表，忽略 `--sheet-id`。

### 上传附件到表格
```
Usage:
  dws sheet media-upload [flags]
Example:
  dws sheet media-upload --node <NODE_ID> --file ./report.pdf
  dws sheet media-upload --node <NODE_ID> --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream
Flags:
      --node string        目标表格文档的标识，支持传入 URL 或 ID (必填)
      --file string        本地文件路径 (必填)
      --name string        附件显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
```

### 上传图片并写入表格单元格
```
Usage:
  dws sheet write-image [flags]
Example:
  dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range A1:A1 --file ./chart.png
  dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range B2:B2 --file ./logo.png --width 200 --height 100
Flags:
      --node string        目标表格文档的标识，支持传入 URL 或 ID (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --range string       目标单元格区域地址，如 A1:A1 (必填)
      --file string        本地图片文件路径 (必填)
      --name string        图片显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
      --width int          图片显示宽度 (可选)
      --height int         图片显示高度 (可选)
```

### 创建浮动图片
```
Usage:
  dws sheet create-float-image [flags]
Example:
  # 先上传图片获取 resourceUrl
  dws sheet media-upload --node <NODE_ID> --file ./chart.png
  # 输出: resourceUrl: /core/api/resources/img/xxxx...

  # 再创建浮动图片
  dws sheet create-float-image --node <NODE_ID> --sheet-id <SHEET_ID> \
    --src "/core/api/resources/img/xxxx..." --range A1 --width 400 --height 300

  # 带偏移量
  dws sheet create-float-image --node <NODE_ID> --sheet-id <SHEET_ID> \
    --src "/core/api/resources/img/xxxx..." --range B2 --width 200 --height 150 --offset-x 10 --offset-y 20
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --src string        图片资源路径，通过 media-upload 获取的 resourceUrl (必填)
      --range string      锚点单元格，A1 表示法，如 A1、B3 (必填)
      --width int         图片宽度，像素，正整数 (必填)
      --height int        图片高度，像素，正整数 (必填)
      --offset-x int      水平偏移量，像素 (默认 0)
      --offset-y int      垂直偏移量，像素 (默认 0)
```

浮动图片悬浮于单元格之上，不占用单元格内容，可自由定位和调整大小。
- `--src` 必须是 `media-upload` 返回的 `resourceUrl`（格式为 `/core/api/resources/img/...`），不能直接传外部 URL
- `--range` 使用 A1 表示法指定锚点单元格（如 `A1`、`B3`），支持带工作表前缀（如 `Sheet1!A1`）
- `--width` / `--height` 为必填，单位像素，必须为正整数
- `--offset-x` / `--offset-y` 表示相对锚点单元格左上角的偏移量（像素），默认 0，不能为负数

### 获取浮动图片详情
```
Usage:
  dws sheet get-float-image [flags]
Example:
  dws sheet get-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --float-image-id string   浮动图片 ID (必填)
```

获取单个浮动图片的详细信息，包括 ID、图片资源路径、锚点位置、尺寸和偏移量。
`--float-image-id` 可通过 `list-float-images` 获取。

### 列出工作表所有浮动图片
```
Usage:
  dws sheet list-float-images [flags]
Example:
  dws sheet list-float-images --node <NODE_ID> --sheet-id <SHEET_ID>
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

列出指定工作表中所有浮动图片，返回 `floatImages` 数组和 `totalCount`。

### 更新浮动图片属性
```
Usage:
  dws sheet update-float-image [flags]
Example:
  # 移动浮动图片到新位置
  dws sheet update-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID> --range C5

  # 调整尺寸
  dws sheet update-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID> --width 600 --height 400

  # 替换图片（需先 media-upload 新图片获取 resourceUrl）
  dws sheet update-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID> \
    --src "/core/api/resources/img/xxxx..."
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --float-image-id string   浮动图片 ID (必填)
      --src string              新的图片资源路径，通过 media-upload 获取的 resourceUrl
      --range string            新的锚点单元格，A1 表示法
      --width int               新的图片宽度，像素
      --height int              新的图片高度，像素
      --offset-x int            新的水平偏移量，像素
      --offset-y int            新的垂直偏移量，像素
```

更新浮动图片的属性，`--src` / `--range` / `--width` / `--height` / `--offset-x` / `--offset-y` 至少传入一个。
`--float-image-id` 可通过 `list-float-images` 获取。

### 删除浮动图片
```
Usage:
  dws sheet delete-float-image [flags]
Example:
  dws sheet delete-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --float-image-id string   浮动图片 ID (必填)
```

删除指定的浮动图片，操作不可恢复。`--float-image-id` 可通过 `list-float-images` 获取。

### 全局查找替换
```
Usage:
  dws sheet replace [flags]
Example:
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧文本" --replacement "新文本"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "待处理" --replacement "已完成" --match-entire-cell
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "\\d{4}" --replacement "****" --use-regexp
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧" --replacement "新" --range "A1:D100"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "临时" --replacement ""
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --find string            查找文本 (必填)
      --replacement string     替换文本 (必填，可为空字符串表示删除)
      --range string           替换范围，A1 表示法 (如 A1:D100)
      --match-case             区分大小写 (默认 false)
      --match-entire-cell      完整单元格匹配
      --use-regexp             启用正则表达式匹配
      --include-hidden         包含隐藏行/列
```

返回被替换的单元格数量。`--replacement` 可以为空字符串，表示删除匹配内容。

### 移动行或列
```
Usage:
  dws sheet move-dimension [flags]
Example:
  # 将第 2 行移动到第 5 行的位置（索引从 0 开始）
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index 1 --end-index 1 --destination-index 4

  # 将第 2~4 行移动到第 1 行之前
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index 1 --end-index 3 --destination-index 0

  # 将 B~C 列移动到 E 列的位置
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension COLUMNS --start-index 1 --end-index 2 --destination-index 4
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --dimension string         维度类型: ROWS 或 COLUMNS (必填)
      --start-index int          源起始索引，0-based (必填)
      --end-index int            源结束索引，0-based，包含 (必填)
      --destination-index int    目标位置索引，0-based (必填)
```

索引均为 0-based（第 1 行/列的索引为 0）。destination-index 不能在 [start-index, end-index] 范围内。

**destination-index 计算规则：**
destination-index 是目标位置的 0-based 索引，即移动到第 n 行/列则传 n-1：
- 通用公式：`destination-index = 目标行号(1-based) - 1`
- 例如：将第 2 行移到第 5 行位置 → `destination-index = 5 - 1 = 4`，即 `start-index=1, end-index=1, destination-index=4`
- 例如：将第 4 行移到第 1 行（最前面）→ `destination-index = 1 - 1 = 0`，即 `start-index=3, end-index=3, destination-index=0`

### 追加空行或空列
```
Usage:
  dws sheet add-dimension [flags]
Example:
  dws sheet add-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --length 5
  dws sheet add-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   维度类型: ROWS 或 COLUMNS (必填)
      --length int         追加数量，正整数，最多 5000 (必填)
```

在工作表末尾追加指定数量的空行或空列。

### 取消合并单元格
```
Usage:
  dws sheet unmerge-cells [flags]
Example:
  dws sheet unmerge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D5"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      取消合并的范围，A1 表示法 (必填)
```

取消指定范围内所有合并的单元格，恢复为独立单元格。

### 设置下拉列表
```
Usage:
  dws sheet set-dropdown [flags]
Example:
  # 设置单选下拉列表
  dws sheet set-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100" \
    --options '[{"value":"选项1"},{"value":"选项2"},{"value":"选项3"}]'

  # 设置带颜色的多选下拉列表
  dws sheet set-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B50" \
    --options '[{"value":"高","color":"#ff0000"},{"value":"中","color":"#ffaa00"},{"value":"低","color":"#00ff00"}]' \
    --multi-select
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --range string        目标单元格范围，A1 表示法，如 A2:A100 (必填)
      --options string      下拉选项 JSON 数组 (必填)，如 '[{"value":"选项1","color":"#ff0000"}]'
      --multi-select        是否允许多选（默认单选）
```

在指定单元格范围内设置下拉列表。设置后用户可从预定义选项中选择值。
- **用途**：为单元格配置下拉列表，支持自定义选项颜色和多选。
- **场景**：规范数据输入，如状态选择（完成/进行中/待处理）、优先级（高/中/低）等。
- **注意**：选项值不能包含英文逗号；如果目标范围已存在下拉列表，会被新配置覆盖。

### 获取下拉列表配置
```
Usage:
  dws sheet get-dropdown [flags]
Example:
  dws sheet get-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100"
  dws sheet get-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      查询范围，A1 表示法，如 A1:A100 (必填)
```

查询指定范围内的下拉列表配置信息，包括选项值、颜色和是否多选。
- **用途**：查看单元格已设置的下拉列表选项和配置。
- **场景**：在修改下拉列表前先查询现有配置；确认下拉列表是否设置成功。
- **返回**：`dataValidations` 数组，相同选项的单元格聚合为一组，每组包含 `conditionValues`（选项值）、`ranges`（覆盖范围）、`options`（含 `enableMultiSelect` 和 `colorValueMap`）。范围内无下拉列表时 `hasDropdown` 为 false。

### 删除下拉列表
```
Usage:
  dws sheet delete-dropdown [flags]
Example:
  dws sheet delete-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100"
  dws sheet delete-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "B1:D10"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      要删除下拉列表的范围，A1 表示法 (必填)
```

删除指定范围内的下拉列表配置，单元格恢复为普通文本格式。
- **用途**：移除不再需要的下拉列表约束。
- **注意**：已填写的单元格值不会被清除；目标范围不存在下拉列表时操作仍返回成功。

### 获取筛选信息
```
Usage:
  dws sheet filter get [flags]
Example:
  dws sheet filter get --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet filter get --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

获取指定工作表的全局筛选信息，返回筛选范围和各列的筛选条件详情。
- **用途**：查看当前工作表上是否存在全局筛选及其配置。
- **场景**：在修改或删除筛选前，先读取当前筛选配置；创建筛选前先确认是否已存在（每个工作表只能有一个筛选）。
- **区分**：全局筛选（filter）影响所有协作者看到的数据展示；筛选视图（filter-view）是个人化的。
- **返回**：`range`（筛选范围，A1 表示法）和 `columnFilterCriteria`（各列条件，key 为列偏移量）。如果未设置筛选，返回筛选信息为空。

### 创建筛选
```
Usage:
  dws sheet filter create [flags]
Example:
  # 创建筛选框架（不设条件）
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100"

  # 创建筛选并同时设置条件（按值筛选）
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100" --criteria '[{"column":1,"filterType":"values","visibleValues":["北京","上海"]}]'

  # 创建筛选并设置条件筛选
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100" --criteria '[{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      筛选范围，A1 表示法，须包含表头行 (必填)
      --criteria string   筛选条件 JSON 数组 (可选)
```

在工作表中创建全局筛选。
- **用途**：为工作表建立筛选器，使数据可按条件过滤展示。
- **约束**：每个工作表只能有一个全局筛选，已存在时会报错。应先 `filter get` 确认不存在后再创建。
- **range 规范**：必须包含表头行（如 `A1:E100`），不能只包含数据行。
- **criteria 格式**：JSON 数组，每个元素含 `column`（列偏移量，从 0 开始）和筛选条件字段。不传则仅创建空筛选框架，后续可通过 `filter update` 设置条件。

### 删除筛选

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet filter delete [flags]
Example:
  dws sheet filter delete --node <NODE_ID> --sheet-id <SHEET_ID> --yes
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

删除工作表的全局筛选。
- **用途**：移除筛选器，所有被隐藏的行将重新显示。
- **不可逆**：删除后所有筛选条件丢失，需重新创建。
- **前置**：工作表没有筛选时调用会报错，应先 `filter get` 确认存在。

### 批量更新筛选条件
```
Usage:
  dws sheet filter update [flags]
Example:
  # 同时设置多列的筛选条件
  dws sheet filter update --node <NODE_ID> --sheet-id <SHEET_ID> --criteria '[{"column":0,"filterType":"values","visibleValues":["已完成","进行中"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50"}]}]'

  # 按颜色筛选
  dws sheet filter update --node <NODE_ID> --sheet-id <SHEET_ID> --criteria '[{"column":1,"filterType":"color","backgroundColor":"#FF0000"}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --criteria string   筛选条件 JSON 数组 (必填)
```

批量更新筛选条件，可同时设置多列的筛选条件。
- **用途**：一次性设置或替换多列的筛选条件。
- **前置**：工作表必须已创建筛选（通过 `filter create`）。
- **覆盖式**：指定列的条件会被替换，未指定的列保持不变。如只想修改某一列，建议先 `filter get` 读取现有配置。
- **criteria 格式**：JSON 数组，支持三种 `filterType`：
  - `values`：按值筛选，指定 `visibleValues` 数组
  - `condition`：按条件筛选，指定 `conditions` 数组（最多 2 个）和可选的 `conditionOperator`（`and`/`or`）
  - `color`：按颜色筛选，指定 `backgroundColor` 或 `fontColor`（二选一）

### 清除单列筛选条件
```
Usage:
  dws sheet filter clear-criteria [flags]
Example:
  # 清除第 2 列（B 列）的筛选条件
  dws sheet filter clear-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --column 1

  # 清除第 1 列（A 列）的筛选条件
  dws sheet filter clear-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --column 0
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --column number     列偏移量，从 0 开始 (必填)
```

清除筛选中某一列的筛选条件。
- **用途**：移除某列的筛选条件，该列不再参与筛选计算。
- **区分**：仅清除指定列的条件，不删除整个筛选。如需删除整个筛选，使用 `filter delete`。
- **幂等**：指定列没有设置筛选条件时调用不会报错。

### 筛选排序
```
Usage:
  dws sheet filter sort [flags]
Example:
  # 按第 1 列（A 列）升序排序
  dws sheet filter sort --node <NODE_ID> --sheet-id <SHEET_ID> --column 0 --ascending

  # 按第 3 列（C 列）降序排序
  dws sheet filter sort --node <NODE_ID> --sheet-id <SHEET_ID> --column 2 --ascending=false
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --column number     排序列偏移量，从 0 开始 (必填)
      --ascending         是否升序，默认 true (可选)
```

对筛选范围内的数据按指定列排序。
- **用途**：对数据行按某一列的值进行升序或降序排列。
- **前置**：工作表必须已创建筛选（通过 `filter create`）。
- **注意**：排序会实际改变工作表中数据行的物理顺序，不可撤销。
- **column**：列偏移量从 0 开始，相对于筛选范围首列。

### 获取所有筛选视图
```
Usage:
  dws sheet filter-view list [flags]
Example:
  dws sheet filter-view list --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet filter-view list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

获取指定工作表的所有筛选视图列表，返回每个筛选视图的 ID、名称和范围信息。
- **用途**：查看当前工作表上已创建的所有筛选视图，获取视图 ID、名称和范围。
- **场景**：在对筛选视图进行 update / delete / update-criteria 等操作前，先用 list 获取可用的 filterViewId。
- **区分**：筛选视图（filter-view）是个人化的数据过滤方式，与全局筛选不同。每个用户可以创建自己的筛选视图，互不影响原始数据。如果没有筛选视图，返回空列表。

### 创建筛选视图
```
Usage:
  dws sheet filter-view create [flags]
Example:
  # 创建不带筛选条件的筛选视图
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "我的视图" --range "A1:E10"

  # 创建带按值筛选条件的筛选视图
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "销售筛选" --range "A1:E10" \
    --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]}]'

  # 创建带按条件筛选的筛选视图（大于等于 200000）
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "高预算" --range "A1:C10" \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --name string       筛选视图名称 (必填)
      --range string      筛选视图范围，A1 表示法，如 A1:E10 (必填)
      --criteria string   筛选条件，JSON 数组 (可选)
```

在指定工作表中创建一个筛选视图。
- **用途**：为指定数据区域创建一个可命名的个人化筛选视图，可选同时设置筛选条件。
- **场景**：用户需要针对某个数据区域建立固定的筛选视角（如"高绩效员工""研发部数据"），方便反复查看。
- **区分**：与全局筛选不同，筛选视图是个人化的，不影响其他用户看到的数据。如果只需创建视图不设条件，后续可通过 `update-criteria` 单独设置；如果要一步到位，可通过 `--criteria` 在创建时直接设置。
`--criteria` 为 JSON 数组，每个元素包含 `column`（列偏移量，从 0 开始）和筛选条件字段。支持三种筛选类型：
- `values`：按值筛选，通过 `visibleValues` 指定允许显示的值列表
- `condition`：按条件筛选，通过 `conditions` 指定条件列表（最多 2 个），每个条件包含 `operator` 和 `value`。支持的操作符（kebab-case）：`equal`、`not-equal`、`contains`、`not-contains`、`starts-with`、`not-starts-with`、`ends-with`、`not-ends-with`、`greater`、`greater-equal`、`less`、`less-equal`。多条件之间通过 `conditionOperator` 指定逻辑关系：`and`（且，默认）或 `or`（或）
- `color`：按颜色筛选，通过 `backgroundColor` 或 `fontColor` 指定颜色值（十六进制，如 `#FF0000`），二选一

### 更新筛选视图属性
```
Usage:
  dws sheet filter-view update [flags]
Example:
  # 更新筛选视图名称
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --name "新名称"

  # 更新筛选视图范围
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --range "A1:F20"

  # 更新筛选条件
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --name string             筛选视图新名称
      --range string            筛选视图新范围，A1 表示法
      --criteria string         筛选条件，JSON 数组
```

更新筛选视图的名称、范围和/或筛选条件，`--name`、`--range`、`--criteria` 至少传入一个。
- **用途**：修改已有筛选视图的名称、数据范围或筛选条件。
- **场景**：数据区域扩展后需要扩大筛选视图范围，或重命名视图，或通过 `--criteria` 一次性批量更新多列筛选条件。
- **区分**：`update` 可同时修改名称、范围和条件，适合批量更新；`update-criteria` 只能设置单列条件，适合精确控制某一列的筛选逻辑。`--criteria` 指定列的条件会被替换，未指定的列保持不变。

`--criteria` 为 JSON 数组，格式与 `filter-view create` 的 `--criteria` 相同，支持的筛选类型和操作符参见「创建筛选视图」说明。

### 删除筛选视图

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet filter-view delete [flags]
Example:
  dws sheet filter-view delete --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

删除指定的筛选视图。
- **用途**：永久删除一个不再需要的筛选视图及其所有筛选条件。
- **场景**：筛选视图已过时或不再需要时，清理无用的视图。
- **区分**：`delete` 删除整个筛选视图（包括所有列的条件），操作不可恢复；`delete-criteria` 只删除某一列的筛选条件，视图本身保留。此操作不影响全局筛选或其他筛选视图，也不影响原始数据。

### 更新筛选视图列条件
```
Usage:
  dws sheet filter-view update-criteria [flags]
Example:
  # 按值筛选：只显示"销售部"和"市场部"
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}'

  # 按条件筛选：大于 100
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}'

  # 按条件筛选：大于等于 200000
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}'

  # 按条件筛选：小于 100
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"less","value":"100"}]}'

  # 多条件筛选：大于等于 60 且 小于等于 90
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 2 --filter-criteria '{"filterType":"condition","conditionOperator":"and","conditions":[{"operator":"greater-equal","value":"60"},{"operator":"less-equal","value":"90"}]}'

  # 按颜色筛选：背景色为红色
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"color","backgroundColor":"#FF0000"}'
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --filter-view-id string    筛选视图 ID (必填)
      --column int               列偏移量，从 0 开始 (必填)
      --filter-criteria string   筛选条件，JSON 对象 (必填)
```

更新筛选视图中某一列的筛选条件。
- **用途**：为筛选视图的指定列创建或更新筛选条件，控制该列哪些数据行可见。
- **场景**：只显示某些特定值的行（如"只看研发部"）→ `filterType: values`；按数值条件筛选（如"绩效 ≥ 85"）→ `filterType: condition` + `operator: greater-equal`；按文本条件筛选（如"名称包含关键字"）→ `filterType: condition` + `operator: contains`。
- **区分**：`update-criteria` 精确控制单列条件，适合逐列设置不同的筛选逻辑；`filter-view update --criteria` 可以批量更新多列条件；`delete-criteria` 是 `update-criteria` 的逆操作，删除指定列的条件。

`--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。
例如筛选视图范围为 `B1:E10`，则 `--column 0` 代表 B 列，`--column 1` 代表 C 列。

`--filter-criteria` 为 JSON 对象，支持三种筛选类型：
- `values`：按值筛选，通过 `visibleValues` 指定允许显示的值列表
- `condition`：按条件筛选，通过 `conditions` 指定条件列表（最多 2 个），每个条件包含 `operator` 和 `value`。支持的操作符：`equal`、`not-equal`、`contains`、`not-contains`、`starts-with`、`not-starts-with`、`ends-with`、`not-ends-with`、`greater`、`greater-equal`、`less`、`less-equal`。多条件之间通过 `conditionOperator` 指定逻辑关系：`and`（且，默认）或 `or`（或）
- `color`：按颜色筛选，通过 `backgroundColor` 或 `fontColor` 指定颜色值（十六进制，如 `#FF0000`），二选一

### 删除筛选视图列条件
```
Usage:
  dws sheet filter-view delete-criteria [flags]
Example:
  # 删除第 1 列（A 列）的筛选条件
  dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 0

  # 删除第 3 列（C 列）的筛选条件
  dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 2
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --column int              列偏移量，从 0 开始 (必填)
```

清除筛选视图中指定列的筛选条件。
- **用途**：移除筛选视图中指定列的筛选条件，使该列不再参与过滤。
- **场景**：之前通过 `update-criteria` 设置了某列的筛选条件，现在需要取消该列的筛选以显示全部数据。
- **区分**：`delete-criteria` 只清除指定列的条件，筛选视图本身和其他列的条件保持不变；`delete` 会删除整个筛选视图。如果指定列没有设置筛选条件，调用此命令不会报错（幂等操作）。

### 获取单个筛选视图详情
```
Usage:
  dws sheet filter-view info [flags]
Example:
  # 查看指定筛选视图的详情
  dws sheet filter-view info --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

获取指定筛选视图的完整信息，包括 ID、名称、范围和筛选条件。
- **用途**：查看某个筛选视图的当前配置，包括已设置的所有筛选条件详情。
- **场景**：在修改或删除筛选视图前，先确认其当前状态；或在 `update-criteria` 后验证条件是否生效。
- **区分**：`info` 返回单个视图的完整信息（含 criteria）；`list` 返回所有视图的列表概要。`info` 需要指定 `--filter-view-id`，ID 可通过 `list` 获取。
- **实现**：内部调用 `get_filter_views` 获取全部列表后按 ID 过滤。

### 列出筛选视图所有列条件
```
Usage:
  dws sheet filter-view list-criteria [flags]
Example:
  # 列出筛选视图的所有条件
  dws sheet filter-view list-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

列出指定筛选视图中已设置的所有列筛选条件。
- **用途**：查看某个筛选视图当前设置了哪些列的筛选条件，包括每列的条件类型和具体规则。
- **场景**：在管理筛选条件（修改/删除特定列条件）前，先了解当前视图有哪些条件；或排查筛选结果不符合预期时检查条件配置。
- **区分**：`list-criteria` 返回所有列的条件（按列偏移量为 key 的对象）；`get-criteria` 只返回指定列的条件。如果没有设置任何条件，返回空对象 `{}`。
- **实现**：内部调用 `get_filter_views` 获取视图详情后提取 `criteria` 字段。

### 获取单列筛选条件
```
Usage:
  dws sheet filter-view get-criteria [flags]
Example:
  # 查看第 1 列（偏移量 0）的筛选条件
  dws sheet filter-view get-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 0

  # 查看第 3 列（偏移量 2）的筛选条件
  dws sheet filter-view get-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 2
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --column int              列偏移量，从 0 开始 (必填)
```

获取指定筛选视图中某一列的筛选条件详情。
- **用途**：查看某个筛选视图中指定列当前设置的筛选条件，包括条件类型、运算符和比较值。
- **场景**：在修改某列条件前，先查看其当前配置；或验证 `update-criteria` 后该列条件是否正确。
- **区分**：`get-criteria` 只返回指定列的条件；`list-criteria` 返回所有列的条件。`--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。
- **实现**：内部调用 `get_filter_views` 获取视图详情后按列偏移量过滤 `criteria` 中的对应条件。

### 导出表格为 xlsx（异步任务一站式）
```
Usage:
  dws sheet export [flags]    # 一站式：提交 → 轮询 → 可选下载
Example:
  # 仅导出，返回 downloadUrl（链接有时效性，请尽快下载）
  dws sheet export --node <NODE_ID>
  dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"

  # 导出并自动下载为本地文件
  dws sheet export --node <NODE_ID> --output ./report.xlsx

  # --output 为目录时，自动按下载链接中的文件名保存
  dws sheet export --node <NODE_ID> --output ./

Flags:
      --node string     表格文档 ID 或 URL (必填)
      --output string   本地保存路径（可选，支持文件路径或目录）
```

将钉钉在线电子表格导出为 Office xlsx 格式。**单命令一站式**：命令内部自动完成「提交任务 → 渐进式退避轮询 → （可选）下载文件」全流程，AI Agent 无需自行拆分步骤或实现轮询。

**内部流程**：
1. 调 `submit_export_job` 获取 `jobId`
2. 按渐进式退避策略轮询 `query_export_job` 直至任务终态或超时
3. 任务成功后取得 `downloadUrl`；若指定了 `--output`，自动 HTTP GET 下载 xlsx 到本地文件

**内置轮询策略（CLI 内实现，无需关心）**：
- 第 1~5 次：每次间隔 2 秒
- 第 6~10 次：每次间隔 5 秒
- 第 11~20 次：每次间隔 10 秒
- 第 21~30 次：每次间隔 15 秒
- **硬上限：最多轮询 30 次（约 5 分钟）**，超时后命令返回错误

**命令返回**：
- `--output` 未指定：进度日志 + 末尾输出 `jobId` 和 `downloadUrl`（链接有时效性，请尽快下载）
- `--output` 指定为文件路径：下载到该路径并输出 `导出完成: <path>`
- `--output` 指定为已存在目录：自动从 `downloadUrl` 推断文件名并保存到该目录下

**失败处理（命令内部已处理，Agent 仅需转述）**：
- MCP 返回 `FAILED`：命令立即返回错误并附带失败原因，**禁止自动重试 `dws sheet export`**，告知用户稍后再试
- 轮询 30 次仍 `PROCESSING`：命令返回超时错误，告知用户稍后再试

**限制**：仅支持钉钉在线电子表格（alxs）→ xlsx。导出钉钉文字文档请使用 `doc` 产品对应的导出工具。

## 核心工作流

```bash
# ── 工作流 1: 创建表格并写入数据 ──

# 1. 创建表格文档 — 提取 nodeId
dws sheet create --name "销售数据" --format json

# 2. 查看工作表列表 — 提取 sheetId
dws sheet list --node <NODE_ID> --format json

# 3. 写入表头和数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C1" \
  --values '[["姓名","部门","销售额"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:C4" \
  --values '[["张三","销售部",50000],["李四","市场部",38000],["王五","销售部",62000]]' --format json

# ── 工作流 2: 读取已有表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（行列数、最后非空位置等）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 读取全部数据
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 4. 读取指定区域
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" --format json

# ── 工作流 3: 多工作表管理 ──

# 1. 新建工作表
dws sheet new --node <NODE_ID> --name "汇总" --format json

# 2. 在新工作表中写入汇总公式
dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A1:B1" \
  --values '[["指标","数值"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A2:B2" \
  --values '[["总销售额","=SUM(Sheet1!C2:C100)"]]' --format json

# ── 工作流 4: 写入数据并设置样式 ──

# 1. 写入数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" \
  --values '[["商品","单价","数量"],["苹果",5.5,100],["香蕉",3.2,200]]' --format json

# 2. 设置数字格式（人民币）——请走 set-style，不要放到 range update
dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B3" \
  --number-format "¥#,##0.00" --format json

# 3. 写入超链接
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D1" \
  --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"详情"}]]' --format json

# ── 工作流 5: 追加数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（确认列结构）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 追加单行数据
dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
  --values '[["张三","销售部",50000]]' --format json

# 4. 追加多行数据
dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
  --values '[["李四","市场部",38000],["王五","销售部",62000]]' --format json
```

```bash
# ── 工作流 6: 插入行或列 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 在第 3 行之前插入 2 行
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "3" --length 2 --format json

# 3. 在 A 列之前插入 1 列
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --position "A" --length 1 --format json

# 4. 使用工作表前缀指定位置
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "Sheet1!5" --length 3 --format json
```

```bash
# ── 工作流 6b: 删除行或列 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 从第 3 行开始删除 2 行
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "3" --length 2 --format json

# 3. 从 A 列开始删除 1 列
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --position "A" --length 1 --format json

# 4. 使用工作表前缀指定位置
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "Sheet1!5" --length 3 --format json
```

```bash
# ── 工作流 6c: 更新行/列属性（显隐、行高/列宽） ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 隐藏第 3~4 行
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --start-index "3" --length 2 --hidden --format json

# 3. 显示 A~B 列
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --start-index "A" --length 2 --hidden=false --format json

# 4. 设置第 1~5 行行高为 40px
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --start-index "1" --length 5 --pixel-size 40 --format json

# 5. 设置 C 列列宽为 200px 并隐藏
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --start-index "C" --length 1 --pixel-size 200 --hidden --format json
```

```bash
# ── 工作流 7: 搜索表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 基本搜索 — 在指定工作表中查找文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "销售额" --format json

# 3. 在指定范围内搜索
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "合计" --range "A1:D100" --format json

# 4. 正则搜索（不区分大小写）
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "^total" --use-regexp --match-case=false --format json

# 5. 精确匹配整个单元格
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "完成" --match-entire-cell --format json

# 6. 搜索公式文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "SUM" --match-formula --format json
```

```bash
# ── 工作流 8: 合并单元格 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 合并所有单元格（默认 mergeAll）
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" --format json

# 3. 按行合并
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeRows --format json

# 4. 按列合并
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeColumns --format json
```

```bash
# ── 工作流 9: 上传附件到表格 ──

# 1. 基本用法: 上传本地文件到表格
dws sheet media-upload --node <NODE_ID> --file ./report.pdf -f json

# 2. 自定义附件显示名称 (--name 指定上传后在表格中显示的名称)
dws sheet media-upload --node <NODE_ID> --file ./data.csv --name "销售数据.csv" -f json

# 3. 指定 MIME 类型 (文件扩展名无法推断时)
dws sheet media-upload --node <NODE_ID> --file ./data.bin --name "导出数据.dat" --mime-type application/octet-stream -f json

# 4. 完整流程: 创建表格 → 上传附件
dws sheet create --name "项目资料" -f json
# 提取 nodeId 后:
dws sheet media-upload --node <NODE_ID> --file ./design.pdf -f json
dws sheet media-upload --node <NODE_ID> --file ./timeline.xlsx --name "项目时间线.xlsx" -f json

# ── 工作流 10: 写入图片到表格单元格 ──

# 1. 基本用法: 写入图片到指定单元格
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range A1:A1 --file ./chart.png -f json

# 2. 指定显示尺寸
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range B2:B2 --file ./logo.png --width 200 --height 100 -f json

# 3. 自定义图片名称
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range C3:C3 --file ./photo.jpg --name "产品图.jpg" -f json

# 4. 完整流程: 创建表格 → 写表头 → 写入图片
dws sheet create --name "产品目录" -f json
# 提取 nodeId 后:
dws sheet range update --node <NODE_ID> --sheet-id Sheet1 --range "A1:B1" --values '[["产品名称","产品图片"]]' -f json
dws sheet range update --node <NODE_ID> --sheet-id Sheet1 --range "A2:A2" --values '[["MacBook Pro"]]' -f json
dws sheet write-image --node <NODE_ID> --sheet-id Sheet1 --range B2:B2 --file ./macbook.png --width 150 --height 100 -f json
```

```bash
# ── 工作流 11: 筛选视图管理 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> -f json

# 2. 查看已有筛选视图
dws sheet filter-view list --node <NODE_ID> --sheet-id <SHEET_ID> -f json

# 3. 创建筛选视图（不带条件）
dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> \
  --name "我的筛选" --range "A1:E100" -f json

# 4. 为筛选视图设置列条件（按值筛选）
dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}' -f json

# 5. 为筛选视图设置列条件（按条件筛选）
dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}' -f json

# 6. 更新筛选视图名称和范围
dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --name "销售数据筛选" --range "A1:F200" -f json

# 7. 清除某列的筛选条件
dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 0 -f json

# 8. 删除筛选视图
dws sheet filter-view delete --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> -f json
```

```bash
# ── 工作流 11b: 创建带条件的筛选视图（一步完成） ──

# 创建筛选视图时直接指定筛选条件
dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> \
  --name "高销售额视图" --range "A1:E100" \
  --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50000"}]}]' \
  -f json
```

```bash
# ── 工作流 12: 导出表格为 xlsx（单命令一站式）──

# 场景 A：仅获取下载链接（命令内部自动完成提交+轮询，最终返回 downloadUrl）
dws sheet export --node <NODE_ID> --format json
# 传入 URL 也可：
# dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# 场景 B：导出并自动下载为本地文件
dws sheet export --node <NODE_ID> --output ./report.xlsx

# 场景 C：下载到目录，自动按链接推断文件名
dws sheet export --node <NODE_ID> --output ./

# 禁止在 Agent 侧实现任何轮询或重试，CLI 内部已按 2s/5s/10s/15s 渐进式退避自动完成（最多 30 次）。
# 若命令返回失败或超时，直接告知用户稍后再试，不要自动重调 dws sheet export。
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `nodeId` | list / info / new / range read / range update / find 的 --node |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |
| `new` | 新工作表的 `sheetId` | range read / range update / find 的 --sheet-id |
| `info` | `rowCount` / `lastNonEmptyRow` | 确定数据范围、追加写入起始行 |
| `find` | `matchedCells` 中的 `a1Notation` | 定位目标单元格，用于 range read / range update |
| `append` | `a1Notation` 追加数据所在范围 | 确认追加位置 |
| `csv-put` | `a1Notation` 实际写入的单元格范围 | 确认写入位置和范围 |
| `insert-dimension` | `a1Notation` 新插入区域范围 | 确认插入位置和范围 |
| `delete-dimension` | `a1Notation` 被删除区域范围 | 确认删除位置和范围 |
| `update-dimension` | `a1Notation` 被更新区域范围、`hidden` 生效的显隐状态、`pixelSize` 生效的尺寸 | 确认更新结果 |
| `merge-cells` | `a1Notation` 实际被合并的范围、`mergeType` 生效的合并方式 | 确认合并结果 |
| `media-upload` | `resourceId`、`resourceUrl` | 附件已上传到表格；`resourceUrl` 可用于 `create-float-image` 的 `--src` |
| `write-image` | `resourceId` | 图片已写入指定单元格 |
| `create-float-image` | `floatImage`（含 `id`、`src`、`range`、`width`、`height`、`offsetX`、`offsetY`） | `id` 用于后续 get / update / delete 的 `--float-image-id` |
| `get-float-image` | `floatImage`（完整信息） | 查看单个浮动图片详情 |
| `list-float-images` | `floatImages` 数组、`totalCount` | 获取所有浮动图片的 `id`，用于后续操作 |
| `update-float-image` | `floatImage`（更新后的完整信息） | 确认更新结果 |
| `delete-float-image` | `message` | 确认删除完成 |
| `replace` | `replaceCount` 被替换的单元格数量 | 确认替换结果 |
| `move-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `add-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `unmerge-cells` | `sheetId` 工作表 ID | 确认操作完成 |
| `set-dropdown` | `range` 实际设置范围、`optionCount` 选项数量、`enableMultiSelect` 是否多选 | 确认下拉列表设置成功 |
| `get-dropdown` | `hasDropdown` 是否存在下拉、`dataValidations` 下拉配置列表（含 `conditionValues`、`ranges`、`options`） | 查看已有下拉配置 |
| `delete-dropdown` | `range` 实际删除范围 | 确认下拉列表删除完成 |
| `filter-view list` | `filterViews` 筛选视图列表（含 `id`、`name`、`range`） | 获取 filterViewId 用于 info / update / delete / update-criteria / delete-criteria / list-criteria / get-criteria |
| `filter-view info` | `id`、`name`、`range`、`criteria` | 查看单个视图完整配置，确认条件是否生效 |
| `filter-view create` | `id` 筛选视图 ID、`name`、`range` | 用于后续 update / delete / update-criteria / delete-criteria 的 --filter-view-id |
| `filter-view update` | `id`、`name`、`range`、`criteria` | 确认更新结果 |
| `filter-view delete` | `id` 被删除的筛选视图 ID | 确认删除完成 |
| `filter-view update-criteria` | `id` 筛选视图 ID | 确认条件设置完成 |
| `filter-view delete-criteria` | `id` 筛选视图 ID | 确认条件清除完成 |
| `filter-view list-criteria` | 所有列条件（按列偏移量为 key 的对象） | 了解当前视图已设置哪些列的条件 |
| `filter-view get-criteria` | 指定列的条件详情（`filterType`、`conditions` 等） | 查看某列的具体筛选规则 |
| `export` | `downloadUrl`（未指定 --output）/ `导出完成: <path>`（指定 --output） | 直接下发给用户或告知文件已保存到本地。命令内部已完成轮询，不要再调用其他 export 相关命令 |

## nodeId 多格式说明

所有 `--node` 参数同时支持文档 ID、文档 URL、表格分享链接，系统自动识别。详细格式和提取规则请参见前文「URL 识别与 NODE_ID 提取」章节。

> ** 禁止使用 `dentryId`**：`drive list` 返回结果中同时包含 `dentryId`（纯数字，如 `218595998810`）和 `fileId`（UUID 格式，如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）两个字段。sheet 的 `--node` 和 `--folder` 参数**只能使用 `fileId`（UUID 格式）**，不能使用纯数字的 `dentryId`，传入纯数字会导致命令失败。

## values 参数格式说明

`--values` 为二维 JSON 数组，第一维为行，第二维为列：
- 字符串值: `"文本"`
- 数字值: `100` 或 `3.14`
- 公式: `"=SUM(B2:B4)"`（以 `=` 开头的字符串自动识别为公式）
- 清空单元格: 统一使用空字符串 `""`（不要用 `null` 取代，null 不会保留原值且全 null 会被视为无效调用跳过）

维度必须与 `--range` 范围一致，例如 `--range "A1:B3"` 需要 3 行 2 列的数组。

## hyperlinks 参数格式说明

`--hyperlinks` 为二维 JSON 数组，每个元素为对象或 null：
- `type`: 链接类型，可选 `path`（外部链接）、`sheet`（工作表跳转）、`range`（单元格跳转）
- `link`: 链接地址
- `text`: 显示文本

与 `--values` 共存时，hyperlinks 优先级更高。

## number-format 常用值

适用范围：`number-format` 仅在 `range set-style` / `range batch-set-style` 中接受（CLI 对应 `--number-format`，batch 配置文件对应 `numberFormat`）；`range update` 不接受该参数。

| 格式代码 | 说明 | 示例 |
|----------|------|------|
| `General` | 常规 | 1234.5 |
| `@` | 文本 | 001234 |
| `#,##0` | 整数千分位 | 1,235 |
| `#,##0.00` | 两位小数 | 1,234.50 |
| `0%` | 百分比 | 85% |
| `yyyy/m/d` | 日期 | 2026/3/15 |
| `hh:mm:ss` | 时间 | 14:30:00 |
| `¥#,##0` | 人民币 | ¥1,235 |

## 注意事项（完整版）

> 标 ★ 的条目已在前文「关键注意事项」中列出，此处为完整说明。

- ★ `--sheet-id` 获取规范（强制）：所有涉及 `--sheet-id` 参数的命令（`info` / `new` / `range read` / `range update` / `find` / `append` / `insert-dimension` / `delete-dimension` / `update-dimension` / `move-dimension` / `add-dimension` / `merge-cells` / `unmerge-cells` / `replace` / `write-image` / `set-dropdown` / `get-dropdown` / `delete-dropdown` / `filter-view *` 等），除非用户主动提供了工作表 ID 或工作表名称，否则在 `sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- ★ `range update` 维度校验（强制）：调用 `range update` 写入 `--values` 或 `--hyperlinks` 时，必须严格校验二维 JSON 数组的行数与列数与 `--range` 指定的范围完全一致：
  - 例如 `--range "A1:C3"` 表示 3 行 × 3 列，`--values` 必须是 `[[v1,v2,v3],[v4,v5,v6],[v7,v8,v9]]` 这样 3×3 的数组
  - `--range "A1"` 表示 1 行 × 1 列，`--values` 必须是 `[[v]]`
  - 行数不足需要用空字符串补齐，列数不足需要补齐到每行相同长度；禁止出现各行列数不一致或与 `--range` 不匹配的情况，否则调用会直接报错
  - 同时传入 `--values` 和 `--hyperlinks` 时，两个二维数组的行列数都必须与 `--range` 严格一致
- ★ `range update` 清空单元格规范（强制）：如需清空单元格内容，统一使用空字符串 `""`。禁止使用 `null`：`null` 不会保留单元格原值，也不存在"选择性保留"场景；且若 `--values` 全部为 `null`，整体调用会被视为无效而跳过，无任何写入效果
- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `list` 返回所有工作表的 ID 和名称，是后续操作的必要前置步骤
- `info` 不传 `--sheet-id` 时默认返回第一个工作表的详情
- `range read` 不传 `--range` 时默认读取整个工作表的全部非空数据
- `range read` 的 `--range` 支持 `Sheet1!A1:D10` 格式直接指定工作表（此时忽略 `--sheet-id`）
- `range read` 遇到超时或响应过慢时，应缩小 `--range` 查询范围，**单次读取的单元格数量建议控制在 5000 个以内**；数据量较大时通过 `info` 获取边界后分批读取，避免不传 `--range` 直接读取整个大工作表
- `range update` 的 `--values` 和 `--hyperlinks` 至少传入一项
- `range update` 职责边界：`range update` 仅写入单元格的值与超链接，不接受任何样式参数（包括但不限于数字格式 / 背景色 / 字体 / 对齐方式等）。如需设置数字格式，请使用 `dws sheet range set-style --number-format <格式代码>`；批量场景走 `dws sheet range batch-set-style --batch <config.json>`（配置文件中使用 `numberFormat` 字段）。不要在同一次 `range update` 调用里同时完成写值与格式设置
- ★ `range update` / `range set-style` / `range batch-set-style` 单次调用上限（强制）：行数 ≤ 1000，单元格总数（行×列）建议≤ 5000（底层硬限 30000）；超限请拆分多次调用。CLI 会在调用前做本地预校验，底层超 30000 会直接报错
- `range set-style` / `range batch-set-style` 的样式枚举按驼峰书写：`wordWrap` 取 `overflow`/`clip`/`autoWrap`，`fontWeight` 取 `bold`/`normal`，`hAlign` 取 `left`/`center`/`right`/`general`，`vAlign` 取 `top`/`middle`/`bottom`；背景色/字体颜色统一使用 `#RRGGBB` 格式
- `new` 创建工作表时，如名称与已有工作表重复，系统会自动重命名
- `find` 返回匹配单元格的地址（A1 表示法）和值，无匹配时返回空数组
- `find` 的 `--match-entire-cell` 用于精确匹配：只返回单元格内容完全等于搜索文本的结果，不会匹配包含该文本的单元格（例如搜索"苹果"时，只匹配"苹果"，不匹配"苹果手机""苹果汁"等）。用户说"精确搜索/完全匹配/只搜等于XX的"时必须使用此参数
- `find` 的 `--match-case` 默认为 true（区分大小写），设为 false 可忽略大小写
- `find` 的 `--use-regexp` 启用后，`--find` 参数作为正则表达式处理
- ★ 当用户要求搜索/查找表格数据时，使用 `find` 命令，不要用 `range read` 读取全量数据后自行过滤——`find` 支持服务端搜索，效率更高、语义更准确
- `append` 自动定位到最后一行有数据的位置下方插入，无需手动计算行号
- `append` 的 `--values` 二维数组中每行的列数必须一致，否则会报错。如果用户提供的数据中各行长度不同，必须先将短行用空字符串 `""` 补齐到与最长行相同的列数后再调用。追加的数据列数也应与工作表已有数据列数保持一致
- `append` vs `range update`：追加新行用 `append`，修改已有单元格用 `range update`
- `insert-dimension` 在指定位置之前插入空行或空列，不写入数据；如需在末尾追加行/列，使用 `append`
- `insert-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `insert-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `insert-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 从指定位置起删除若干连续的行或列，删除后后续行/列向前移动填补空位
- `delete-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `delete-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `delete-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 若需仅清空内容但保留行/列占位，请使用 `range update` 将目标区域写入空字符串 `""`（参见《range update 清空单元格规范》）
- `update-dimension` 批量更新连续行/列的显隐状态与行高/列宽
- `update-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `update-dimension` 的 `--start-index` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `update-dimension` 的 `--length` 最大为 5000
- `update-dimension` 的 `--hidden` 与 `--pixel-size` 至少必须提供一个
- `update-dimension` 的 `--pixel-size` 单位为像素，`dimension=ROWS` 时表示行高、`dimension=COLUMNS` 时表示列宽
- `update-dimension` 当同时提供 `--hidden` 与 `--pixel-size` 时，将先应用尺寸再应用显隐，任一失败整体失败
- `merge-cells` 合并时只保留左上角单元格的值，其他单元格的值会被丢弃
- `merge-cells` 的 `--merge-type` 不传时默认为 `mergeAll`（合并所有单元格）
- `merge-cells` 的 `--range` 支持带工作表前缀的写法（如 `Sheet1!A1:B3`），此时忽略 `--sheet-id`
- `merge-cells` 如果目标区域与其他合并单元格、锁定区域或表格区域存在交集，合并将失败
- `media-upload` 是两步自动完成的流程 (获取附件上传凭证 → OSS 上传)，无需手动分步操作
- `write-image` 是三步自动完成的流程 (获取附件上传凭证 → OSS 上传 → 写入图片到单元格)，无需手动分步操作
- ★ 向表格单元格中写入图片必须使用 `write-image`，禁止使用 `range update`。`range update` 底层调用的 `update_range` MCP 工具不支持图片类型参数，调用会失败
- `write-image` 与 `media-upload` 的区别：`media-upload` 仅上传附件到表格获取 resourceId；`write-image` 在上传后还会将图片写入指定单元格
- `create-float-image` 创建浮动图片前必须先通过 `media-upload` 上传图片获取 `resourceUrl`，再将其作为 `--src` 传入。`--src` 的格式为 `/core/api/resources/img/...`，不能直接传外部 URL
- `create-float-image` 的 `--range` 使用 A1 表示法指定锚点单元格（如 `A1`、`B3`），支持带工作表前缀（如 `Sheet1!A1`）
- `create-float-image` 的 `--width` / `--height` 为必填，单位像素，必须为正整数；`--offset-x` / `--offset-y` 可选，默认 0，不能为负数
- `write-image`（单元格内嵌图片）vs `create-float-image`（浮动图片）：`write-image` 将图片写入单元格内部，占据单元格内容；`create-float-image` 创建悬浮于单元格之上的浮动图片，不占用单元格内容，可自由调整位置和大小
- `update-float-image` 的 `--src` / `--range` / `--width` / `--height` / `--offset-x` / `--offset-y` 至少必须提供一个
- `list-float-images` 返回 `floatImages` 数组和 `totalCount`，每个元素包含 `id`（用于后续 get / update / delete）
- `delete-float-image` 操作不可恢复，删除后图片将从工作表中移除
- `replace` 的 `--find` 不能为空字符串，`--replace` 可以为空字符串（表示删除匹配内容）
- `replace` 的 `--match-case` 默认为 false（不区分大小写），与 `find` 的默认行为不同
- ★ `replace` vs `range update`：需要批量替换文本时，必须使用 `replace` 命令，禁止用 `range update` 手动重写单元格来实现替换效果。`replace` 支持服务端全局替换，效率更高且会返回替换计数
- ★ `move-dimension` vs `range update`：需要移动行或列时，必须使用 `move-dimension` 命令，禁止用 `range update` 读取数据后手动重写来模拟移动效果。`move-dimension` 是原子操作，能保留单元格的格式、合并状态等属性
- `move-dimension` 的索引均为 0-based（第 1 行/列的索引为 0），`endIndex` 包含在移动范围内
- `move-dimension` 的 `--destination-index` 不能在 [start-index, end-index] 范围内
- `move-dimension` 的移动跨度（end-index - start-index + 1）不超过 5000
- `move-dimension` 的 `--destination-index` 是目标位置的 0-based 索引，即移动到第 n 行/列则传 `n - 1`（通用公式：`destination-index = 目标行号(1-based) - 1`）
- `add-dimension` vs `range update`：需要在末尾追加空行/空列时，必须使用 `add-dimension` 命令，禁止用 `range update` 写空数据来模拟追加效果
- `add-dimension` 追加的是空行/空列，与 `append`（追加带数据的行）不同
- `add-dimension` 的 `--length` 必须为正整数（>= 1），行列均不超过 5000
- `unmerge-cells` 取消指定范围内所有合并单元格，使用 A1 表示法指定范围
- `set-dropdown` 在指定范围内设置下拉列表，`--options` 为 JSON 数组，每个元素包含 `value`（必填）和 `color`（可选，`#RRGGBB` 格式）。选项值不能包含英文逗号。`--multi-select` 启用多选模式。如果目标范围已存在下拉列表，会被新配置覆盖
- `get-dropdown` 查询指定范围内的下拉列表配置，返回 `dataValidations` 数组，相同选项的单元格聚合为一组。无下拉列表时 `hasDropdown` 为 false
- `delete-dropdown` 删除指定范围内的下拉列表配置，单元格恢复为普通文本格式。已填写的值不会被清除。目标范围不存在下拉列表时操作仍返回成功
- ★ **全局筛选（filter）与筛选视图（filter-view）的区别**：全局筛选影响所有协作者看到的数据展示，每个工作表最多一个；筛选视图是个人化的，互不影响。用户只说"筛选"时默认走 `filter` 系列
- `filter get` 获取工作表的全局筛选信息，返回 `range`（筛选范围）和 `columnFilterCriteria`（各列条件）。无筛选时返回空
- `filter create` 创建全局筛选时 `--range` 必须包含表头行（如 `A1:E100`），不能只包含数据行。每个工作表只能有一个筛选，已存在时报错
- `filter create` 的 `--criteria` 可选，不传则仅创建空筛选框架，后续通过 `filter update` 设置条件
- `filter delete` 删除后所有筛选条件丢失且所有被隐藏行重新显示，不可恢复
- `filter delete` 工作表没有筛选时调用会报错，应先 `filter get` 确认存在
- `filter update` 是覆盖式：指定列的条件会被替换，未指定的列保持不变。如只想修改某一列，建议先 `filter get` 读取现有配置再 patch
- `filter update` 前置：工作表必须已创建筛选
- `filter clear-criteria` 仅清除指定列的条件，不删除整个筛选。指定列无条件时不报错（幂等）
- `filter sort` 会实际改变数据行的物理顺序，不可撤销。前置：工作表必须已创建筛选
- ★ **筛选操作规范**（参照飞书 core-operations）：
  - 当用户要求"筛选/只看/仅保留 X"时，**必须**通过 `filter create` / `filter update` 创建真实的筛选器。**禁止**用"删除不符合条件的行"或"新建工作表只放符合条件的行"来代替
  - 创建/更新筛选后**必须** `filter get` 回读验证配置正确
  - 更新已有筛选前先 `filter get` 读取当前配置，确认目标存在且了解现有条件后再操作
  - 筛选条件的列索引（`column`）必须与实际数据列精确对应，不要凭猜测填写
  - 筛选不支持正则表达式，传入正则会当成普通文本处理
- `filter-view list` 获取指定工作表的所有筛选视图列表，返回的 `id` 可用于后续 info / update / delete / update-criteria / delete-criteria / list-criteria / get-criteria 的 `--filter-view-id`
- `filter-view info` 获取单个筛选视图的完整信息（含 criteria），内部复用 `get_filter_views` MCP 按 ID 过滤
- `filter-view list-criteria` 列出指定筛选视图已设置的所有列条件，返回按列偏移量为 key 的对象；无条件时返回空对象 `{}`
- `filter-view get-criteria` 获取指定列的条件详情，`--column` 为列偏移量（从 0 开始）；该列无条件时返回错误提示
- `filter-view create` 创建筛选视图时 `--range` 应包含表头行。`--criteria` 可选，不传则创建后无筛选条件，后续可通过 `filter-view update-criteria` 设置
- `filter-view update` 的 `--name`、`--range`、`--criteria` 至少需要传入一个，未指定的字段保持不变
- `filter-view update` 的 `--criteria` 中指定列的条件会被替换，未指定的列保持不变
- `filter-view delete` 删除后该视图及其所有筛选条件将被永久移除，不可恢复
- `filter-view delete` 不影响全局筛选或其他筛选视图
- `filter-view update-criteria` 的 `--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。例如筛选视图范围为 `B1:E10`，则 `--column 0` 代表 B 列
- `filter-view update-criteria` 设置条件后立即在该筛选视图中生效，仅影响当前视图，不影响全局筛选或其他筛选视图
- `filter-view update-criteria` 的 `--filter-criteria` 中 `conditions` 最多 2 个条件，多条件之间通过 `conditionOperator` 指定逻辑关系（`and` 或 `or`）
- `filter-view delete-criteria` 仅清除指定列的条件，不会删除整个筛选视图。如需删除整个筛选视图，请使用 `filter-view delete`
- `filter-view delete-criteria` 如果指定列没有设置筛选条件，调用不会报错
- 筛选视图相关操作需要"可阅读"权限（list / info / list-criteria / get-criteria）或"可编辑"权限（create / update / delete / update-criteria / delete-criteria），不支持跨组织操作
- ★ `export` 仅支持钉钉在线电子表格（alxs）→ xlsx；传入钉钉文字文档会报 `invalidRequest.document.typeIllegal`
- ★ `export` 为单命令一站式，CLI 内部已自动完成「提交 → 渐进式退避轮询 → 可选下载」，**Agent 不得在外部实现轮询或重试**；命令返回成功后不再调用其他 export 相关命令
- `export` 内置轮询策略：1~5 次间隔 2s、6~10 次间隔 5s、11~20 次间隔 10s、21~30 次间隔 15s，硬上限 30 次（约 5 分钟）；超时后命令返回错误，告知用户稍后再试即可
- ★ `export` 命令返回失败或超时时，**禁止自动重调 `dws sheet export`**；直接告知用户导出失败并建议稍后再试
- `export` 未指定 `--output` 时，返回的 `downloadUrl` 具有时效性，获取后请尽快下载；若用户需要本地文件，优先直接传 `--output` 让 CLI 代为下载
- `export` 的 `--output` 可为文件路径或已存在目录；为目录时自动从 `downloadUrl` 推断文件名，为文件路径时直接按该路径保存
- 用户要求"导出表格/下载 xlsx"时，必须使用 `export` 单命令，禁止用 `range read` 读全量数据后自行拼 xlsx 模拟导出（服务端导出会保留格式/合并/公式等完整属性）
- `update` 的 `--title`、`--index`、`--hidden`、`--frozen-row-count`、`--frozen-column-count` 至少必须提供一个
- `update` 的 `--title` 最长 100 字符，不能包含 `/ \ ? * [ ] :` 等特殊字符
- `update` 的 `--index` 为 0-based 非负整数，0 表示移动到最前面
- `update` 的 `--hidden` 设为 true 时，至少需要保留一个可见的工作表，不能将所有工作表都隐藏
- `update` 的 `--frozen-row-count` / `--frozen-column-count` 为非负整数，不能超过工作表的总行数/列数，设为 0 表示取消冻结
- `update` 当同时提供多个属性时，所有属性将在同一次请求中更新
- `copy` 复制操作会将源工作表的所有内容（包括数据、格式、公式等）完整复制到新工作表
- `copy` 的 `--title` 可选，不传时系统自动生成名称（通常为"源名称 副本"或类似格式）
- `copy` 的 `--title` 最长 100 字符，不能包含 `/ \ ? * [ ] :` 等特殊字符
- `copy` 当指定名称与已有工作表重复时，系统会自动重命名为合法值
- `copy` 的 `--index` 可选，不传时副本将放置在源工作表之后的默认位置
- ★ 关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)
- sheet 产品线仅支持 `axls`（在线电子表格，`contentType=ALIDOC`），不支持 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件
- 遇到未知 `alidocs` URL 时，必须先 probe（`dws doc info --node <URL> --format json`）确认 `contentType` 和 `extension`，才能决定是否走 sheet
- 当节点 `extension=xlsx` / `xls` / `xlsm` / `csv`（`contentType≠ALIDOC`）时，必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再处理，禁止调用任何 sheet 子命令（sheet 底层 MCP 工具只识别 axls，调用 xlsx 节点必失败）
- 要把在线表格导出为 xlsx 文件——走 `dws sheet export`（axls → xlsx 的格式转换）；要读已有的 xlsx 文件——走 `dws doc download` 后在本地解析，两者方向相反
