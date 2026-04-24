# AI表格 (aitable) 命令参考

## 文档地址 (URI)

所有 AI 表格操作完成后，可通过以下 URI 直接访问对应文档：

| 资源 | URI 格式 |
|------|----------|
| Base 文档 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` |
| 模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` |

> 💡 **操作后请返回文档 URI**：每次执行 base list/search/create/get 操作后，从返回数据中提取 `baseId`，拼接为 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 返回给用户，方便直接点击打开。

## 命令总览

### base (Base 管理)

#### 获取 AI 表格列表
```
Usage:
  dws aitable base list [flags]
Example:
  dws aitable base list
  dws aitable base list --limit 5 --cursor NEXT_CURSOR
Flags:
      --cursor string   分页游标，首次不传
      --limit int       每页数量，默认 10，最大 10
```

返回 baseId 与 baseName。

> 📎 **操作后返回文档链接**：遍历返回的每个 base，拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 返回给用户。

> ⚠️ **重要**：`base list` 仅返回**最近访问过**的 Base，**不是全部 Base**。
> 如需查找表格，**请优先使用 `base search`**；`base list` 仅作为浏览最近表格的辅助手段。
> 如果刚创建完，直接使用 `create` 返回的 `baseId` 即可。

#### 搜索 AI 表格
```
Usage:
  dws aitable base search [flags]
Example:
  dws aitable base search --query "项目管理"
Flags:
      --cursor string   分页游标，首次不传
      --query string    Base 名称关键词，建议至少 2 个字符 (必填)
```

#### 获取 AI 表格信息
```
Usage:
  dws aitable base get [flags]
Example:
  dws aitable base get --base-id <BASE_ID>
Flags:
      --base-id string   Base 唯一标识 (必填)
```

> 💡 **用户提供 URL 时**：如果用户给出了链接如 `https://alidocs.dingtalk.com/i/nodes/ABC123`，请提取末尾的 `ABC123` 作为 `--base-id` 传入。详见下方「URL → baseId 提取」章节。

返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
后续如需 tableId，优先从这里读取。

> 📎 **文档地址**：`https://alidocs.dingtalk.com/i/nodes/{baseId}`

#### 创建 AI 表格
```
Usage:
  dws aitable base create [flags]
Example:
  dws aitable base create --name "项目跟踪"
Flags:
      --name string          Base 名称，1-50 字符 (必填)
      --template-id string   模板 ID (可选，可通过 template search 获取)
```

> 💡 **创建后直接使用返回的 `baseId`**，无需再调用 `base list` 或 `base search` 查找。
> 后续可直接 `base get --base-id <返回的baseId>` 获取 tableId，或 `table create --base-id <返回的baseId>` 创建数据表。
>
> 📎 **文档地址**：`https://alidocs.dingtalk.com/i/nodes/{返回的baseId}`

#### 更新 AI 表格
```
Usage:
  dws aitable base update [flags]
Example:
  dws aitable base update --base-id <BASE_ID> --name "新名称"
Flags:
      --base-id string   目标 Base ID (必填)
      --desc string      备注文本
      --name string      新名称，1-50 字符 (必填)
```

#### 删除 AI 表格
```
Usage:
  dws aitable base delete [flags]
Example:
  dws aitable base delete --base-id <BASE_ID> --yes
Flags:
      --base-id string   待删除 Base ID (必填)
      --reason string    删除原因
```

高风险操作，不可逆。

### table (数据表管理)

#### 获取数据表
```
Usage:
  dws aitable table get [flags]
Example:
  dws aitable table get --base-id <BASE_ID>
  dws aitable table get --base-id <BASE_ID> --table-ids tbl1,tbl2
Flags:
      --base-id string     所属 Base ID (必填)
      --table-ids string   Table ID 列表，逗号分隔，单次最多 10 个
```

返回 tableId、tableName、description、fields 目录、views 目录。不传 table-ids 返回全部表。

> 📎 **文档地址**：`https://alidocs.dingtalk.com/i/nodes/{baseId}`

