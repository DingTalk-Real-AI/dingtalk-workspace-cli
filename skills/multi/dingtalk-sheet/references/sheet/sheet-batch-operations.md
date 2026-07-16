# 批量操作

## 写入边界 + 回读校验

`batch-update` 把多次写入打包成单次请求，但每个子操作仍受编辑类任务硬性默认规则约束：

1. **目标 range 必须落在用户授权范围内**：除用户明示要修改的区域外，子操作禁止扩张到无关单元格 / 列 / 工作表。规划 range 时先确认每个子操作的边界。
2. **批次完成后必须回读校验**：整个 `batch-update` 执行成功后，用 `range read` 或 `csv-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与本地预先计算的预期值对照。
3. **预期条数前置断言**：涉及"批量填充 N 行"或"对 M 个区域分别写入"时，先把 N、M 硬编码进代码，回读后断言实际等于预期；不一致就再发一轮 `batch-update` 补齐，禁止交付半成品。
4. **统一确认语义**：`range batch-clear` 和 `batch-update` 都可删除内容或结构；先用 `--dry-run` 预览，真实执行必须获得用户确认后追加 `--yes`。

## 使用场景

用户说"批量清除/一次清除多个区域/清空多个 sheet 的数据":
- 批量清除多个区域 → `range batch-clear`
- 每个区域必须带工作表前缀（如 "Sheet1!A1:B3"）
- 原子事务：任一区域清除失败则整批回滚
- 请勿用多次 `range clear` 逐个调用，非原子，中途失败会留下半成品

用户说"批量操作/一次执行多个写操作/原子批量/先清除再写入":
- 多个写操作打包 → `batch-update`
- 支持混合操作（清除 + 写入 + 合并 + 调行列尺寸等）
- 原子事务：任一操作失败则整批回滚
- 传 `--continue-on-error` 可切换为宽松模式（失败继续）

**何时推荐使用 `batch-update`**：
- 需要对**多个**不同区域执行 `merge-cells` / `unmerge-cells` 时（如按分组合并多列相同内容）
- 需要对**多个**不同区域执行 `update-dimension` 时（如统一调整多列列宽或多行行高，含 hidden + pixelSize）
- 需要先插入行列再写入数据时（`add-dimension` + `range update` / `csv-put`）
- 需要批量创建或取消行列分组时（`group-dimension` / `ungroup-dimension`）
- 需要对多个区域执行不同写入操作时（多次 `range update` + `range clear` 等组合）

当同一工具需要对多个区域重复调用时，**推荐**改用 `batch-update` 合并为单次请求——`batch-update` 是原子提交（要么全成功要么整批回滚）；逐个调用非原子，中途失败会留下半成品。

**不可放进 `--operations` 的操作**（强行写入会被校验拒或行为未定义）：
- `range read` / `csv-get`（只读操作，不属于写入）
- `range sort` / `range move-to`（尚未支持，后续版本补充）
- `write-image` / `media-upload` / `create-float-image` / `update-float-image`（需本地上传或依赖前置上传句柄）
- `set-style` / `range batch-set-style`（自身已是批量入口，不可再嵌套）
- `find` / `replace`（尚未支持，后续版本补充）
- `export`（异步轮询操作）
- `sheet list` / `sheet info`（只读操作）

这些操作需在 `batch-update` 之外单独调用。

## 命令详细参考

### 批量清除区域
```
Usage:
  dws sheet range batch-clear [flags]
Example:
  dws sheet range batch-clear --node <NODE_ID> --ranges '["Sheet1!A1:B3","Sheet2!C1:D5"]' --yes
  dws sheet range batch-clear --node <NODE_ID> --ranges '["Sheet1!A1:Z1000"]' --type all --yes
  dws sheet range batch-clear --node <NODE_ID> --ranges '["Sheet1!A2:A100","Sheet1!C2:C100"]' --type format --yes
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --ranges string          目标区域 JSON 数组，每项带 sheet 前缀 (必填)
      --type string            清除类型: content(仅值,默认) / format(仅格式) / all(全部)
Notes:
  - 原子事务：任一区域清除失败则整批回滚
  - 每个 --ranges 项必须包含工作表前缀（格式: "SheetName!A1:B3"）
  - 不同区域可以属于不同工作表
```

### 批量更新（原子事务）
```
Usage:
  dws sheet batch-update [flags]
Example:
  dws sheet batch-update --node <NODE_ID> --operations '[
    {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},
    {"toolName":"range update","input":{"sheet-id":"Sheet1","range":"A1","values":[[{"type":"text","text":"hello"}]]}},
    {"toolName":"merge-cells","input":{"sheet-id":"Sheet1","range":"A1:B1","merge-type":"mergeAll"}},
    {"toolName":"update-dimension","input":{"sheet-id":"Sheet1","dimension":"ROWS","start-index":"1","length":1,"pixel-size":40}}
  ]' --yes
  dws sheet batch-update --node <NODE_ID> --continue-on-error --operations '[...]' --yes
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --operations string      操作数组 JSON (必填，每项 {toolName, input})
      --continue-on-error      遇失败继续执行（默认 false，严格事务）
