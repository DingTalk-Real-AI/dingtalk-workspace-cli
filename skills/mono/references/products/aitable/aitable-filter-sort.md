# filters & sort — 筛选排序语法参考

> 视图（view）配置的 filter/sort/group **整体写入**请优先用 `view update filter` / `view update sort` / `view update group` 子命令，详见 [aitable-view-config.md](./aitable-view-config.md)。本文件聚焦于 `record query --filters` 与 view config filter 的语法和差异。

## filters 结构规范

### 强制规则

1. **根节点必须是逻辑操作符**：`"operator"` 必须是 `"and"` 或 `"or"`，不能是 `"eq"` 等比较操作符
2. 比较操作必须放在根节点的 `"operands"` 数组内的对象中
3. **查询指定数据前必须先读一遍完整表头**：先用 `field get`（不加 `--field-ids`）取回所有字段的 `fieldId`/`name`/`type`/`config`，从表头中确定用户条件对应哪个字段，再按该字段类型解析值并传入查询条件；不能直接用字段名称、凭用户原话猜字段或猜类型
4. `singleSelect` 和 `multipleSelect` 必须先通过 `field get` 或 `field search-options` 唯一解析选项，filter 优先传稳定 option ID；`record create/update` 才传 option name
5. 人员、部门、群组必须先调用对应的通讯录/会话查询解析稳定 ID，再传结构化 ID 数组；禁止把姓名、部门名、群名、裸 ID 字符串直接放进 filter

### 按字段类型解析比较值

用户提供的是自然语言展示值时，必须先解析再组装 filter，禁止原值透传：

| 字段类型 | filter 比较值 | 前置解析 |
|---|---|---|
| `text` | JSON string | 无；按用户原文本使用 |
| `number` | JSON number | 将明确数值转换为数字，禁止传字符串数字 |
| `date` | `YYYY-MM-DD`、RFC3339 或毫秒时间戳 | 使用日期专用操作符；范围拆为 `not_before` + `not_after` |
| `singleSelect` / `multipleSelect` | singleSelect 通常传 option ID 标量；multipleSelect 或数组型条件传 option ID 数组，如 `["optA"]` | `field get` 或 `field search-options` 唯一匹配；写记录时才使用 option name |
| 人员 | `[{"userId":"..."}]`（需要时同时带 `corpId`） | `dws aisearch person --keyword "<姓名>" --dimension name --format json`；取唯一 `userId`，重名必须消歧 |
| 部门 | `[{"departmentId":"..."}]` | `dws contact +resolve-dept --name "<部门名>" --format json`；取唯一 `deptId` 并写入 `departmentId`，零/多命中必须停止 |
| 群组 | `[{"cid":"..."}]` | `dws chat +chat-search --query "<群名>" --page-all --format json`；取唯一 `openConversationId` 并写入 `cid`，零/多命中必须停止 |
| 关联记录 | 稳定 `recordId` 或字段协议要求的 recordId 结构 | 先查询关联目标表并唯一定位记录；不得传记录标题 |

`exist` / `un_exist` 不传第二个操作数。数组、对象、布尔值和数字必须保持 JSON 原生类型，不得统一字符串化。人员、部门、群组属于数组型字段，即使只筛一个目标也必须保留数组外层，例如 `"operands":["fldUser",[{"userId":"u1"}]]`，不能传姓名、`"u1"` 或单个对象。当 `eq` / `ne` / `any_of` / `none_of` / `all_of` 等条件涉及 multipleSelect 或其他数组型字段时，第二个操作数必须是 option ID/稳定 ID 数组，例如 `"operands":["fldMulti",["optA"]]`；不要传裸字符串 `"operands":["fldMulti","optA"]`。任何解析出现零命中或多命中时，必须停止并让用户补充或选择，禁止默认取第一项。

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
| `gt` / `gte` / `lt` / `lte` | 大于 / ≥ / 小于 / ≤ | `["fieldId", 123]`（JSON number，禁止字符串数字） |
| `exist` / `un_exist` | 有值 / 为空 | `["fieldId"]`（无需第二项） |
| `any_of` / `none_of` / `all_of` | 包含任一 / 不包含任一 / 全包含 | `singleSelect` 可传 option ID 标量；涉及 `multipleSelect` / 数组型字段时传 `["fieldId", ["optionId"]]`（第二项必须是数组） |
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

❌ **直接透传未确认的选项名称或展示文本**（可能重名、改名或无法转换）：
```json
{"operator":"and","operands":[{"operator":"eq","operands":["fldXXX","进行中"]}]}
```

### 完整示例

单条件（select 字段已先经 `field get`/`field search-options` 唯一解析为 option ID）：
```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldStatusId","optDoing"]}]}'
```

多条件 AND：
```bash
dws aitable record query --base-id X --table-id Y \
  --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldStatusId","optDoing"]},{"operator":"gt","operands":["fldStockId",0]}]}'
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