#### 创建数据表
```
Usage:
  dws aitable table create [flags]
Example:
  dws aitable table create --base-id <BASE_ID> --name "任务表" \
    --fields '[{"fieldName":"任务名称","type":"text"},{"fieldName":"优先级","type":"singleSelect","config":{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}}]'
Flags:
      --base-id string   目标 Base ID (必填)
      --fields string    初始字段 JSON 数组，至少 1 个，单次最多 15 个 (必填)
      --name string      表格名称，1-100 字符 (必填)
```

> 💡 **创建后直接使用返回的 `tableId`**，无需再调 `table get` 查找。
> 后续可直接 `field create --table-id <返回的tableId>` 补充字段，或 `record create` 写入数据。
>
> 📎 **文档地址**：`https://alidocs.dingtalk.com/i/nodes/{baseId}`

#### 更新数据表
```
Usage:
  dws aitable table update [flags]
Example:
  dws aitable table update --base-id <BASE_ID> --table-id <TABLE_ID> --name "新表名"
Flags:
      --base-id string    所属 Base ID (必填)
      --name string       新表名 (必填)
      --table-id string   目标 Table ID (必填)
```

#### 删除数据表
```
Usage:
  dws aitable table delete [flags]
Example:
  dws aitable table delete --base-id <BASE_ID> --table-id <TABLE_ID> --yes
Flags:
      --base-id string    目标 Base ID (必填)
      --reason string     删除原因
      --table-id string   将被删除的 Table ID (必填)
```

不可逆。若为 Base 中最后一张表，删除会失败。

### field (字段管理)

#### 获取字段详情
```
Usage:
  dws aitable field get [flags]
Example:
  dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --field-ids fld1,fld2
Flags:
      --base-id string     Base ID (必填)
      --field-ids string   字段 ID 列表，逗号分隔，单次最多 10 个
      --table-id string    Table ID (必填)
```

返回字段的完整配置（含 options 等）。在 table get 拿到字段目录后，按需展开少量字段的完整配置。

#### 创建字段
```
Usage:
  dws aitable field create [flags]
Example:
  # 单字段模式
  dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --name "状态" --type "singleSelect" --config '{"options":[{"name":"待办"},{"name":"进行中"},{"name":"已完成"}]}'

  # 批量模式
  dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --fields '[{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"}]}}]'
Flags:
      --base-id string    Base ID (必填)
      --name string       单字段名称（与 --type 配合使用，替代 --fields）
      --type string       单字段类型（参考 table create 字段类型）
      --config string     单字段配置 JSON（可选，如 options）
      --fields string     批量新增字段 JSON 数组，单次最多 15 个（与 --name/--type 二选一）
      --table-id string   Table ID (必填)
```

允许部分成功，返回结果逐项标明成功/失败状态。
`--name/--type/--config` 为单字段模式；`--fields` 为批量模式；两种模式二选一。

#### 更新字段
```
Usage:
  dws aitable field update [flags]
Example:
  dws aitable field update --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --name "新字段名"
  dws aitable field update --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --config '{"options":[{"name":"A"},{"name":"B"}]}'
Flags:
      --ai-config string   AI 字段配置 JSON（AI 字段类型专用，可选）
      --base-id string     Base ID (必填)
      --config string      字段配置 JSON (不修改时省略)
      --field-id string    Field ID (必填)
      --name string        新字段名称 (不修改时省略)
      --table-id string    Table ID (必填)
```

不可变更字段类型。更新 singleSelect/multipleSelect 的 options 时需传入完整列表，已有选项应回传原 id。

#### 删除字段
```
Usage:
  dws aitable field delete [flags]
Example:
  dws aitable field delete --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --yes
Flags:
      --base-id string    Base ID (必填)
      --field-id string   待删除字段 ID (必填)
      --table-id string   Table ID (必填)
```

不可逆。禁止删除主字段和最后一个字段。

### record (记录管理)

