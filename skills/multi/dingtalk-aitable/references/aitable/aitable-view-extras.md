# 视图扩展操作（lock / frozen-cols / row-height / fill-color-rule / duplicate）

本文档讲 5 项视图操作命令：

- 锁定 / 解锁视图：`view lock` / `view get lock`
- 冻结列：`view update frozen-cols` / `view get frozen-cols`
- 行高：`view update row-height` / `view get row-height`
- 数据高亮规则（条件填色）：`view update fill-color-rule` / `view get fill-color-rule`
- 复制视图：`view duplicate`

> **与 [aitable-view-config.md](./aitable-view-config.md) 的分工**：
> - `aitable-view-config.md` 讲 `view get/update <attr>` 中 8 个属性：filter / sort / group / visible-fields / field-widths / aggregate / card / timebar。
> - 本文档讲上面 5 项额外能力（包括 attr 形式的 frozen-cols / row-height / fill-color-rule，以及顶层独立的 lock / duplicate）。
> 这 5 项**不能**通过 `view update --config '{...}'` 写入，必须用各自专属子命令。

## 命令矩阵

| 子命令 | 用途 | 必填参数 | 适用 viewType |
|---|---|---|---|
| `view lock [--off]` | 锁定（默认）/ 解锁视图 | `--base-id --table-id --view-id` | 全部 |
| `view get lock` | 读取锁定状态 | `--base-id --table-id --view-id` | 全部 |
| `view update frozen-cols --count N` | 冻结左侧 N 列（0 取消） | `--base-id --table-id --view-id --count` | Grid |
| `view get frozen-cols` | 读取冻结列数 | `--base-id --table-id --view-id` | Grid |
| `view update row-height --cell-height N` | 设置单元格高度（像素） | `--base-id --table-id --view-id --cell-height` | Grid |
| `view get row-height` | 读取单元格高度 | `--base-id --table-id --view-id` | Grid |
| `view update fill-color-rule --json '[...]'` | 全量覆盖条件填色规则 | `--base-id --table-id --view-id --json` | Grid |
| `view get fill-color-rule` | 读取条件填色规则 | `--base-id --table-id --view-id` | 全部（其他视图返回 `[]`） |
| `view duplicate [--new-name X]` | 复制视图 | `--base-id --table-id --view-id` | 全部 |

## 视图锁定 / 解锁

```bash
# 锁定（默认）
dws aitable view lock --view-id VIEW_ID

# 解锁
dws aitable view lock --view-id VIEW_ID --off

# 查询当前是否锁定
dws aitable view get lock --view-id VIEW_ID --format json
# → {"data": {"baseId": ..., "tableId": ..., "viewId": ..., "locked": true|false}}
```

锁定的视图禁止他人修改其配置（filter/sort/group/字段顺序等），但记录读写不受影响。锁定状态可重复 set，幂等。

## 冻结列（仅 Grid）

```bash
# 冻结从首列起 1 列
dws aitable view update frozen-cols --view-id VIEW_ID --count 1

# 取消冻结
dws aitable view update frozen-cols --view-id VIEW_ID --count 0

# 查询当前冻结列数
dws aitable view get frozen-cols --view-id VIEW_ID --format json
# → {"data": {..., "count": 1}}    未显式设置时整个 count 键缺失（不是返回 null）
```

`--count` 必须 ≥ 0；负数会被拒绝。

## 行高（仅 Grid）

⚠️ **`--cell-height` 只接受 4 档枚举：32 / 56 / 88 / 128**（与前端 CELL_HEIGHTS 约定一致），其他值会被拒绝。默认值为 32。

```bash
# 设置行高 — 推荐档位 32 / 56 / 88 / 128
dws aitable view update row-height --view-id VIEW_ID --cell-height 56

# 查询当前行高
dws aitable view get row-height --view-id VIEW_ID --format json
# → {"data": {..., "cellHeight": 56}}    未显式设置时整个 cellHeight 键缺失（不是返回 null；前端按 32 渲染）
```

## 数据高亮规则（条件填色，仅 Grid）

`view update fill-color-rule` **整组覆盖**，传 `--json '[]'` 清空所有规则。

### 规则结构

每条规则 JSON 结构：

```jsonc
{
  "type": "cell" | "row" | "column" | "preRow",
  "formatFieldId": "fldX",                     // 命中规则后被高亮的字段（cell/column 类型有意义）
  "format": { "color": "firstLine5" },         // ⚠️ 必须用 FORMAT_COLORS 代号，不接受 hex
  "filters": [                                 // 当前固定 1 条
    {
      "fieldId": "fldX",                       // ⚠️ 不是 operands[0]
      "symbol": "GT",                          // ⚠️ 不是 operator；大写枚举
      "value": 100                             // 部分 symbol（EXIST/UN_EXIST）不需要 value
    }
  ]
}
```

