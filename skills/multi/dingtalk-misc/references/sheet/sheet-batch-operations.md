# Sheet 批量原子操作

## 使用边界

`batch-update` 用于把多个已经确认参数、需要同一事务语义的 Sheet 写操作组合成一次请求；`range batch-clear` 用于跨工作表批量清空。不要把一次独立写强行拆成 batch，也不要为了减少调用把不相关风险混在同一个事务里。

默认不传 `--continue-on-error`：任一子操作失败时，已执行操作全部回滚。只有用户明确接受部分成功时才启用宽松模式，并逐项报告成功、失败和实际返回。

## 支持的 `toolName`

`toolName` 必须逐字使用下列 CLI 命令名：

- 值与区域：`range clear`、`range update`、`range fill`、`range copy-to`、`range move-to`、`range sort`、`range set-style`、`csv-put`、`replace`、`merge-cells`、`unmerge-cells`。
- 行列与分组：`add-dimension`、`insert-dimension`、`delete-dimension`、`move-dimension`、`update-dimension`、`group-dimension`、`ungroup-dimension`。
- 下拉：`set-dropdown`、`delete-dropdown`。
- 工作表：`new`、`delete-sheet`、`update`、`show-gridline`、`hide-gridline`。
- 对象删除：`chart delete`、`pivot-table delete`。
- 条件格式：`cond-format create`、`cond-format update`、`cond-format delete`。
- 筛选：`filter create`、`filter update`、`filter delete`。
- 个人筛选视图：`filter-view create`、`filter-view update`、`filter-view delete`。
- 浮动图片：`create-float-image`、`update-float-image`、`delete-float-image`。

以下能力不进入 `batch-update`：`range read`、`csv-get`、`table-get`、`table-put`、`write-image`、`media-upload`、`range batch-set-style`、`find`、`copy`、`chart create/update`、`pivot-table create/update`、`export`、`sheet list/info`、嵌套 batch，以及需要立即折叠的 `group-dimension`。结构化 table 用独立 `table-put`；批量样式入口用独立 `range batch-set-style`；本地图片先执行 `media-upload`。

## 批量清空

```bash
dws sheet range batch-clear \
  --node <NODE_ID> \
  --ranges '["Sheet1!A1:B3","Sheet2!C1:D5"]' \
  --type content --format json
```

每个范围必须带工作表前缀。`type` 为 `content`、`format` 或 `all`；`all` 会同时删除值和格式。执行前确认精确目标；默认原子，任一区域失败时整批回滚。

## `batch-update` 请求结构

```bash
dws sheet batch-update --node <NODE_ID> --operations '[
  {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},
  {"toolName":"range update","input":{"sheet-id":"Sheet1","range":"A1","values":[[{"type":"text","text":"hello"}]]}},
  {"toolName":"merge-cells","input":{"sheet-id":"Sheet1","range":"A1:B1","merge-type":"mergeAll"}},
  {"toolName":"csv-put","input":{"sheet-id":"Sheet1","start-cell":"E1","csv":"001,2026/8/1,=1+1,'=1+1","auto-convert":false}}
]' --format json
```

每项必须是 `{"toolName":"...","input":{...}}`：

- `input` 的键使用对应 CLI flag 名去掉 `--`，例如 `sheet-id`、`start-index`、`merge-type`。
- `node` 只在批次顶层传一次，禁止放入子操作。
- 子操作按数组顺序执行，单批最多 20 条。
- 所有 `sheet-id`、范围、对象 ID 都必须来自当前任务的真实返回；不能猜 `Sheet1`、`0` 或历史 ID。
- 不确定字段时只读取对应 standalone leaf 的 compact Schema，不要批量读取全部 Help。

用户侧始终传 `--operations`，不新增 `--operations-json`。CLI 在本地解析、严格校验和翻译后，只把最终 MCP 操作数组编码到远端 `operationsJson` string，避免平台把 JSON 中的 number/boolean 改成字符串。`--dry-run` 的 `arguments.operationsJson` 会显示为带转义的 JSON 字符串，这就是实际请求形态；CLI 不同时发送旧 `operations`，写失败后也不会换入口自动重试。

