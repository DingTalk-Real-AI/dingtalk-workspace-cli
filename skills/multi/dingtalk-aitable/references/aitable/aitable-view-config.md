# 视图布局与条件配置

仅在任务涉及 filter/sort/group、列顺序、列宽或重命名时读取。本文自包含这些通用属性；Kanban/Gallery card、Gantt timebar、Grid aggregate 以及 lock/frozen-cols/row-height/fill-color-rule/duplicate 均由根 Skill 直接路由到各自 reference，不要连读多个视图 reference。

按属性局部读/写视图配置；向后兼容的 `view update --config '{...}'` 仅用于一次写多个通用属性。

## viewType × 支持矩阵

| viewType | filter / sort / group | visible-fields | field-widths | name |
|---|:---:|:---:|:---:|:---:|
| Grid     | ✅ | ✅ | ✅ | ✅ |
| Kanban   | ✅ | ✅ |    | ✅ |
| Gallery  | ✅ | ✅ |    | ✅ |
| Gantt    | ✅ | ✅ |    | ✅ |
| Calendar | ✅ | ✅ |    | ✅ |
| FormDesigner | （走 `form` 系列命令） | | | |

## 创建：view create

`--view-type` 支持 `Grid`、`Kanban`、`Gantt`、`Calendar`、`Gallery`、`FormDesigner`。创建时通过 `--config` JSON 设置可见字段：

```bash
# --config JSON：可同时配置可见字段、筛选、排序和分组；主字段必须排第一
dws aitable view create --base-id BASE_ID --table-id TABLE_ID \
  --view-type Grid --name "任务视图" \
  --config '{"visibleFieldIds":["fldPrimary","fldStatus","fldOwner"],"sort":[{"fieldId":"fldStatus","direction":"asc"}]}'
```

创建阶段的 `--config` 是 JSON 对象，并且只接受以下 4 个 key：

| key | 类型 | 说明 |
|---|---|---|
| `visibleFieldIds` | `string[]` | fieldId 数组，不接受字段名；至少一个，主字段必须排第一 |
| `filter` | `object[]` | 筛选规则数组；兼容单个 object，CLI 会自动包装为数组 |
| `sort` | `object[]` | 排序规则数组；兼容单个 object，CLI 会自动包装为数组 |
| `group` | `object[]` | 分组规则数组；兼容单个 object，CLI 会自动包装为数组 |

描述使用独立的 `--desc '{"content":[]}'`；`description`、`fieldWidths`、`aggregate`、`kanbanCard`、`ganttTimebar`、`galleryCard` 等其他 key 会在调用服务端前被拒绝，并提示对应的 `view update` 子命令。

## 读取：view get <attr>

所有 `view get <attr>` 共用 `--base-id` / `--table-id` / `--view-id`，输出是该属性子块的 JSON（不存在时输出 `{}`）。viewType 不匹配会报错并指明应该选哪种视图。

```bash
dws aitable view get filter --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json          # 所有
dws aitable view get sort --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json
dws aitable view get group --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json
dws aitable view get visible-fields --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json
dws aitable view get field-widths --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json    # Grid
```

## 写入：view update <attr>

所有 `view update <attr>` 共用 `--base-id` / `--table-id` / `--view-id`。
**typed flag + `--json` 可混用**；冲突时 typed flag 优先并 stderr 提示。

### view update field-widths（仅 Grid）

| flag | 类型 |
|------|------|
| `--field-id` + `--width` | string + int（单字段） |
| `--json` | `{fldId: width, ...}` |

```bash
dws aitable view update field-widths --base-id BASE_ID --table-id TABLE_ID --view-id GRID_ID --field-id fldX --width 200
dws aitable view update field-widths --base-id BASE_ID --table-id TABLE_ID --view-id GRID_ID --json '{"fldA":120,"fldB":200}'
```

### view update visible-fields（通用）

整组替换可见字段列表与顺序。`field get` 返回的第一个字段是系统行索引/主字段；无论它显示为 text 还是 primaryDoc，都必须保留在数组第一位，且不能隐藏。不要仅凭字段类型猜主字段。

> ⚠️ 注意：服务端**只接受 reorder，不接受真"隐藏字段"**——如果传入的列表比当前 columns 短，缺失的字段不会被隐藏。需要真正隐藏字段请到 AI 表格 Web UI。

| flag | 类型 |
|------|------|
| `--field-ids` | string (CSV) |
| `--json` | string 数组 JSON（与 `--field-ids` 同传时 `--json` 优先） |

```bash
dws aitable view update visible-fields --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-ids fldPrimary,fldA,fldB
dws aitable view update visible-fields --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '["fldPrimary","fldA","fldB"]'
```

### 列顺序最短闭环

用户说“客户名称最左、状态在金额前”时，不要用通用 `+view-update --config` 探索：

