# 工作簿 revision 与 changeset

## 何时使用

当用户要知道在线电子表格当前 revision，或要复核两个 revision 之间提交了哪些变更、是否发生撤销或整表状态替换时使用。

- 当前 revision → `dws sheet revision-get`
- revision 区间内的变更 → `dws sheet changeset-get`
- 可保存、展示或回滚的历史版本 → [sheet-version](./sheet-version.md)
- 当前单元格值、公式或工作簿结构 → 使用对应 read/list/get 命令

两条命令都是工作簿级能力，只接受 `--node`，不接受 `--sheet-id`。`--node` 可传在线电子表格文档 ID 或完整 URL；`spreadsheetv2` URL 必须原样传入。

revision 表示工作簿的编辑进度；历史版本是选定的保存或回滚点。`version list` 返回的 `version` 可以作为 changeset 查询锚点，但相邻历史版本之间可能包含多个 revision。

当用户明确要求恢复到某个精确 revision 时，可以把已确认属于同一工作簿的 revision 传给 `version revert --version`，即使它没有出现在 `version list` 中。默认仍应从版本列表选择稳定的历史版本；未列入列表的 revision 只有在服务端仍可恢复时才会成功，禁止猜测或自动改用相邻 revision。具体安全流程见 [sheet-version](./sheet-version.md)。

## 获取当前 revision

```bash
dws sheet revision-get --node <NODE_ID_OR_URL> --format json
```

成功时从统一输出的 `data.revision` 读取当前 revision：

```json
{
  "ok": true,
  "outcome": "success",
  "data": {
    "success": true,
    "logId": "trace-id",
    "revision": 142
  }
}
```

- `revision=0` 是合法的空工作簿基线。
- revision 不是工作表数量，也不是历史版本列表序号。
- `logId` 仅用于失败反馈和问题排查，不是 revision。

## 获取 revision 区间 changeset

```bash
dws sheet changeset-get \
  --node <NODE_ID_OR_URL> \
  --start-revision 120 \
  --end-revision 140 \
  --format json
```

也可以省略 `--end-revision`，查询到本次请求开始时的最新 revision：

```bash
dws sheet changeset-get \
  --node <NODE_ID_OR_URL> \
  --start-revision 120 \
  --format json
```

参数规则：

- `--start-revision` 必填，必须为非负整数；`0` 合法。
- `--end-revision` 可选；提供时必须大于或等于 start。
- 查询区间固定为 `(startRevision, endRevision]`：不包含 start，包含 end。
- `startRevision == endRevision` 合法，返回空 `changesets`。
- 单次最多查询 20 个 revision，即 `endRevision - startRevision <= 20`；更大区间必须连续分段查询。
- 省略 end 时，本次响应会固定一个 `endRevision`；查询期间产生的新 revision 不进入本次结果。
- 无法完整取得所请求区间时整次失败，不会把缺失的 revision 当作成功结果。

成功后直接读取统一输出中的 `data.changesets` 数组。DWS 已把它转换为可遍历的 JSON 对象，不需要额外解析。

## 先看区间摘要

`data.summary` 用于快速判断本次结果是否完整以及哪些工作表受到影响：

| 字段 | 含义 |
|------|------|
| `changeCount` | `changes[]` 中变更对象的总数 |
| `completeChangeCount` | 详情完整的变更数量 |
| `partialChangeCount` | 只返回部分可信详情的变更数量 |
| `unsupportedChangeCount` | 当前无法解释的变更数量 |
| `containsStateReset` | 区间内是否发生整表状态替换 |
| `containsIncompleteChanges` | 是否存在不完整或无法解释的详情 |
| `affectedSheets` | 受影响工作表及其 A1 范围摘要 |

`affectedSheets[]` 常用字段：

| 字段 | 含义 |
|------|------|
| `sheetId` | 工作表 ID |
| `sheetName` | 可用时的工作表名称；缺失时不要从 ID 猜测 |
| `ranges` | 去重后的 1-based A1 范围；工作表级变更可为空数组，整行/整列可表示为 `1:3` / `A:C` |

摘要用于定位，不是完整变更副本。要回答改了什么，仍需遍历 `changesets[].changes[]`。

## 读取 `changesets[]`

每个 changeset 对应一个 revision：

| 字段 | 含义 | 使用规则 |
|------|------|----------|
| `revision` | 该事件对应的 revision | 按升序解释 |
| `createTime` | 事件创建时间 | 时间仅用于说明，顺序以 revision 为准 |
| `isSelfEdit` | 是否由当前请求用户提交 | `false` 不能用于识别具体编辑者，也不能判断是否由人工或自动化产生 |
| `eventType` | `EDIT`、`UNDO` 或 `STATE_RESET` | 先按事件类型分流 |
| `detailsStatus` | `COMPLETE`、`PARTIAL` 或 `UNAVAILABLE` | 决定能否把详情当作完整描述 |
| `changes` | 本 revision 的变更数组 | `STATE_RESET` 时为空数组 |
| `reset` | 整表状态替换信息 | 只在 `STATE_RESET` 时读取 |

