# filters & sort — 筛选排序语法参考

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
| `from_now` | 从现在起 N 天内 | `["fieldId", "天数"]` |
| `date_between` | 日期区间 | `["fieldId", "[startTs, endTs]"]` |

> **操作符拼写必须严格匹配上表**，CLI 会在调用前校验，错误拼写会被拒绝。

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

---

## view update --config 中的 filter / sort 格式

> **重要区分**：`record query --filters` 和 `view update --config` 中的 filter **格式不同**！

| 场景 | filter 格式 | 说明 |
|------|-------------|------|
| `record query --filters` | **对象**：`{"operator":"and","operands":[...]}` | 直接传最外层逻辑对象 |
| `view update --config` 的 filter | **数组**：`[{"operator":"and","operands":[...]}]` | 外面多一层数组包裹 |
| `view update --config` 的 sort | **数组**：`[{"fieldId":"X","direction":"asc"}]` | 与 record query --sort 一致 |

### 正确示例

```bash
# view update 设置筛选（filter 是数组）
dws aitable view update --base-id X --table-id Y --view-id Z \
  --config '{"filter":[{"operator":"and","operands":[{"operator":"eq","operands":["fldStatus","待处理"]}]}]}'

# view update 设置排序（sort 是数组）
dws aitable view update --base-id X --table-id Y --view-id Z \
  --config '{"sort":[{"fieldId":"fldPriority","direction":"desc"}]}'

# 同时设置 filter + sort + visibleFieldIds
dws aitable view update --base-id X --table-id Y --view-id Z \
  --config '{"filter":[{"operator":"and","operands":[{"operator":"eq","operands":["fldStatus","进行中"]}]}],"sort":[{"fieldId":"fldDate","direction":"asc"}],"visibleFieldIds":["fld1","fld2","fld3"]}'
```

### CLI 自动容错

CLI 会自动修正以下常见错误格式（不会报错，但建议直接使用正确格式）：

| 错误写法 | CLI 自动修正为 |
|----------|---------------|
| `"filter":{"operator":"and",...}` （对象） | `"filter":[{"operator":"and",...}]` （数组） |
| `"sort":{"fieldId":"X","direction":"asc"}` （对象） | `"sort":[{"fieldId":"X","direction":"asc"}]` （数组） |
| 子条件用 MCP 简写 `{"fieldId":"X","operator":"eq","value":"Y"}` | 自动转为 `{"operator":"eq","operands":["X","Y"]}` |