#### 查询记录
```
Usage:
  dws aitable record query [flags]
Example:
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec1,rec2
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --query "关键词" --limit 50
Flags:
      --base-id string      Base ID (必填)
      --cursor string       分页游标，首次不传
      --field-ids string    返回字段 ID 列表，逗号分隔，单次最多 100 个
      --filters string      结构化过滤条件 JSON
      --query string        全文关键词搜索
      --limit int           单次最大记录数，默认 100，最大 100
      --record-ids string   指定记录 ID 列表，逗号分隔，单次最多 100 个
      --sort string         排序条件 JSON 数组
      --table-id string     Table ID (必填)
```

两种模式: 按 ID 取（传 record-ids，忽略 filters/sort）或条件查（filters+sort+cursor 分页）。

> ⚠️ **排序参数规范（关键）**：`--sort` 需要传 JSON 数组，排序方向字段必须是 `direction`（`asc` 或 `desc`），**不要使用 `order`**。
>
> 正确示例：`--sort '[{"fieldId":"wm8ns9bw2vmucb45xj3ix","direction":"desc"}]'`

filters 结构：`{"operator":"and|or","operands":[{"operator":"<op>","operands":["<fieldId>","<value>"]}]}`

> 💡 **singleSelect/multipleSelect 过滤**：filters 中可传 option id 或 option name，但建议优先用 **option id**（通过 `field get` 获取），更可靠。
> 写入时（record create/update）可直接传 option name。

> 💡 **减少响应体积**：字段较多时，用 `--field-ids` 仅返回需要的字段，可显著减少返回数据量。

#### 新增记录
```
Usage:
  dws aitable record create [flags]
Example:
  dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"cells":{"fldTextId":"文本内容","fldNumId":123}}]'
Flags:
      --base-id string    Base ID (必填)
      --records string    记录列表 JSON 数组，单次最多 100 条 (必填)
      --table-id string   Table ID (必填)
```

> ⚠️ **常见错误（严格避免）**：
> - **参数名是 `--records`**，不是 `--data`
> - **cells 的 key 必须是 fieldId**（如 `fldXXX`），**不是字段名称**（如 `"课程名称"`）
> - 必须先 `table get` 获取 fieldId，再写入记录

```bash
# 正确流程：先获取 fieldId
dws aitable table get --base-id <BASE_ID> --table-id <TABLE_ID> --format json
# 从返回中提取 fieldId（如 fldABC123）

# 再用 fieldId 写入记录
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldABC123":"Python入门"}}]' --format json
```

#### 更新记录
```
Usage:
  dws aitable record update [flags]
Example:
  dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]'
Flags:
      --base-id string    Base ID (必填)
      --records string    待更新记录 JSON 数组，单次最多 100 条 (必填)
      --table-id string   Table ID (必填)
```

只需传入需修改的字段，未传入的保持原值。每条记录必须含 recordId 和 cells。

#### 删除记录
```
Usage:
  dws aitable record delete [flags]
Example:
  dws aitable record delete --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec1,rec2 --yes
Flags:
      --base-id string      Base ID (必填)
      --record-ids string   待删除记录 ID 列表，逗号分隔，最多 100 条 (必填)
      --table-id string     Table ID (必填)
```

不可逆。调用前建议先 record query 确认目标记录。

### attachment (附件管理)

> 🛑 **STOP — 不要使用钉盘 (drive) 上传！** 钉盘 fileId 无法写入 attachment 字段。必须使用以下流程。

#### 准备附件上传
```
Usage:
  dws aitable attachment upload [flags]
Example:
  dws aitable attachment upload --base-id <BASE_ID> --file-name report.xlsx --size 204800
  dws aitable attachment upload --base-id <BASE_ID> --file-name photo.png --size 1024 --mime-type image/png
Flags:
      --base-id string     Base ID (必填)
      --file-name string   文件名，必须含扩展名 (必填)
      --size int           文件大小（字节），>0 (必填)
      --mime-type string   MIME type（不传时根据扩展名推断）
```

#### 附件上传完整流程（推荐：使用脚本，2 步完成）