事件类型：

- `EDIT`：一次普通编辑提交产生的效果。
- `UNDO`：一次撤销提交产生的效果。只描述这次撤销实际改变了什么，不要反推被撤销操作的完整原貌。
- `STATE_RESET`：工作簿整体状态被替换。不要跨过该事件拼接推导完整状态；需要重新读取工作簿。

changeset 描述的是该 revision 提交的效果，不是统一的变更前/变更后快照，也不保证仍是当前最终状态。

## 读取 `reset`

| 字段 | 含义 |
|------|------|
| `type` | 状态替换类型：`ROLLBACK`、`OVERWRITE`、`UPGRADE`、`TEMPLATE`、`PRETTIFY` 或 `UNKNOWN_RESET` |
| `targetRevision` | 可确认时的目标 revision；`0` 是合法空基线 |
| `targetStatus` | `KNOWN` 表示目标已返回；`NOT_APPLICABLE` 表示该事件没有目标 revision；`UNAVAILABLE` 表示目标无法确认 |

当 `targetStatus=UNAVAILABLE` 时，禁止把目标猜成 `revision-1`。即使目标 revision 已知，也要通过读取命令确认需要的当前内容。

回滚完成后，可用回滚前后的 revision 查询 changeset：若返回 `STATE_RESET`、`reset.type=ROLLBACK` 且 `reset.targetRevision` 等于请求目标，表示回滚事件指向了该 revision；工作簿内容是否符合预期仍需独立回读。

## 读取 `changes[]`

每个 change 都按以下顺序读取：

1. `type`：发生了哪类变更。
2. `targets`：变更作用在哪个工作簿、工作表或范围。
3. `detailsStatus`：详情是否完整。
4. `details`：本次变更的可用详情。
5. `omissions`：哪些详情未能提供。

公共字段：

| 字段 | 含义 |
|------|------|
| `type` | 语义变更类型，见下方字典 |
| `targets` | 工作簿、工作表或 A1 范围定位数组 |
| `details` | 与 `type` 对应的变更详情；无详情时为空对象 |
| `detailsStatus` | `COMPLETE`、`PARTIAL` 或 `UNAVAILABLE` |
| `omissions` | 未提供详情的原因；没有省略时为空数组 |

`detailsStatus` 的处理规则：

- `COMPLETE`：已完整解释本类型可提供的本次变更详情，但不代表包含旧值或当前值。
- `PARTIAL`：已返回字段可信，同时存在缺失；回答时要明确“仅确认到这些内容”。
- `UNAVAILABLE`：不能可靠说明具体详情；只报告事件、类型和可信位置，不猜内容。

`omissions[].fields` 可指出受影响的详情字段。`omissions[].code` 只用于判断存在何种缺口；面向用户时翻译成“部分详情未提供”“公式文本不可用”“位置无法确认”等自然语言，不需要展示内部原因码。

## 读取 `targets[]`

| 字段 | 含义 |
|------|------|
| `scope` | `WORKBOOK`、`SHEET` 或 `RANGE` |
| `sheetId` | 工作表 ID |
| `sheetName` | 可用时的工作表名称 |
| `sheetNameSource` | 名称是变更时名称、当前名称或无法确认 |
| `a1Range` | 1-based A1 范围 |
| `role` | `AFFECTED`、`SOURCE` 或 `DESTINATION`；缺失时按 `AFFECTED` 处理 |

- `SOURCE` 是读取或复制来源，不表示该位置被修改。
- `DESTINATION` 是写入目标，应算作受影响位置。
- `WORKBOOK` 级 target 通常没有工作表或 A1 范围，不要补造。
- `sheetName` 缺失或来源无法确认时，优先使用 `sheetId` 定位并按需调用 `sheet list`。

## 变更类型字典

### 行列与工作表

| type | 含义 | 常用 `details` |
|------|------|----------------|
| `ROWS_INSERTED` / `COLUMNS_INSERTED` | 插入行或列 | `startNumber`、`count`、`styleSource?` |
| `ROWS_DELETED` / `COLUMNS_DELETED` | 删除行或列 | `startNumber`、`count` |
| `ROWS_UPDATED` / `COLUMNS_UPDATED` | 更新行列属性 | `startNumber`、`count`、`properties?`、`relativeChanges?`、`replace?` |
| `SHEET_CREATED` | 创建工作表 | `position?`、`properties?`、`copiedFromSheetId?` |
| `SHEET_DELETED` | 删除工作表 | 通常由 target 定位 |
| `SHEET_UPDATED` | 更新工作表名称、位置或属性 | `position?`、`properties?` |
| `CUSTOM_TAB_ADDED` / `CUSTOM_TAB_DELETED` / `CUSTOM_TAB_UPDATED` | 新增、删除或更新自定义 Tab | `tabId`、`position?`、`properties?` |

