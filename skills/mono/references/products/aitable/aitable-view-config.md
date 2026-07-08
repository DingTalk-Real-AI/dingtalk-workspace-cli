# 视图配置（view get/update <attr>）

按属性局部读/写视图配置。每个属性独立子命令，typed flag 友好，agent 不必拼 JSON。
向后兼容：`view update --config '{...}'` 一次多属性入口仍可用。

## viewType × 支持矩阵

| viewType | card | timebar | aggregate | filter / sort / group | visible-fields | field-widths | name |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Grid     |     |     | ✅ |  ✅ | ✅ | ✅ | ✅ |
| Kanban   | ✅ |     |     |  ✅ | ✅ |    | ✅ |
| Gallery  | ✅ |     |     |  ✅ | ✅ |    | ✅ |
| Gantt    |     | ✅ |     |  ✅ | ✅ |    | ✅ |
| Calendar |     |     |     |  ✅ | ✅ |    | ✅ |
| FormDesigner | （走 `form` 系列命令） | | | | | | |

> card 在 Kanban 走 `kanbanCard`，在 Gallery 走 `galleryCard`，CLI 自动按 viewType dispatch（preflight 1 次 `get_views`）。timebar 仅 Gantt 支持；Calendar 服务端未暴露任何 timebar 配置。

> **Gantt 视图必须两步创建**：`view create --view-type Gantt` 只创建空壳（`ganttTimebar: {}`），**必须**紧跟 `view update timebar --start-field <日期字段ID>` 绑定时间轴字段，否则视图打开是空白。`create_view` 的 `--config` 中传入 `ganttTimebar` 会被服务端忽略。

## 读取：view get <attr>

所有 `view get <attr>` 共用 `--base-id` / `--table-id` / `--view-id`，输出是该属性子块的 JSON（不存在时输出 `{}`）。viewType 不匹配会报错并指明应该选哪种视图。

```bash
dws aitable view get card --view-id VIEW_ID --format json            # Kanban / Gallery
dws aitable view get timebar --view-id VIEW_ID --format json         # Gantt
dws aitable view get aggregate --view-id VIEW_ID --format json       # Grid
dws aitable view get filter --view-id VIEW_ID --format json          # 所有
dws aitable view get sort --view-id VIEW_ID --format json
dws aitable view get group --view-id VIEW_ID --format json
dws aitable view get visible-fields --view-id VIEW_ID --format json
dws aitable view get field-widths --view-id VIEW_ID --format json    # Grid
```

## 写入：view update <attr>

所有 `view update <attr>` 共用 `--base-id` / `--table-id` / `--view-id`。
**typed flag + `--json` 可混用**；冲突时 typed flag 优先并 stderr 提示。
card / timebar / aggregate 三类写入有 viewType 校验（preflight 1 次 get_views）。

### view update card（Kanban / Gallery）

服务端按 viewType 分发到 `kanbanCard` 或 `galleryCard`。typed flag 共享。

| flag | 类型 | 说明 |
|------|------|------|
| `--cover-field-id` | string | 封面字段 ID（Kanban / Gallery 通用），与 `--no-cover` 互斥 |
| `--no-cover` | bool | 清除封面（等价 `coverFieldId="NONE"`） |
| `--cover-resize-mode` | string | `cover` / `contain` / `stretch` |
| `--hidden-field-title` | bool | 隐藏字段名标题（仅 Kanban 生效） |
| `--cover-mode` | string | `none` / `auto` / `custom`（仅 Gallery 生效） |
| `--display-field-name` | bool | 是否显示字段名（仅 Gallery 生效） |
| `--json` | JSON | 完整 card 子块对象 |

```bash
dws aitable view update card --view-id KANBAN_ID --cover-field-id fldAttachment --cover-resize-mode contain
dws aitable view update card --view-id KANBAN_ID --no-cover
dws aitable view update card --view-id GALLERY_ID --cover-mode auto
dws aitable view update card --view-id GALLERY_ID --json '{"coverMode":"custom","coverFieldId":"fldX","displayFieldName":true}'
```

### view update timebar（仅 Gantt）

| flag | 类型 | 说明 |
|------|------|------|
| `--start-field` | string (date fieldId) | 开始日期字段 |
| `--end-field` | string (date fieldId) | 结束日期字段 |
| `--display-field-id` | string | 时间条上显示的标题字段 |
| `--timeline-scale` | string | `year` / `quarter` / `month` / `weeks` |
| `--color-configs` | JSON 数组 | 颜色配置数组（结构由下游协议定义；清空传 `[]`） |
| `--official-holiday` | bool | 是否标注法定节假日 |
| `--json` | JSON | 完整 ganttTimebar 子块 |

```bash
dws aitable view update timebar --view-id GANTT_ID --start-field fldStart --end-field fldEnd --timeline-scale month
dws aitable view update timebar --view-id GANTT_ID --official-holiday=true
```

### view update aggregate（仅 Grid）

值是 `map[fieldId]→AggregateAction string`。**设置**聚合可用；**清除**聚合当前无效（见下方警告）。

| flag | 类型 | 说明 |
|------|------|------|
| `--field-id` | string | 配合 `--action` 设置**单字段**聚合 |
| `--action` | string | `SUM`/`AVG`/`MAX`/`MIN`/`MEDIAN`/`RANGE`/`TOTAL`/`DISTINCT`/`EXIST`/`UN_EXIST`/`CHECKED`/`EARLIEST_DATE` 等（按字段类型可用） |
| `--clear-field-id` | string (CSV) | 一/多个字段 ID，本意是清除其聚合，但**当前服务端不支持清除，静默无效**（见下方警告） |
| `--json` | JSON | 完整 aggregate map |

