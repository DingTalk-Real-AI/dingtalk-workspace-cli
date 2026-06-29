# 媒体上传与图片 (media & image)

## 使用场景

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

## 命令详细参考

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

## 核心工作流

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

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `media-upload` | `resourceId`、`resourceUrl` | 附件已上传到表格；`resourceUrl` 可用于 `create-float-image` 的 `--src` |
| `write-image` | `resourceId` | 图片已写入指定单元格 |
| `create-float-image` | `floatImage`（含 `id`、`src`、`range`、`width`、`height`、`offsetX`、`offsetY`） | `id` 用于后续 get / update / delete 的 `--float-image-id` |
| `get-float-image` | `floatImage`（完整信息） | 查看单个浮动图片详情 |
| `list-float-images` | `floatImages` 数组、`totalCount` | 获取所有浮动图片的 `id`，用于后续操作 |
| `update-float-image` | `floatImage`（更新后的完整信息） | 确认更新结果 |
| `delete-float-image` | `message` | 确认删除完成 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- `media-upload` 是两步自动完成的流程 (获取附件上传凭证 → OSS 上传)，无需手动分步操作
- `write-image` 是三步自动完成的流程 (获取附件上传凭证 → OSS 上传 → 写入图片到单元格)，无需手动分步操作
- ★ 向表格单元格中写入图片必须使用 `write-image`，禁止使用 `range update`。`range update` 底层调用的 `update_range` MCP 工具不支持图片类型参数，调用会失败
- `write-image` 与 `media-upload` 的区别：`media-upload` 仅上传附件到表格获取 resourceId；`write-image` 在上传后还会将图片写入指定单元格
- `create-float-image` 创建浮动图片前必须先通过 `media-upload` 上传图片获取 `resourceUrl`，再将其作为 `--src` 传入。`--src` 的格式为 `/core/api/resources/img/...`，不能直接传外部 URL
- `create-float-image` 的 `--range` 使用 A1 表示法指定锚点单元格（如 `A1`、`B3`），支持带工作表前缀（如 `Sheet1!A1`）
- `create-float-image` 的 `--width` / `--height` 为必填，单位像素，必须为正整数；`--offset-x` / `--offset-y` 可选，默认 0，不能为负数
- `write-image`（单元格内嵌图片）vs `create-float-image`（浮动图片）：`write-image` 将图片写入单元格内部，占据单元格内容；`create-float-image` 创建悬浮于单元格之上的浮动图片，不占用单元格内容，可自由调整位置和大小
- ★ **浮动图片用 `create-float-image` 不用 `write-image`**：两者用途不同——`write-image` 写入单元格内部，`create-float-image` 创建悬浮于单元格之上的浮动图片；`--src` 必须来自 `media-upload` 的 `resourceUrl`
- `update-float-image` 的 `--src` / `--range` / `--width` / `--height` / `--offset-x` / `--offset-y` 至少必须提供一个
- `list-float-images` 返回 `floatImages` 数组和 `totalCount`，每个元素包含 `id`（用于后续 get / update / delete）
- `delete-float-image` 操作不可恢复，删除后图片将从工作表中移除