`startNumber` 和 `position` 为 1-based。行列 `properties` 常见字段包括 `size`、`hidden`、`sticky`、`cellType`；工作表属性常见字段包括 `name`、`hidden`、`tabColor`、冻结行列数和默认行列尺寸。只解释实际返回的字段。

### 单元格与范围

| type | 含义 | 常用 `details` |
|------|------|----------------|
| `CELLS_INSERTED` / `CELLS_DELETED` | 在 target 内插入或删除单元格并平移 | `axis` |
| `RANGE_PASTED` | 将来源内容粘贴到目标范围 | `pasteMode`、`isCut`、`step?`、`includedParts?`、`contentPattern?` |
| `RANGE_AUTOFILLED` | 自动填充目标范围 | `fillMode`、`copyStyle`、`step?` |
| `RANGE_CLEARED` | 清除范围内指定内容 | `clearParts?`、`preservedCellTypes?` |
| `RANGE_CONTENT_SET` | 对整个范围写入相同内容或公式 | `cell?`、`replaceMode?` |
| `RANGE_BORDER_SET` | 设置范围边框 | `border?`、`borderSides?` |
| `RANGE_STYLE_SET` | 设置范围样式 | `style`、`replace?` |
| `RANGE_TAG_SET` | 设置或清除范围标签 | `tag?`、`replace?` |
| `RANGE_SORTED` | 对范围排序 | `sortByRange?`、`beforeOrder?`、`afterOrder?`、`indexesAreRelative` |
| `CELLS_CONTENT_SET` | 按相对位置分别修改内容 | `relativeChanges`、`replaceMode?` |
| `CELLS_STYLE_SET` | 按相对位置分别修改样式 | `relativeChanges?`、`styles?`、`styleMode?`、`replace?` |
| `CELLS_TAG_SET` | 按相对位置分别修改标签 | `relativeChanges`、`replace?` |

粘贴和自动填充的 `SOURCE` target 是来源，`DESTINATION` target 是写入位置。自动填充的 `DESTINATION` 可能是包含 `SOURCE` 的完整填充跨度；重叠部分不表示来源值发生了变化。描述新增填充区域时使用 `DESTINATION` 减去 `SOURCE`，核对最终结果时仍回读完整 `DESTINATION`。`step.rows` / `step.columns` 表示重复模式的大小。`contentPattern.cells[]` 使用 `rowOffset` / `columnOffset` 表示相对模式左上角的位置，并可返回写入的 `value`、`formula`、`cellType` 或 `link`。

`RANGE_SORTED.beforeOrder` / `afterOrder` 只列实际换位的行或列，未移动的位置会省略；数组短于 target 的行列数不代表详情缺失。`indexesAreRelative` 决定这些索引是相对 target 还是工作表绝对索引。

`relativeChanges[]` 中的 `rowOffset` / `columnOffset` 都是相对 target 左上角的 0-based 偏移，不是工作表绝对行列号；必须与 target 起点相加后再换算成 A1。

### 分组、数据验证和结构对象

| type | 含义 | 常用 `details` |
|------|------|----------------|
| `DIMENSION_GROUP_ADDED` / `DIMENSION_GROUP_REMOVED` / `DIMENSION_GROUP_UPDATED` | 新增、删除或更新行列分组 | `axis`、`startNumber`、`count`、`depth?`、`collapsed?` |
| `DATA_VALIDATION_SET` | 设置数据验证规则 | `ruleId?`、`dataValidation` |
| `DATA_VALIDATION_CLEARED` | 清除数据验证规则 | `ruleId?` |
| `CELLS_MERGED` / `CELLS_UNMERGED` | 合并或取消合并单元格 | `featureId?`、`effectless?` |
| `NAMED_RANGE_SET` | 设置命名范围 | `name`、`baseSheetId?`、`formula?` |
| `NAMED_RANGE_CLEARED` | 清除命名范围 | `name`、`baseSheetId?` |
| `FEATURE_ADDED` / `FEATURE_DELETED` / `FEATURE_UPDATED` | 新增、删除或更新筛选、评论、透视表、保护范围等扩展对象 | `featureType`、`featureId?` |

扩展对象通常只返回类型、标识和可信 target，不保证包含完整配置。用户询问当前配置时，调用该对象对应的 list/get 命令。

`dataValidation` 常用字段：