### color 合法值（FORMAT_COLORS）

`firstLine1` ～ `firstLine11`（共 11 档色码，对应前端调色盘）。**不接受 `#FF0000` 这种 hex**。

### filter.symbol 合法值

| 类别 | symbol |
|---|---|
| 数值/通用比较 | `GT` / `LT` / `GTE` / `LTE` / `EQ` / `NE` |
| 文本 | `CONTAIN` / `EXCLUSIVE` |
| 存在性（无 value） | `EXIST` / `UN_EXIST` |
| 多选 / 集合 | `ALL_OF` / `ANY_OF` / `NONE_OF` |
| 日期 | `BEFORE` / `AFTER` / `NOT_BEFORE` / `NOT_AFTER` / `DATE_EQ` / `FROM_NOW` / `DATE_BETWEEN` |

> **与 `record query --filters` / `view update filter` 的格式不同**：那两处用 `{operator, operands}` 结构；这里是 `{fieldId, symbol, value}`。不要混用。

### 典型用法

```bash
# 1) 给金额字段 > 100 的单元格上 firstLine5 色
dws aitable view update fill-color-rule --view-id GRID_ID --json '[
  {
    "type":"cell",
    "formatFieldId":"fldAmount",
    "format":{"color":"firstLine5"},
    "filters":[{"fieldId":"fldAmount","symbol":"GT","value":100}]
  }
]'

# 2) 清空所有规则
dws aitable view update fill-color-rule --view-id GRID_ID --json '[]'

# 3) 查询当前规则
dws aitable view get fill-color-rule --view-id GRID_ID --format json
# → {"data": [...]} 数组
```

> **写入后请用 `view get fill-color-rule` 二次确认实际生效**，以读到的 `data` 数组为准。

## 复制视图

```bash
# 显式命名
dws aitable view duplicate --view-id VIEW_ID --new-name "副本视图"

# 系统自动命名（一般是 "原视图名 (副本)"）
dws aitable view duplicate --view-id VIEW_ID --format json
# → {"data": {..., "viewId": "<新视图ID>", "sourceViewId": "<原视图ID>", "viewName": "..."}}
```

复制会保留源视图的 filter / sort / group / visible-fields / card / timebar 等全部配置；新视图的 viewId 与源视图独立。

## 这些字段不能用 `view update --config '{...}'` 写

下列字段必须用对应的专属子命令；如果错塞进 `view update --config`，CLI 会在 stderr 提示对应子命令并拒绝把字段当 view config 处理：

| 错误用法 | 应改用 |
|---|---|
| `--config '{"flags":1}'` | `view lock` / `view lock --off` |
| `--config '{"frozenColCount":2}'` | `view update frozen-cols --count N` |
| `--config '{"cellHeight":56}'` | `view update row-height --cell-height N` |
| `--config '{"rowHeightLevel":"tall"}'` | `view update row-height --cell-height N`（合法档位 32/56/88/128） |
| `--config '{"conditionalFormats":[...]}'` | `view update fill-color-rule --json '[...]'` |

## 典型工作流

### 配置一个"金额超阈值红色高亮"的 Grid 视图

```bash
BASE=baseXXX; TABLE=tblYYY; VIEW=viwGridZZ; FLD=fldAmount

# 1) 关键字段冻结，避免横向滚动看不到
dws aitable view update frozen-cols --base-id $BASE --table-id $TABLE --view-id $VIEW --count 1

# 2) 加大行高让数据更易读
dws aitable view update row-height --base-id $BASE --table-id $TABLE --view-id $VIEW --cell-height 56

# 3) 金额 > 100 的单元格上色
dws aitable view update fill-color-rule --base-id $BASE --table-id $TABLE --view-id $VIEW --json "[
  {\"type\":\"cell\",\"formatFieldId\":\"$FLD\",\"format\":{\"color\":\"firstLine5\"},
   \"filters\":[{\"fieldId\":\"$FLD\",\"symbol\":\"GT\",\"value\":100}]}
]"

# 4) 锁定视图，防止他人改坏
dws aitable view lock --base-id $BASE --table-id $TABLE --view-id $VIEW
```

### 复制一个"金牌客户"视图给销售团队

```bash
dws aitable view duplicate --view-id viw_VIP_template --new-name "金牌客户-华东区"
# 取返回里 data.viewId 进一步定制
```

### 排查"我设置了高亮规则为啥没生效"

```bash
# 看实际生效的 conditionalFormats
dws aitable view get fill-color-rule --view-id VIEW_ID --format json
# → 如果是 [] 说明上次写入失败；常见原因：color 用了 hex（必须 firstLineN）/ filter 用了 operator（必须 symbol）
```