```bash
# 步骤 1: 使用脚本一键上传（内部自动完成 prepare + PUT）
python3 scripts/upload_attachment.py <BASE_ID> /path/to/report.pdf
# 输出: { "fileToken": "ft_xxx", "fileName": "report.pdf", "size": 204800 }

# 步骤 2: 在 record create/update 中使用 fileToken 写入
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

> ⚠️ `uploadUrl` 有时效性（`expiresAt`），脚本会自动在获取后立即上传。

### view (视图管理)

视图是同一张数据表的筛选/排序/分组/可见字段组合的备用呈现。record query 可以通过视图收敛查询范围。

#### 获取视图
```
Usage:
  dws aitable view get [flags]
Example:
  dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID> --view-ids viw1,viw2
Flags:
      --base-id string    Base ID (必填)
      --table-id string   Table ID (必填)
      --view-ids string   View ID 列表，逗号分隔，单次最多 10 个
```

返回视图配置（过滤、排序、可见字段、分组）。不传 view-ids 返回该表全部视图。

#### 创建视图
```
Usage:
  dws aitable view create [flags]
Example:
  dws aitable view create --base-id <BASE_ID> --table-id <TABLE_ID> --name "进行中看板" --view-type kanban
Flags:
      --base-id string     Base ID (必填)
      --config string      视图配置 JSON：过滤/排序/分组/可见字段（可选，创建后可再 update）
      --name string        视图名称 (必填)
      --table-id string    Table ID (必填)
      --view-type string   视图类型：grid/gallery/kanban/gantt/calendar/form 等 (必填)
```

> ⚠️ 视图类型参数是 `--view-type`，不是 `--type`。

#### 更新视图
```
Usage:
  dws aitable view update [flags]
Example:
  dws aitable view update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --name "新视图名"
Flags:
      --base-id string    Base ID (必填)
      --config string     视图配置 JSON：过滤/排序/分组/可见字段
      --name string       新视图名
      --table-id string   Table ID (必填)
      --view-id string    目标 View ID (必填)
```

调整过滤、排序、分组、可见字段时使用。不重建视图即可替换配置。

#### 删除视图
```
Usage:
  dws aitable view delete [flags]
Example:
  dws aitable view delete --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --yes
Flags:
      --base-id string    Base ID (必填)
      --table-id string   Table ID (必填)
      --view-id string    待删除 View ID (必填)
```

不可逆。若是主视图或最后一个视图，删除会失败。

### import (数据导入)

把外部 Excel/CSV 批量写入某张数据表，按两步走：先 upload 拿 importId，再 data 触发导入。

#### 准备导入上传
```
Usage:
  dws aitable import upload [flags]
Example:
  dws aitable import upload --base-id <BASE_ID> --file-name sales.xlsx --file-size 204800
Flags:
      --base-id string     Base ID (必填)
      --file-name string   文件名，必须含扩展名 (必填)
      --file-size int      文件大小（字节），>0 (必填)
```

> ⚠️ 参数是 `--file-size`，不是 `--size`。返回 uploadUrl（PUT 上传）与 importId。

#### 触发导入
```
Usage:
  dws aitable import data [flags]
Example:
  dws aitable import data --import-id <IMPORT_ID> --timeout 60
Flags:
      --import-id string   import upload 返回的 importId (必填)
      --timeout int        等待导入完成的超时秒数，默认 30
```

把上一步上传到暂存区的文件正式导入为记录；超时未完成时返回处理状态，按 importId 再次查询即可。

### dashboard (仪表盘)

仪表盘 = 多个图表的布局容器；图表绑定数据表/视图。创建仪表盘之前通常先查 `dashboard config-example`，拿到 JSON 骨架，再把图表塞进 `--config` 的布局里。

#### 仪表盘配置示例
```
Usage:
  dws aitable dashboard config-example [flags]
Example:
  dws aitable dashboard config-example --format json
```

返回仪表盘 `--config` 的 JSON 示例，供 create/update 复制裁剪。

#### 获取仪表盘
```
Usage:
  dws aitable dashboard get [flags]
Example:
  dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID>
Flags:
      --base-id string        Base ID (必填)
      --dashboard-id string   Dashboard ID (必填)