```bash
dws aitable view update aggregate --view-id GRID_ID --field-id fldX --action SUM
```

> ⚠️ **当前无法清除已设置的聚合（服务端限制）**：`--clear-field-id fldA,fldB` 与 `--json '{"fldX":null}'` 两种清除写法都返回 `success`，但用 `view get aggregate` 复核会发现聚合**原样不动**——是静默无效，不是真的清掉。这是服务端没有清除语义所致，直至服务端修复前不要依赖它。改聚合方式可行（重新 `--action` 覆盖成别的），只是无法回到"无聚合"。

### view update field-widths（仅 Grid）

| flag | 类型 |
|------|------|
| `--field-id` + `--width` | string + int（单字段） |
| `--json` | `{fldId: width, ...}` |

```bash
dws aitable view update field-widths --view-id GRID_ID --field-id fldX --width 200
dws aitable view update field-widths --view-id GRID_ID --json '{"fldA":120,"fldB":200}'
```

### view update visible-fields（通用）

整组替换可见字段列表与顺序，同时兼作**隐藏/显示**入口。首列字段（primaryDoc）必须保留在数组第一位。

> **传入的列表既定顺序又定可见性**：传一个比当前 columns 短的列表，缺失的字段会被真正隐藏（该字段在 `view list` 的 `custom.hiddenFields` 里变 `true`）；再传回全量列表即可解除隐藏（`hiddenFields` 变 `false`）。
>
> ⚠️ **查隐藏状态别看这里**：`view get visible-fields` 返回的数组**包含已隐藏字段**（列的完整顺序），看不出谁被隐藏。要确认隐藏状态，读 `view get`（view list）里该视图的 `custom.hiddenFields`（`{fieldId: true|false}`）。

| flag | 类型 |
|------|------|
| `--field-ids` | string (CSV) |
| `--json` | string 数组 JSON（与 `--field-ids` 同传时 `--json` 优先） |

```bash
# 只保留首列和 fldA，其余字段被隐藏
dws aitable view update visible-fields --view-id VIEW_ID --field-ids fldPrimary,fldA
# 传回全量列表解除隐藏
dws aitable view update visible-fields --view-id VIEW_ID --json '["fldPrimary","fldA","fldB"]'
```

### view update filter / sort / group（通用，纯 --json）

```bash
dws aitable view update filter --view-id VIEW_ID --json '[{"operator":"and","operands":[{"operator":"eq","operands":["fldX","value"]}]}]'
dws aitable view update sort   --view-id VIEW_ID --json '[{"fieldId":"fldX","direction":"asc"}]'
dws aitable view update group  --view-id VIEW_ID --json '[{"fieldId":"fldX","direction":"asc"}]'
```

> filter/sort/group 入参格式与 `record query --filters`（对象格式）**不同**：view config 这边外层必须是数组。传对象 CLI 会自动 wrap，建议直接用数组。详见 [aitable-filter-sort.md](./aitable-filter-sort.md)。

### view update name（重命名）

```bash
dws aitable view update name --view-id VIEW_ID --name "新视图名"
```

等价于 `dws aitable view update --view-id VIEW_ID --name "新视图名"`，无 `config` 参数。

## 服务端字段速查（与 dws CLI 关系）

| dws 子命令 | 服务端 `update_view.config` 子键 | 服务端 Java 模型 |
|---|---|---|
| `view update card`（Kanban） | `kanbanCard` | `KanbanCardUpdateInput` |
| `view update card`（Gallery） | `galleryCard` | `GalleryCardUpdateInput` |
| `view update timebar` | `ganttTimebar` | `GanttTimebarUpdateInput` |
| `view update aggregate` | `aggregate` | `Map<fieldId, AggregateAction>` |
| `view update visible-fields` | `visibleFieldIds` | `List<String>` |
| `view update filter / sort / group` | `filter` / `sort` / `group` | `List<Object>` |
| `view update field-widths` | `fieldWidths` | `Map<String, Object>` |
| `view update name` | （不在 config 内）`newViewName` 顶层 | — |

## 典型工作流

### 排查"Kanban 卡片为啥不显示封面"

```bash
dws aitable view get card --view-id KANBAN_ID --format json
# → 看 coverFieldId 是不是 "NONE" 或缺失；不是再看 coverResizeMode 是不是 contain 导致裁掉
```

### 创建可用的 Gantt 视图（必须两步）

```bash
# 第 1 步：创建 Gantt 视图
dws aitable view create --base-id BASE_ID --table-id TABLE_ID \
  --view-type Gantt --name "项目甘特图" -f json
# → 记录返回的 viewId

# 第 2 步（必须）：绑定日期字段，否则视图为空
dws aitable view update timebar --base-id BASE_ID --table-id TABLE_ID \
  --view-id VIEW_ID --start-field fldDateStart
# 可选：加结束日期、标题字段、时间尺度
#   --end-field fldDateEnd --display-field-id fldName --timeline-scale month
```

### 把 Gantt 时间轴改成季度尺度并加节假日

```bash
dws aitable view update timebar --view-id GANTT_ID \
  --timeline-scale quarter --official-holiday=true
```

### 用 dws 脚本批量替换 Kanban 封面字段

```bash
for v in viw1 viw2 viw3; do
  dws aitable view update card --view-id $v --cover-field-id fldNewCover --cover-resize-mode cover --format json | jq .status
done
```

### 一次性多属性更新（仍走 legacy --config）

```bash
dws aitable view update --view-id VIEW_ID --config '{
  "visibleFieldIds":["fldPrimary","fldA","fldB"],
  "filter":[{"operator":"and","operands":[]}],
  "kanbanCard":{"coverFieldId":"fldImg","coverResizeMode":"contain"}
}'
```
