# 记录筛选与排序语法

仅在记录查询或按条件批改明确涉及 AND/OR、日期或比较操作符时读取。本文件只负责 `record query --filters` / `--sort` 的记录级语法；视图的 filter/sort/group 由根 Skill 直接路由到视图布局 reference。选中本文后不要再加载视图 reference。

## filters 结构规范

### 强制规则

1. **根节点必须是逻辑操作符**：`"operator"` 必须是 `"and"` 或 `"or"`，不能是 `"eq"` 等比较操作符
2. 比较操作必须放在根节点的 `"operands"` 数组内的对象中
3. `singleSelect` 和 `multipleSelect` 字段，推荐使用 **选项的 exact String 名称 (name)** 作为比较值
4. fieldId 必须通过 `field get` 获取，不能直接用字段名称

### 精简防呆模板

CLI 同时兼容两种子条件写法（推荐格式 A）：

**格式 A（operands 数组，推荐）：**
```json
{
  "operator": "and",
  "operands": [
    {"operator": "eq", "operands": ["fld_state", "进行中"]}
  ]
}
```

**格式 B（fieldId/value 对象，CLI 自动转换）：**
```json
{
  "operator": "and",
  "operands": [
    {"fieldId": "fld_state", "operator": "eq", "value": "进行中"}
  ]
}
```

4 种衍生：
- **OR 查询**：根节点 `"operator"` 改为 `"or"`
- **多条件 AND**：在 `"operands"` 数组中增加对象
- **文本包含**：内层 `"operator"` 改为 `"contain"`
- **为空判断**：`"operator":"un_exist"`，operands 只需 `["fieldId"]`

### 支持的操作符（已验证完整列表）

| 操作符 | 含义 | operands 格式 |
|--------|------|--------------|
| `eq` / `ne` | 等于 / 不等于 | `["fieldId", "value"]` |
| `contain` / `exclusive` | 包含 / 不包含（文本模糊） | `["fieldId", "value"]` |
| `gt` / `gte` / `lt` / `lte` | 大于 / ≥ / 小于 / ≤ | `["fieldId", "numStr"]` |
| `exist` / `un_exist` | 有值 / 为空 | `["fieldId"]`（无需第二项） |
| `any_of` / `none_of` / `all_of` | 包含任一 / 不包含任一 / 全包含（多选字段） | `["fieldId", "optionName"]` |
| `date_eq` / `before` / `after` | 日期等于 / 早于 / 晚于 | `["fieldId", "dateStr"]` |
| `not_before` / `not_after` | 不早于（≥） / 不晚于（≤） | `["fieldId", "2026-05-22"]` |

> **操作符拼写必须严格匹配上表**，CLI 会在调用前校验，错误拼写会被拒绝。
>
> **没有 `date_between`（区间）操作符**，也**不支持 `from_now`**——date 字段不支持区间/相对过滤，传了会被 CLI 拒绝。范围查询用 `not_before` + `not_after` 组合，见下方专节。

### 日期字段过滤（date / 创建时间 / 修改时间）

日期类字段的过滤规则与其它字段**不同**，是线上反馈最高频的踩坑点。**经集成测试实测**确认的规则：

1. **只能用日期专用操作符**：`date_eq` / `before` / `after` / `not_before` / `not_after` / `exist` / `un_exist`（与前端筛选 UI 的「等于 / 早于 / 晚于 / 早于或等于 / 晚于或等于 / 不为空 / 为空」一一对应）。
2. **比较值用日期字符串**，如 `"2026-05-22"`（也接受 RFC3339 / 毫秒时间戳，内部统一转成毫秒比较）。读取返回的是带时区 RFC3339（如 `"2026-05-22T00:00:00+08:00"`）。
3. **通用操作符 `eq` / `ne` / `gt` / `gte` / `lt` / `lte` / `contain` 对 date 字段无效**——无论传 ISO 字符串还是毫秒时间戳，都会**静默返回 0 条**。这是后端 date 字段的比较规则，不是 bug，CLI 也无法在本地拦截（不知道字段类型），务必用对操作符。
4. **没有区间操作符 `date_between`**，也**不支持 `from_now`（相对天数）**——均会静默返回 0 条，CLI 已直接拒绝。范围查询用 `not_before`（≥起点）+ `not_after`（≤终点）两个条件 `and` 组合。

| 需求 | 操作符 | 示例 operands |
|------|--------|--------------|
| 等于某天 | `date_eq` | `["fldDate", "2026-05-22"]` |
| 早于 / 晚于（不含当天） | `before` / `after` | `["fldDate", "2026-05-22"]` |
| 不早于（≥） / 不晚于（≤） | `not_before` / `not_after` | `["fldDate", "2026-05-22"]` |
| 有值 / 为空 | `exist` / `un_exist` | `["fldDate"]` |

**日期区间查询（替代 between）**——查 `2026-05-01 ~ 2026-05-31`（含端点）：

```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"not_before","operands":["fldDate","2026-05-01"]},{"operator":"not_after","operands":["fldDate","2026-05-31"]}]}'
```

### 常见错误拼写（CLI 会自动提示纠正）

| 错误写法 | 正确写法 | 说明 |
|------------|-----------|------|
| `equal` / `equals` / `is` / `==` | `eq` | 等于 |
| `neq` / `not_eq` / `not_equal` / `not_equals` / `is_not` / `!=` | `ne` | 不等于；CLI 会提示使用规范操作符 `ne` |
| `like` / `contains` / `include` | `contain` | 文本包含 |
| `greater_than` | `gt` | 大于 |
| `less_than` | `lt` | 小于 |
| `not_contain` / `is_empty` | `exclusive` / `un_exist` | 其他易混淆 |

### 错误示例

❌ **缺失根节点 and/or**（API 将忽略该 filter，返回全表）：
```json
{"operator":"eq","operands":["fldXXX","本科"]}
```

❌ **传入选项 ID 而非名称**（可能导致匹配不到 0 记录）：
```json
{"operator":"and","operands":[{"operator":"eq","operands":["fldXXX","CXzrOHK9JI"]}]}
```

### 完整示例

单条件：
```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldStatusId","进行中"]}]}'
```

多条件 AND：
```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldStatusId","进行中"]},{"operator":"gt","operands":["fldStockId","0"]}]}'
```

分页时只透传服务端真实 `nextCursor`。出现 `pagination_cursor_cycle`、相同 cursor 或重复页时，保留已取记录并立即停止；不要用相同参数重跑，也不要切换到其他查询命令把不完整结果误报为完整。

若写入已返回真实 recordId，但同一 Base/Table 的无筛选查询也无法读到该记录，问题已不属于 filters/sort 语法。保留写入返回和查询证据并停止；不要更换操作符、加载其他 AITable Reference 或 Help、重建表，也不要重放写入。

## sort 结构规范

`--sort` 传 JSON 数组，排序方向字段**必须是 `direction`**，不要使用 `order`。

```bash
--sort '[{"fieldId":"fldXXX","direction":"desc"}]'
```

多字段排序：
```bash
--sort '[{"fieldId":"fldPriority","direction":"desc"},{"fieldId":"fldCreatedAt","direction":"asc"}]'
```