| 字段 | 含义 |
|------|------|
| `type` | 验证规则类型 |
| `sourceType` | 下拉来源：`inline` 或 `sourceRange` |
| `options` | 内联下拉选项数组，每项包含 `value` 和可选 `color` |
| `sourceRange` | 已确认的来源工作表与 A1 范围 |
| `sourceRangeStatus` | `RESOLVED`、`UNRESOLVED` 或 `INVALID` |
| `sourceRangeExpression` | 可读的来源表达式 |
| `enableMultiSelect` | 是否允许多选 |
| `criteria` | 非下拉规则的运算符和条件值 |
| `settings` | 是否允许空值、是否显示提示、错误提示等行为设置 |

只有 `sourceRangeStatus=RESOLVED` 时，才能把 `sourceRange` 当作已确认范围；否则说明来源未解析或无效，并在需要时回读当前规则。

### 工作簿设置与其他变更

| type | 含义 | 常用 `details` |
|------|------|----------------|
| `WORKBOOK_SETTING_UPDATED` | 更新工作簿设置 | `setting`、`changes` |
| `EXTERNAL_REFERENCES_REPLACED` | 替换外部引用 | `referenceCount` |
| `UNSUPPORTED_CHANGE` | 存在当前无法可靠解释的变更 | `category?` |

`WORKBOOK_SETTING_UPDATED` 的 `setting` 可表示计算设置或背景设置；只解释 `changes` 中实际返回的字段。遇到 `UNSUPPORTED_CHANGE` 时，只说明存在未能解释的变更，不根据相邻事件猜测其内容。

## 常用详情值

### 单元格内容

`cell` 常见字段：

- `value`：带类型的值；`kind` 可为 `STRING`、`NUMBER`、`BOOLEAN`、`NULL` 或 `MULTI`，分别读取对应的 `stringValue`、`numberValue`、`booleanValue` 或 `values`。
- `formula`：本次写入的可读公式。
- `formulaCleared=true`：本次清除了公式。
- `cellType`：`general`、`checkbox` 或 `select`；select 可带 `options`。
- `link`：可用时返回工作簿内范围链接及目标 `sheetId` / `a1Range`。

### 清除、null 与字段缺失

- 对象整体为 `{ "cleared": true }`：本次清除了该对象。
- `formulaCleared=true`：本次清除了公式。
- `styleCleared=true`：本次清除了该相对位置的样式。
- 对象内某个已知属性为 `null`：本次清除该属性或恢复默认。
- 字段缺失：本次未提供或不适用；若状态不是 `COMPLETE`，结合 `omissions` 判断缺口。

不要把整体清除、单属性清除和字段缺失混为一谈，也不要补造清除前内容。

## Agent 解读顺序

1. 检查统一输出 `ok=true`、`outcome=success` 和 `data.success=true`。
2. 直接遍历 `data.changesets`，不要把它当作字符串。
3. 用 `startRevision` / `endRevision` 准确表述 `(start,end]`。
4. 先看 `summary.containsStateReset` 和 `summary.containsIncompleteChanges`。
5. 每个事件先看 `eventType`；`STATE_RESET` 读 `reset`，`EDIT` / `UNDO` 再读 `changes`。
6. 每条 change 按 `type -> targets -> detailsStatus -> details -> omissions` 解读。
7. `PARTIAL` 只使用已返回字段并说明缺口；`UNAVAILABLE` 和 `UNSUPPORTED_CHANGE` 不猜。
8. `isSelfEdit=false` 不用于归因具体用户、系统或自动化。
9. 用户问“现在是什么”时，按 target 使用对应 read/list/get 命令回读。
10. 看到可用 revision 不代表已获准回滚；只有用户明确要求并确认覆盖风险后，才按 [sheet-version](./sheet-version.md) 执行 `version revert`。

## 最终状态回读

changeset 用于解释“某个 revision 提交了什么”，不是“当前最终是什么”。

- 值和公式：用 `csv-get`、`range read` 或 `table-get` 回读目标范围。
- 行列、分组、隐藏和尺寸：用 `sheet info --include ...` 或对应结构读取命令。
- 数据验证、筛选、评论、透视表等对象：用对应 list/get 命令回读。
- 粘贴或自动填充：优先回读 `DESTINATION`，不要把 `SOURCE` 当作已修改范围。
- `STATE_RESET`：重新读取工作簿当前状态。
- 分段查询时，只要任一段失败或包含不完整变更，就不能声称整个跨度已完整解释。

## 常见失败处理

- revision 非法或超出范围：先调用 `revision-get`，再修正或分段查询。
- 区间内容不可用：说明无法获得完整 changeset，不要解释为空 changeset。
- changeset 过大：缩小 revision 区间；不要假设结果会自动截断。
- 读取权限不足或无法确认受保护内容的完整读取权限：申请对目标内容的直接访问权限，或请管理员执行；不要把权限错误解释为空数据，也不要拆分范围绕过权限。
- `containsIncompleteChanges=true`：可以报告已确认部分和明确缺口，不能声称所有改动均已完整解析。