```

返回布局和 `charts[].chartId`，可直接喂给 `chart get`。

#### 创建仪表盘
```
Usage:
  dws aitable dashboard create [flags]
Example:
  dws aitable dashboard create --base-id <BASE_ID> --config '<JSON>'
Flags:
      --base-id string   Base ID (必填)
      --config string    仪表盘配置 JSON（名称、布局、图表列表都在里面） (必填)
```

> ⚠️ 没有独立的 `--name`；名称和布局一起放在 `--config` JSON 中。

#### 更新仪表盘
```
Usage:
  dws aitable dashboard update [flags]
Example:
  dws aitable dashboard update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --config '<JSON>'
Flags:
      --base-id string        Base ID (必填)
      --config string         更新后的仪表盘配置 JSON (必填)
      --dashboard-id string   Dashboard ID (必填)
```

调整布局、增删图表一律改 `--config`。

#### 删除仪表盘
```
Usage:
  dws aitable dashboard delete [flags]
Example:
  dws aitable dashboard delete --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --yes
Flags:
      --base-id string        Base ID (必填)
      --dashboard-id string   Dashboard ID (必填)
      --reason string         删除原因
```

不可逆。

#### 查看仪表盘分享状态
```
Usage:
  dws aitable dashboard share get [flags]
Example:
  dws aitable dashboard share get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID>
Flags:
      --base-id string        Base ID (必填)
      --dashboard-id string   Dashboard ID (必填)
```

> ⚠️ 可能返回 404（资源不存在或未开通外链），按可重试错误处理，不要误判为参数拼错。

#### 更新仪表盘分享
```
Usage:
  dws aitable dashboard share update [flags]
Example:
  dws aitable dashboard share update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --enabled true
Flags:
      --allow-back-to-doc     是否允许从分享页返回原文档（可选）
      --base-id string        Base ID (必填)
      --dashboard-id string   Dashboard ID (必填)
      --enabled               是否开启外链分享 (必填)
      --share-type string     分享类型（权限/可见范围等，可选）
```

开启后返回 shareUrl；关闭后原链接失效。

### chart (图表)

图表挂在某个仪表盘下，绑定数据表（可进一步绑定视图）。新建前通常先查 `chart widgets-example`。

#### 图表组件示例
```
Usage:
  dws aitable chart widgets-example [flags]
Example:
  dws aitable chart widgets-example --format json
```

返回图表 widget 的 JSON 示例，供 `chart create/update --config` 参考。

#### 获取图表
```
Usage:
  dws aitable chart get [flags]
Example:
  dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID>
Flags:
      --base-id string        Base ID (必填)
      --chart-id string       Chart ID (必填)
      --dashboard-id string   Dashboard ID (必填)
```

#### 创建图表
```
Usage:
  dws aitable chart create [flags]
Example:
  dws aitable chart create --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --config '<JSON>'
Flags:
      --base-id string        Base ID (必填)
      --config string         图表配置 JSON：数据源、维度、度量、样式 (必填)
      --dashboard-id string   挂载到的 Dashboard ID (必填)
      --layout string         图表布局 JSON（位置、尺寸，可选）
```

> ⚠️ 名称和类型都在 `--config` JSON 里，没有独立 `--name` / `--type`。

#### 更新图表
```
Usage:
  dws aitable chart update [flags]
Example:
  dws aitable chart update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --config '<JSON>'
Flags:
      --base-id string        Base ID (必填)
      --chart-id string       Chart ID (必填)
      --config string         图表配置 JSON (必填)
      --dashboard-id string   Dashboard ID (必填)
      --layout string         图表布局 JSON（位置、尺寸，可选）
```

#### 删除图表
```
Usage:
  dws aitable chart delete [flags]
Example:
  dws aitable chart delete --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --yes
Flags:
      --base-id string        Base ID (必填)
      --chart-id string       待删除 Chart ID (必填)
      --dashboard-id string   Dashboard ID (必填)
      --reason string         删除原因