toolName 用 CLI 命令名，input 的键用 CLI flag 名去掉 --：
  range clear / range update / merge-cells / unmerge-cells / update-dimension
  range fill / range copy-to / add-dimension / delete-dimension / move-dimension
  group-dimension / ungroup-dimension
  set-dropdown / delete-dropdown / csv-put / delete-float-image
Notes:
  - 默认严格事务模式：任一子操作失败 → 整批回滚到初始状态
  - --continue-on-error: 宽松模式，遇失败继续执行后续操作（已执行的子操作不回滚）
  - operations 最多 20 条
  - 当需要对多个区域执行相同清除时，优先使用 `range batch-clear`（更简洁）
  - 典型场景：先插入行列再写入数据、先清除再写入、批量合并+调整行高列宽
  - 真实执行必须获得用户确认后加 --yes；--dry-run 不需要 --yes
```

### 子操作定位规则

> **`--node` 只在顶层传一次**，每个子操作的 `input` 不含 `node` 字段；
> **子表定位 `sheet-id` 写进各自 `input`**，不同子操作可指向不同工作表。

```jsonc
// 正确：node 在顶层，sheet-id 在各自 input
[
  {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3"}},
  {"toolName":"range update","input":{"sheet-id":"Sheet2","range":"A1","values":[[...]]}}  // 不同工作表
]
```

## 校验与预览

### Validate 阶段

CLI 在发送请求前执行以下本地校验（不消耗网络请求）：

| 校验项 | 失败行为 | 示例 |
|--------|---------|------|
| `--node` / `--operations` 必填 | `missing required flag(s)` | 不传 `--node` |
| `--operations` 必须合法 JSON | `--operations JSON 解析失败` | `--operations 'not json'` |
| `--operations` 必须非空数组 | `--operations 不能为空数组` | `--operations '[]'` |
| 每项必须是 object | `operations[N] 不是 object` | `--operations '["string"]'` |
| `toolName` 必须是支持的 CLI 命令名 | `unsupported toolName "xxx"` | 传 `"batch-update"`（禁止嵌套）或拼写错误 |
| 禁止嵌套 `batch-update` | 同上（`batch-update` 不在 dispatch 表中，自动拦截） | `toolName: "batch-update"` |

> `input` 内的字段（如 `sheet-id`、`range`、`values` 等）由服务端校验，CLI 不提前拦截——传入空值或缺失必填字段时，请求会到达服务端再返回错误。

### DryRun 阶段

加 `--dry-run` 预览翻译后的完整 MCP 请求，不实际执行任何操作：

```bash
dws sheet batch-update --node NODE_ID --dry-run --operations '[...]'
```

DryRun 输出内容：
- `Tool: batch_update`（最终调用的 MCP 工具名）
- `Arguments:` 完整 JSON（含翻译后的 `operations` 数组，展示每个子操作的 MCP toolName + 参数名 + 参数值）
- 若传了 `--continue-on-error`，输出 JSON 中会包含 `"continueOnError": true`

DryRun 适用场景：
- 确认 CLI 命令名 → MCP toolName 的翻译是否正确
- 确认参数名转换（如 `--range` → `rangeAddress`）是否符合预期
- 检查 `csv-put` 的 `@filepath` 是否被正确解析为文件内容

### --continue-on-error 行为

| 模式 | flag | 子操作失败时 | 返回格式 |
|------|------|------------|---------|
| 严格事务（默认） | 不传 | 整批回滚到初始状态，后续子操作不执行 | 顶层 `error` 对象 |
| 宽松模式 | `--continue-on-error` | 已执行的子操作不回滚，继续执行后续子操作 | `results` 数组 |

**严格事务失败的错误信息**由服务端（flex-table-app）生成，包含以下内容：
- 失败操作索引：`operations[N]`（0-based，即 `--operations` JSON 数组中的下标）
- 回滚告知："All previously executed operations have been rolled back."
- 失败原因：原始错误码和错误消息

示例：
```
Batch update failed at operations[1] (0-based index).
All previously executed operations have been rolled back.
Cause: The requested resource was not found by the identifier 'NonExistentSheet'...
```

**宽松模式下**，返回 `results` 数组，逐个标注每个子操作的成功/失败状态：
- 成功的子操作：`success: true` + `message`
- 失败的子操作：`success: false` + `errorCode` + `errorMsg`
- 顶层 `success: true`（batch 本身完成执行，只是部分子操作失败）

## CLI 命令名 → 原子命令参考

`toolName` 使用 CLI 命令名，`input` 的键使用 CLI flag 名（去掉 `--`），与原子命令文档完全一致。各命令的参数详情请查阅对应参考文档或执行 `dws sheet <命令> --help`。

| CLI 命令名 | 参考文档 |
|----------|---------|
| `range clear` | [sheet-range-operations](./sheet-range-operations.md) |
| `range update` | [sheet-write-data](./sheet-write-data.md) |
| `merge-cells` | [sheet-style-format](./sheet-style-format.md) |
| `unmerge-cells` | [sheet-style-format](./sheet-style-format.md) |
| `update-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `range fill` | [sheet-range-operations](./sheet-range-operations.md) |
| `range copy-to` | [sheet-range-operations](./sheet-range-operations.md) |
| `add-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `delete-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `move-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `group-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `ungroup-dimension` | [sheet-dimension-operations](./sheet-dimension-operations.md) |
| `set-dropdown` | [sheet-dropdown](./sheet-dropdown.md) |
| `delete-dropdown` | [sheet-dropdown](./sheet-dropdown.md) |
| `csv-put` | [sheet-write-data](./sheet-write-data.md) |
| `delete-float-image` | [sheet-media-image](./sheet-media-image.md) |

## 典型组合场景

### 插列 + 写表头 + 整列回填

一次原子提交，不要拆成多次独立调用。批量回填同一列**只需一次** `range update`（range 写整列范围、values 写 N×1 矩阵），不需要逐行循环。

```jsonc
// 在 C 列前插入新列 → 写表头 C1 → 回填 C2:C100 共 99 行
[
  {"toolName":"add-dimension",
   "input":{"sheet-id":"...","dimension":"COLUMNS","length":1}},
  {"toolName":"range update",
   "input":{"sheet-id":"...","range":"C1:C100",
            "values":[[{"type":"text","text":"score"}],[{"type":"text","text":"95"}],[{"type":"text","text":"87"}]/* ... 97 more rows ... */]}}
]
```

### 先清除再写入

清除目标区域残留数据后写入新数据，保证目标区域干净。

```jsonc
[
  {"toolName":"range clear",
   "input":{"sheet-id":"...","range":"A1:D50","type":"all"}},
  {"toolName":"range update",
   "input":{"sheet-id":"...","range":"A1:D50",
            "values":[[{"type":"text","text":"姓名"},{"type":"text","text":"部门"},{"type":"text","text":"金额"},{"type":"text","text":"日期"}],/* ... */]}}
]
```

### 批量合并 + 调整行高列宽

统一多列合并和尺寸调整，原子提交避免中途失败留下不一致格式。

```jsonc
[
  {"toolName":"merge-cells",
   "input":{"sheet-id":"...","range":"A1:B1","merge-type":"mergeAll"}},
  {"toolName":"merge-cells",
   "input":{"sheet-id":"...","range":"C1:D1","merge-type":"mergeAll"}},
  {"toolName":"update-dimension",
   "input":{"sheet-id":"...","dimension":"ROWS","start-index":"1","length":1,"pixel-size":40}},
  {"toolName":"update-dimension",
   "input":{"sheet-id":"...","dimension":"COLUMNS","start-index":"A","length":4,"pixel-size":120}}
]
```

### 批量创建行列分组

```jsonc
[
  {"toolName":"group-dimension",
   "input":{"sheet-id":"...","range":"3:7","group-state":"expand"}},
  {"toolName":"group-dimension",
   "input":{"sheet-id":"...","range":"C:F"}},
  {"toolName":"ungroup-dimension",
   "input":{"sheet-id":"...","range":"10:12"}}
]
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | 工作表的 `sheetId` | batch-clear 的 --ranges sheet 前缀 / batch-update 子操作 input 的 sheet-id |
| `batch-update` | 各子操作的执行结果 | 回读校验（range read / csv-get） |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ **需要对多个区域执行相同清除操作时，用 `range batch-clear`**：一次原子请求清除多个区域（可跨工作表），失败时整批回滚
- ★ **需要组合多个不同写操作（清除+写入等）时，用 `batch-update`**：原子事务，任一操作失败则整批回滚，避免留下半成品
- ★ **批次完成后必须回读校验**：用 `range read` 或 `csv-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与预期值对照
- ★ **`batch-update` 不支持嵌套**：`--operations` 中的 `toolName` 必须是原子操作，不可再嵌套 `batch-update`
- `batch-update` 支持 `group-dimension` / `ungroup-dimension`；`input.range` 只接受整行或整列范围，如 `3:7`、`C:F`
- 需要创建后立即折叠分组时，优先使用独立 `dws sheet group-dimension --group-state fold`
