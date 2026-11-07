# 视图类型专属配置（card / timebar / aggregate）

仅在任务明确涉及 Kanban/Gallery 卡片、Gantt 时间轴或 Grid 聚合时读取。本文件自包含这三类属性的命令与工作流；普通列顺序、筛选、排序、分组和列宽由根 Skill 直接路由到通用视图配置 reference，不要连读两个视图 reference。

所有命令都必须同时传 `--base-id`、`--table-id`、`--view-id`。`view get/update card` 会先读取一次 viewType，再按 Kanban/Gallery 分发；timebar 仅 Gantt，aggregate 仅 Grid。

## 支持矩阵

| viewType | card | timebar | aggregate |
|---|:---:|:---:|:---:|
| Grid |  |  | ✅ |
| Kanban | ✅ |  |  |
| Gallery | ✅ |  |  |
| Gantt |  | ✅ |  |
| Calendar |  |  |  |

## 读取专属属性

```bash
dws aitable view get card --base-id <B> --table-id <T> --view-id <V> --format json
dws aitable view get timebar --base-id <B> --table-id <T> --view-id <V> --format json
dws aitable view get aggregate --base-id <B> --table-id <T> --view-id <V> --format json
```

## card（Kanban / Gallery）

服务端按 viewType 分发到 `kanbanCard` 或 `galleryCard`。typed flag 与 `--json` 同时存在时 typed flag 优先；`--no-cover` 与 `--cover-field-id` 互斥。

| flag | 适用范围 | 说明 |
|---|---|---|
| `--cover-field-id` | Kanban / Gallery | 封面字段 ID |
| `--no-cover` | Kanban / Gallery | 清除封面 |
| `--cover-resize-mode` | Kanban / Gallery | `cover` / `contain` / `stretch` |
| `--hidden-field-title` | Kanban | 隐藏字段名标题 |
| `--cover-mode` | Gallery | `none` / `auto` / `custom` |
| `--display-field-name` | Gallery | 是否显示字段名 |
| `--json` | Kanban / Gallery | 完整 card 子块对象 |

```bash
dws aitable view update card --base-id <B> --table-id <T> --view-id <KANBAN_V> \
  --cover-field-id <ATTACHMENT_FIELD_ID> --cover-resize-mode contain
dws aitable view update card --base-id <B> --table-id <T> --view-id <KANBAN_V> --no-cover
dws aitable view update card --base-id <B> --table-id <T> --view-id <GALLERY_V> --cover-mode auto
dws aitable view update card --base-id <B> --table-id <T> --view-id <GALLERY_V> \
  --json '{"coverMode":"custom","coverFieldId":"fldX","displayFieldName":true}'
```

排查 Kanban 封面时先读取 card：`coverFieldId` 为 `NONE` 或缺失表示未配置封面；否则再检查 `coverResizeMode`。

## timebar（仅 Gantt）

`view create --view-type Gantt` 只创建空壳，必须紧跟 `view update timebar --start-field <日期字段ID>`；`view create --config` 不接受 `ganttTimebar`。

| flag | 说明 |
|---|---|
| `--start-field` | 开始日期 fieldId |
| `--end-field` | 结束日期 fieldId |
| `--display-field-id` | 时间条标题 fieldId |
| `--timeline-scale` | `year` / `quarter` / `month` / `weeks` |
| `--color-configs` | 颜色配置 JSON 数组；清空传 `[]` |
| `--official-holiday` | 是否标注法定节假日 |
| `--json` | 完整 ganttTimebar 子块 |

```bash
# 第一步：创建并取得真实 viewId
dws aitable view create --base-id <B> --table-id <T> --view-type Gantt --name "项目甘特图" --format json

# 第二步：绑定日期字段，否则视图为空
dws aitable view update timebar --base-id <B> --table-id <T> --view-id <GANTT_V> \
  --start-field <START_DATE_FIELD_ID> --end-field <END_DATE_FIELD_ID> --timeline-scale month

# 后续只改时间尺度和节假日
dws aitable view update timebar --base-id <B> --table-id <T> --view-id <GANTT_V> \
  --timeline-scale quarter --official-holiday=true
```

## aggregate（仅 Grid）

值是 `map[fieldId]→AggregateAction`；传 `null` 清除某字段聚合。常用 action 包括 `SUM`、`AVG`、`MAX`、`MIN`、`MEDIAN`、`RANGE`、`TOTAL`、`DISTINCT`、`EXIST`、`UN_EXIST`、`CHECKED`、`EARLIEST_DATE`，实际可用项受字段类型限制。

```bash
dws aitable view update aggregate --base-id <B> --table-id <T> --view-id <GRID_V> \
  --field-id <FIELD_ID> --action SUM
dws aitable view update aggregate --base-id <B> --table-id <T> --view-id <GRID_V> \
  --clear-field-id <FIELD_ID_1,FIELD_ID_2>
dws aitable view update aggregate --base-id <B> --table-id <T> --view-id <GRID_V> \
  --json '{"fldAmount":"AVG","fldObsolete":null}'
```

## 写后验证与停止条件

- 每次 update 后只用同一属性的 `view get card|timebar|aggregate` 回读，不加载其他 reference。
- viewType 不匹配时停止并保留真实错误，不更换 viewId 猜测。
- Gantt 未取得真实日期 fieldId 时停止，不用字段名代替。
- typed flag 与 `--json` 冲突时以 typed flag 为准；不要重复提交同一写操作。