```

不可逆。

#### 查看图表分享状态
```
Usage:
  dws aitable chart share get [flags]
Example:
  dws aitable chart share get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID>
Flags:
      --base-id string        Base ID (必填)
      --chart-id string       Chart ID (必填)
      --dashboard-id string   Dashboard ID (必填)
```

返回 `enabled` 与 `shareUrl`，用来判断是否已经对外分享。

#### 更新图表分享
```
Usage:
  dws aitable chart share update [flags]
Example:
  dws aitable chart share update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --enabled true
Flags:
      --allow-back-to-doc     是否允许从分享页返回原文档（可选）
      --base-id string        Base ID (必填)
      --chart-id string       Chart ID (必填)
      --dashboard-id string   Dashboard ID (必填)
      --enabled               是否开启外链分享 (必填)
      --share-type string     分享类型（权限/可见范围等，可选）
```

### export (数据导出)

把数据表/视图/整张 Base 导出为 Excel/CSV，下发下载链接；常见是异步任务，首次调用可能只返回 `taskId`，需要按 `taskId` 继续轮询直到拿到 `downloadUrl`。

#### 导出数据
```
Usage:
  dws aitable export data [flags]
Example:
  # 第一步：创建任务（按 scope 传必要参数）
  dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --format excel --timeout-ms 1000

  # 第二步：拿 taskId 继续轮询，直到返回 downloadUrl
  dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000
Flags:
      --base-id string     Base ID (必填)
      --scope string       导出范围：all / table / view
      --table-id string    Table ID（scope=table/view 时必填）
      --view-id string     View ID（scope=view 时必填）
      --format string      导出格式：excel / csv，默认 excel
      --task-id string     轮询已有任务的 taskId
      --timeout-ms int     单次调用服务端等待时间（毫秒）
```

参数约束：

- `scope=all`：只需 `--base-id`
- `scope=table`：必须同时传 `--table-id`
- `scope=view`：必须同时传 `--table-id + --view-id`

### template (模板搜索)

#### 搜索模板
```
Usage:
  dws aitable template search [flags]
Example:
  dws aitable template search --query "项目管理"
Flags:
      --cursor string   分页游标，首次不传
      --limit int       每页返回数量，默认 10，最大 30
      --query string    模板名称关键词 (必填)
```

返回 templateId 可用于 `base create --template-id`。

> 📎 **模板预览地址**：`https://docs.dingtalk.com/table/template/{templateId}`

## 复杂操作

### 仪表盘 / 图表（建议顺序）

```bash
# 1) 先看配置模板（JSONC）
dws aitable dashboard config-example --format json
dws aitable chart widgets-example --format json

# 2) 先拿 dashboard，再拿 chart 详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json

# 3) 写入/更新都用 --config JSON（名称、布局、图表类型都在里面）
dws aitable dashboard create --base-id <BASE_ID> --config '<JSON>' --format json
dws aitable chart update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --config '<JSON>' --format json
```

要点：

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用。
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错。
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断。
- dashboard/chart 的 create/update 只有 `--config`，没有独立的 `--name` / `--type`，名称与布局都在 JSON 里。

### 导入外部数据（两步走）

```bash
# 1) 先上传文件拿 importId
dws aitable import upload --base-id <BASE_ID> --file-name sales.xlsx --file-size 204800 --format json
# 把本地文件 PUT 到返回的 uploadUrl（过期前必须上传完成）

# 2) 触发导入
dws aitable import data --import-id <IMPORT_ID> --timeout 60 --format json
```

要点：`--file-size` 单位字节，必须与实际文件一致；`--timeout` 单位秒，不是毫秒。

## 意图判断

用户说"表格/多维表/AI表格":
- 查看/查找/列表 → `base search`（优先）或 `base list`（仅浏览最近访问）
- 搜索 → `base search`
- 详情 → `base get`
- 创建 → `base create`
- 修改 → `base update`
- 删除 → `base delete` [危险]

用户说"数据表/子表/table":
- 查看 → `table get`
- 创建 → `table create`
- 重命名 → `table update`
- 删除 → `table delete` [危险]