新增操作会在发请求前拒绝未知字段和错误 JSON 类型：整数、布尔值不能写成字符串。`operations[N]` 中的 `N` 是 0-based 的错误/结果索引，仅用于对应 `--operations` 数组项，不是 Sheet 的行列坐标。

## 关键参数语义

- `csv-put` 子操作用 `"auto-convert":false` 关闭非公式字段自动转换，向 MCP 透传为 `autoConvert:false`。首字符为 `=` 的字段仍按公式解析；`'=...` 是普通文本并保留前置单引号。缺省或 `true` 保持现有自动类型转换行为。
- `delete-dimension` / `move-dimension` 与独立命令一致：`ROWS` 使用 1-based 行号，`COLUMNS` 使用列字母；DWS 负责转换成 batch API 的 0-based 索引，调用方不要提前减 1。其他子操作也应沿用对应 standalone 命令的公开坐标契约。
- `set-dropdown` 的 Inline 模式使用 `options`；SourceRange 模式使用 `source-sheet-id` 与 `source-range`，两种模式必须且只能选一个。Inline 颜色写在 `options[].color`；顶层 `colors` / `source-colors` 和 SourceRange 颜色不支持。
- `source-range` 按 `toolName` 解释：在 `set-dropdown` 中是候选项来源，在 `range fill` / `range copy-to` / `range move-to` 中是数据源区域。
- `create-float-image` / `update-float-image` 在 batch 中只接受 `src` URL，不接受 `file`；本地文件必须先上传。
- `group-dimension` 在 batch 中只适合默认展开分组；需要 `group-state=fold` 时调用独立命令。

## 返回值与批内依赖

batch 不支持 `$ref`，后序子操作不能引用前序 create 的返回。`new`、`cond-format create`、`filter create`、`filter-view create`、`create-float-image` 等创建类操作成功时，对应 `results[].data` 会保留服务端返回（包括可用的 ID）；调用方只能在下一次请求中使用这些 ID。

`update` 改名或 `delete-sheet` 删除工作表后，后序子操作若仍按旧名称定位会失败：严格模式整批回滚，宽松模式保留此前已成功操作。跨操作关联优先使用稳定的 `sheet-id`；同一静态批次内不能依赖前序新建对象的 ID。

## 严格与宽松模式

| 模式 | flag | 子操作失败时 | 结果判断 |
|---|---|---|---|
| 严格事务（默认） | 不传 | 整批回滚，后续不再执行 | 顶层错误包含失败的 `operations[N]` |
| 宽松模式 | `--continue-on-error` | 保留已执行操作并继续 | 逐项检查 `results[].success`、`errorCode`、`errorMsg` 和 `data` |

顶层请求成功不代表宽松模式下每个子操作都成功。必须按原输入索引解析全部结果，不能让成功项掩盖失败项。

## 预检与安全

发送前检查：

- 操作数与数组长度一致且不超过 20；
- 工作表、范围和对象 ID 真实，范围没有意外重叠；
- 操作顺序满足依赖，删除、清空、移动和覆盖的影响已确认；
- 不包含 unsupported toolName、嵌套 batch、`$ref` 或子操作 `node`；
- 非幂等 create/insert/move 没有在状态未知时被重放。

超时或返回状态不确定时先回读目标。原子模式先判断整批是否落地；宽松模式只为已证明缺失且可安全重试的项构造新批次。不要盲目重发整个批次。

## 写后验证

合并相同类型的验证，避免逐操作全表读取：

- 值和公式：回读覆盖受影响值的最小范围；公式先读文本，再按需执行 `formula-verify`。
- 样式、合并、网格线、行列和分组：使用 `sheet info` 或对应结构读取；分组加 `--include groups`。
- dropdown、条件格式、筛选、筛选视图、图表、透视表、浮动图片：调用对应 `list/get`。
- 创建类：除了检查 `results[].data`，还要用对象读取确认持久化状态。

预期条数、结果条数或关键状态不一致即失败。只读验证可做有界退避，不得通过重复写“碰碰运气”。最终说明使用的是原子还是部分模式、成功项、失败项及已验证范围。