1. `dws aitable field get --base-id <B> --table-id <T> --format json` 取字段有序列表；第一个 fieldId 固定为数组第 1 项。目标 viewId 从真实上下文或 `view get` 返回中取得。
2. `dws aitable view get visible-fields --base-id <B> --table-id <T> --view-id <V> --format json` 取当前完整列数组；必须保留全部现有字段，因为该接口只支持 reorder，不是真隐藏。
3. 只重排目标：`[主字段, 客户名称, ..., 状态, 金额, ...]`，其他字段保持相对顺序；一次执行 `view update visible-fields`。
4. 再次 `view get visible-fields`，数组完全一致才算完成。遇到 `PRIMARY_FIELD_CANNOT_BE_MOVED/HIDDEN` 立即停止，重新按步骤 1 构造一次；禁止继续猜排列。

“固定/冻结左侧列”与“放到最左边”不是同一操作。只有 Grid 支持冻结；若要冻结主字段后的目标列，需要冻结前 N 列（例如目标位于第 2 列则 count=2）：

```bash
dws aitable +view-set-frozen-cols --base-id <B> --table-id <T> --view-id <V> --count <N>
dws aitable +view-get-frozen-cols --base-id <B> --table-id <T> --view-id <V>
```

Kanban/Gallery 等视图只调整列顺序，不尝试冻结。

### view update filter / sort / group（通用，纯 --json）

```bash
dws aitable view update filter --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '[{"operator":"eq","operands":["fldX","value"]}]'
dws aitable view update sort --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '[{"fieldId":"fldX","direction":"asc"}]'
dws aitable view update group --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --json '[{"fieldId":"fldX","direction":"asc"}]'
```

日期筛选使用 View 专用结构化 Scheme，不使用 `record query` 的日期字符串写法：

```json
[{"operator":"and","operands":[
  {"operator":"date_eq","operands":["fldDate",{"type":"relative","period":"month","offset":0}]},
  {"operator":"from_now","operands":["fldDate",{"type":"relative","period":"day","offset":"-30"}]}
]}]
```

`date_eq.relative.offset` 和 `date_eq.exact.timestamp` 必须是 JSON number 整数；`from_now.offset` 必须是 JSON string。完整规则与指定日期示例见 [aitable-filter-sort.md](./aitable-filter-sort.md#view-日期-scheme仅-view-update-filter)。

人员、部门、群组筛选必须使用稳定身份；只有显示名称时，也可以显式传 `entityName`，CLI 会先调用 `search_entities`，仅在唯一精确匹配且候选分页完整时执行更新：

```bash
# 只搜索候选，不更新 View
dws aitable entity search --entity-type PERSON --keyword "张三" --format json
dws aitable entity search --entity-type DEPARTMENT --keyword "客户成功部" --format json
dws aitable entity search --entity-type GROUP --keyword "项目群" --format json

# 已知稳定身份直接写；只有名称时让 CLI 先解析
dws aitable view update filter --view-id VIEW_ID --json '[{"operator":"eq","operands":["fldOwner",{"userId":"staff1","corpId":"dingCorp"}]}]'
dws aitable view update filter --view-id VIEW_ID --json '[{"operator":"eq","operands":["fldDept",{"entityName":"客户成功部"}]}]'
```

稳定身份分别是：人员 `{"userId":"...","corpId":"..."}` 或 `{"userRef":"..."}`，部门 `{"departmentId":"..."}`，群组 `{"cid":"..."}` 或 `{"openConversationId":"..."}`（二选一）。DWS 原样传递群组标识，MCP 负责将 `openConversationId` 转成持久化的 `cid`；不要传裸名称字符串，也不要同时传两种群组标识。零命中、重名、模糊命中或候选分页不完整时停止，不执行 `update_view`。

人员身份写入后，旧服务可能只回读内部人员 key，CLI 无法把它与外部稳定身份安全比较。此时返回 `reason=view_filter_verification_unknown`、`execution_started=true`、`details.status=unknown` 和 `details.verified=false`；这表示更新已经提交但未证实最终状态，禁止自动重放，应先读取当前 View 再决定后续动作。

> filter/sort/group 入参格式与 `record query --filters`（对象格式）**不同**：view config 这边外层必须是数组。传对象 CLI 会自动 wrap，建议直接用数组。详见 [aitable-filter-sort.md](./aitable-filter-sort.md)。

### view update name（重命名）

```bash
dws aitable view update name --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --name "新视图名"
```

等价于 `dws aitable view update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --name "新视图名"`，无 `config` 参数。

## 服务端字段速查（与 dws CLI 关系）

| dws 子命令 | 服务端 `update_view.config` 子键 | 服务端 Java 模型 |
|---|---|---|
| `view update visible-fields` | `visibleFieldIds` | `List<String>` |
| `view update filter / sort / group` | `filter` / `sort` / `group` | `List<Object>` |
| `view update field-widths` | `fieldWidths` | `Map<String, Object>` |
| `view update name` | （不在 config 内）`newViewName` 顶层 | — |

## 典型工作流

### 一次性多属性更新（仍走 legacy --config）

```bash
dws aitable view update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --config '{
  "visibleFieldIds":["fldPrimary","fldA","fldB"],
  "filter":[{"operator":"eq","operands":["fldA","value"]}],
  "sort":[{"fieldId":"fldA","direction":"asc"}]
}'
```
