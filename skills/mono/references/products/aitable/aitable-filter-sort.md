# filters & sort — 筛选排序语法参考

> `record query --filters` 与持久化 View filter 是两套日期值协议：前者使用日期字符串/毫秒数，后者使用保留 JSON 类型的 relative/exact Scheme。不要混用。

## filters 结构规范

### 强制规则

1. **根节点必须是逻辑操作符**：`"operator"` 必须是 `"and"` 或 `"or"`，不能是 `"eq"` 等比较操作符
2. 比较操作必须放在根节点的 `"operands"` 数组内的对象中
3. `singleSelect` 和 `multipleSelect` 字段，推荐使用 **选项的 exact String 名称 (name)** 作为比较值
4. fieldId 必须通过 `table get` 或 `field get` 获取，不能直接用字段名称

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
| `not_before` / `not_after` | 不早于 / 不晚于 | `["fieldId", "dateStr"]` |

> **操作符拼写必须严格匹配上表**，CLI 会在调用前校验，错误拼写会被拒绝。
>
> `record query` 不支持 `from_now` / `date_between`。日期范围使用 `not_before` + `not_after`；相对日期请先计算绝对日期。View 的 relative/exact 对象不能传给本命令。

### record query 日期字段过滤

日期类字段只能使用 `date_eq` / `before` / `after` / `not_before` / `not_after` / `exist` / `un_exist`，比较值使用日期字符串、RFC3339 或毫秒时间戳。通用 `eq/gte/lte/contain` 对日期字段可能静默返回 0 条。

```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"not_before","operands":["fldDate","2026-05-01"]},{"operator":"not_after","operands":["fldDate","2026-05-31"]}]}'
```

### View 日期 Scheme（仅 `view update filter`）

以下结构已经过真实写入、读回和 UI 验证。最外层是数组，内部保留显式 `and/or` 根节点。

| UI 语义 | operator / value | JSON 类型要求 |
|---|---|---|
| 今天/本周/本月/今年及前后周期 | `date_eq` + `{"type":"relative","period":"day|week|month|year","offset":N}` | `offset` 必须是 JSON number 整数 |
| 过去/未来 X 天 | `from_now` + `{"type":"relative","period":"day","offset":"N"}` | `offset` 必须是 JSON string；过去为负、未来为正 |
| 指定日期 | `date_eq` + `{"type":"exact","timestamp":TIMESTAMP_MS}` | `timestamp` 必须是目标时区当天 00:00 的 Unix 毫秒 JSON number 整数 |

```bash
# 本月
dws aitable view update filter --base-id X --table-id Y --view-id Z \
  --json '[{"operator":"and","operands":[{"operator":"date_eq","operands":["fldDate",{"type":"relative","period":"month","offset":0}]}]}]'

# 过去 30 天；offset 是字符串
dws aitable view update filter --base-id X --table-id Y --view-id Z \
  --json '[{"operator":"and","operands":[{"operator":"from_now","operands":["fldDate",{"type":"relative","period":"day","offset":"-30"}]}]}]'

# 指定日期；timestamp 是毫秒数字
dws aitable view update filter --base-id X --table-id Y --view-id Z \
  --json '[{"operator":"and","operands":[{"operator":"date_eq","operands":["fldDate",{"type":"exact","timestamp":1786896000000}]}]}]'
```

写入后必须执行 `view get filter`，核对 operator、period、offset/timestamp 的值和 JSON 类型。`[object Object]`、`Invalid Date` 或类型被字符串化均不能判为成功。明确日期范围不得从等值 Scheme 猜测。

### 常见错误拼写（CLI 会自动提示纠正）

| 错误写法 | 正确写法 | 说明 |
|------------|-----------|------|
| `equal` / `equals` / `is` / `==` | `eq` | 等于 |
| `not_equal` / `not_equals` / `is_not` / `!=` | `ne` | 不等于 |
| `like` / `contains` / `include` | `contain` | 文本包含 |
| `greater_than` | `gt` | 大于 |
| `less_than` | `lt` | 小于 |
| `not_eq` / `not_contain` / `is_empty` | `ne` / `exclusive` / `un_exist` | 其他易混淆 |

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

## sort 结构规范

`--sort` 传 JSON 数组，排序方向字段**必须是 `direction`**，不要使用 `order`。

```bash
--sort '[{"fieldId":"fldXXX","direction":"desc"}]'
```

多字段排序：
```bash
--sort '[{"fieldId":"fldPriority","direction":"desc"},{"fieldId":"fldCreatedAt","direction":"asc"}]'
```