用户说"字段/列/column":
- 查看 → `field get`
- 添加 → `field create`
- 修改 → `field update`
- 删除 → `field delete` [危险]

用户说"记录/行/数据/row":
- 查看/搜索 → `record query`（先 `table get` 获取 fieldId）
- 添加/写入 → `record create`（先 `table get` 必须!）
- 修改/更新 → `record update`（需 recordId，先 `record query`）
- 删除 → `record delete` [危险]（需 recordId）

用户说"视图/筛选视图/看板/画廊":
- 查看 → `view get`
- 新建 → `view create`（`--view-type` 指定视图类型）
- 修改（过滤/排序/分组/可见字段） → `view update`
- 删除 → `view delete` [危险]

用户说"导入/上传 Excel/CSV":
- 外部文件导入 → `import upload` → PUT 文件 → `import data`（两步）

用户说"导出/下载表格数据":
- → `export data`（按 scope=all/table/view 传参，异步轮询）

用户说"仪表盘/dashboard":
- 查看布局 → `dashboard get`
- 查看模板 → `dashboard config-example`
- 新建/修改布局 → `dashboard create` / `dashboard update`（配置都在 `--config` JSON）
- 删除 → `dashboard delete` [危险]
- 对外分享 → `dashboard share get` / `dashboard share update`

用户说"图表/chart":
- 查看 → `chart get`（需要 dashboardId）
- 查看组件模板 → `chart widgets-example`
- 新建/修改 → `chart create` / `chart update`（配置都在 `--config` JSON）
- 删除 → `chart delete` [危险]
- 对外分享 → `chart share get` / `chart share update`

用户说"附件/文件字段" → `attachment upload`（不要用钉盘 drive）

用户说"模板" → `template search`

关键区分: base=表格文件, table=数据表, field=列, record=行, view=视图, dashboard=仪表盘, chart=图表

## 核心工作流

```bash
# 1. 搜索/列出 Base — 提取 baseId
dws aitable base search --query "项目" --format json

# 2. 获取 Base 信息 — 提取 tableId
dws aitable base get --base-id <BASE_ID> --format json

# 3. 获取表结构 — 提取 fieldId
dws aitable table get --base-id <BASE_ID> --table-id <TABLE_ID> --format json

# 4. 查询记录
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --format json

# 5. 新增记录 (cells 用 fieldId 作 key)
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldXXX":"值"}}]' --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `base list/search` | `baseId` | 所有后续命令的 --base-id，拼接文档 URI |
| `base create` | `baseId` | 后续命令 + 文档 URI |
| `base get` | `tables[].tableId`、`dashboards[].dashboardId` | --table-id / --dashboard-id |
| `table get` | `fields[].fieldId`、`views[].viewId` | record 操作的 cells key；field get/update/delete；view 操作 |
| `view get` | `viewId` | record query 按视图收敛；view update/delete |
| `record query` | `recordId` | record update/delete |
| `dashboard get` | `charts[].chartId` | chart get/update/delete/share |
| `import upload` | `importId`、`uploadUrl` | PUT 文件后调 import data |
| `export data` (首次) | `taskId` | 下一轮 export data 轮询 |
| `attachment upload` | `fileToken` | record create/update 写入 attachment 字段 |
| `template search` | `templateId` | base create --template-id，拼接模板预览 URI |

## 注意事项

- 所有操作使用 ID（baseId/tableId/fieldId/recordId/viewId/dashboardId/chartId），不使用名称
- records 的 cells key 是 fieldId，不是字段名称
- dashboard/chart 的 create/update 只有 `--config`（JSON 内含名称/布局/类型），没有独立 `--name`
- view 创建的类型参数是 `--view-type`，不是 `--type`
- import upload 的文件大小参数是 `--file-size`，不是 `--size`

## `--filters` 筛选语法排错与使用规范（极易出错）

调用 `record query` 时，如果条件筛选**完全失效（查询返回了所有记录）**，通常是因为 `--filters` JSON 语法错误，API 默默丢弃了不合规的 filter。

**强制规则：**
1. **根节点必须是逻辑操作符**：`"operator"` 必须是 `"and"` 或 `"or"`，不能是 `"eq"` 等比较操作符。
2. 比较操作必须放在根节点的 `"operands"` 数组内的对象中。
3. `singleSelect` 和 `multipleSelect` 字段，推荐使用 **选项的 exact String 名称 (name)** 作为比较值，而不是 ID。
4. **内层比较操作符语义**：支持 `eq`(等于)、`not_eq`(不等于)、`contain`(包含/模糊搜索)、`not_contain`(不包含)、`gt/gte`(大于/大于等于)、`lt/lte`(小于/小于等于)、`is_empty/is_not_empty`(为空/不为空，对应 operands 内只传单个 fieldId)。

**精简防呆模板与 4 种衍生情况**
```json
{
  "operator": "and",       // 情况 4 (OR 查询): 这里改为 "or"
  "operands": [
    {
      "operator": "eq",    // 情况 3 (文本包含): 这里改为 "contain"
      "operands": ["fld_state", "进行中"] // 情况 1 (基础等于)
    }
    // 情况 2 (多条件 AND): 在此行增加类似 {"operator":"eq","operands":["fld_priority","高"]}
  ]
}
```

**错误示例 1：缺失根节点 and/or**（API 将忽略该 filter，返回全表）
```json
{"operator":"eq","operands":["fldXXX","本科"]}
```

**错误示例 2：传入选项 ID 而非名称**（可能导致匹配不到 0 记录）
```json
{"operator":"and","operands":[{"operator":"eq","operands":["fldXXX","CXzrOHK9JI"]}]}
```

## URL → baseId 提取

用户经常通过钉钉链接指定表格，链接格式为：

```
https://alidocs.dingtalk.com/i/nodes/{baseId}
https://alidocs.dingtalk.com/i/nodes/{baseId}?xxx=yyy
```

**处理规则**（必须严格遵守）：
1. 当用户提供了包含 `alidocs.dingtalk.com/i/nodes/` 的 URL 时，提取 `/nodes/` 后的路径段作为 `baseId`
2. 去掉尾部的查询参数（`?` 及其后内容）和尾部斜杠
3. 将提取得到的 ID 传入 `--base-id` 参数

**示例**：
```
用户输入：帮我查看 https://alidocs.dingtalk.com/i/nodes/ABC123XYZ 这个表格
→ 提取 baseId = ABC123XYZ
→ 执行: dws aitable base get --base-id ABC123XYZ --format json
```

> 💡 **注意**：URL 中的 nodeId 在 AI 表格场景下等同于 baseId，可以直接作为 `--base-id` 使用。

### cells 写入/读取格式速查

| 字段类型 | 写入格式 | 读取返回格式 |
|---------|---------|------------|
| text | `"字符串"` | `"字符串"` |
| number | `123` | `"123"` |
| singleSelect | `"选项名"` 或 `{"id":"xxx"}` | `{"id":"xxx","name":"选项名"}` |
| multipleSelect | `["选项A","选项B"]` | `[{"id":"x","name":"选项A"},...]` |
| date | `"2026-03-13"` 或时间戳 | ISO 日期字符串 |
| checkbox | `true`/`false` | `true`/`false` |
| user | `[{"userId":"xxx"}]` | `[{"corpId":"xxx","userId":"xxx"}]` |
| attachment | `[{"fileToken":"ft_xxx"}]` ⚠️需先走 attachment upload 3步流程 | `[{"url":"...","filename":"...","size":N}]` |
| url | `{"text":"显示文本","link":"https://..."}` | 同写入 |
| richText | `{"markdown":"**加粗**"}` | `{"markdown":"..."}` |
| group | `[{"cid":"xxx"}]` (注意: key 是 cid，不是 openConversationId) | 同写入 |

- 详见 [field-rules.md](../field-rules.md) 和 [error-codes.md](../error-codes.md)

## 相关产品

- [doc](./doc.md) — 富文本文档编辑，不是结构化数据表格
