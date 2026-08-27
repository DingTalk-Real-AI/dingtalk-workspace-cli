# Sheet 批量原子操作

## 使用边界

batch-update 用于多个相互依赖且已确认参数的原子写；range batch-clear 用于跨工作表批量清空。不要把独立命令能完成的一次写拆成 batch，也不要为了减少调用把不相关风险混在一起。

当前 batch-update 只支持以下精确 toolName：`range clear`、`range update`、`merge-cells`、`unmerge-cells`、`range fill`、`range copy-to`、`add-dimension`、`delete-dimension`、`move-dimension`、`update-dimension`、`group-dimension`、`ungroup-dimension`、`set-dropdown`、`delete-dropdown`、`csv-put`、`delete-float-image`。

以下不进入 batch-update：set-style/batch-set-style、table-put/table-get、range read/csv-get、对象 create/update/list、嵌套 batch、需要立即折叠的 group-dimension。样式批量使用独立 range batch-set-style；结构化 table 独立写并独立回读；折叠分组用独立 group-dimension --group-state fold。不要把相似的 CLI leaf 名称猜成受支持 toolName。

## 批量清空

    dws sheet range batch-clear       --node <NODE_ID>       --ranges '["Sheet1!A1:B3","Sheet2!C1:D5"]'       --type content --format json

每个范围必须带工作表前缀。type 为 content、format 或 all；all 会同时删除值和格式，属于高风险操作。执行前读最小范围并展示目标，获得明确确认后才执行破坏性清空。默认原子：任一区域失败整批回滚。

## batch-update 结构

    dws sheet batch-update --node <NODE_ID> --operations '[
      {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},
      {"toolName":"range update","input":{"sheet-id":"Sheet1","range":"A1","values":[[{"type":"text","text":"hello"}]]}},
      {"toolName":"merge-cells","input":{"sheet-id":"Sheet1","range":"A1:B1","merge-type":"mergeAll"}}
    ]' --format json

每项形如 {"toolName": "...", "input": {...}}：

- toolName 必须逐字取上面的支持清单，不接受 MCP RPC 名或缩写。
- input 键使用 CLI flag 名去掉 --，例如 sheet-id、start-index、merge-type。
- node 只在批次顶层传，不在子操作中重复。
- 子操作按数组顺序执行；依赖前项产生的新 ID 的对象不适合放进同一静态批次。
- 不确定某个 leaf 的字段时只读该 leaf compact Schema，不读取所有 Help。

set-dropdown 的 batch input 仍遵循精确互斥：inline 用 options（颜色只能写 options[].color）；SourceRange 用 source-sheet-id 与 source-range 且二者必须同时出现。options 与 source-range 必须且只能选一个；顶层 colors/source-colors 不支持，SourceRange 颜色也不支持。

默认不加 --continue-on-error，这样任一失败整批回滚。只有用户明确接受部分成功时才加；此时顶层请求可能成功但子项仍有失败，必须按输入索引逐项解析状态、错误和实际结果，失败项不能被成功项掩盖。

## 预检与安全

批次发送前检查：所有 sheetId 来自当前任务；范围无意外重叠；操作顺序满足依赖；删除/清空/移动影响已确认；预计操作数与数组长度一致。不要用 validate 成功替代真实执行，也不要在失败后盲目重发整批。

超时或响应不确定时先回读目标状态：

- 原子模式：判断整批是否已落地，再决定是否重试。
- continue-on-error：只为已证明缺失且可安全重试的项目构造新批次。
- 非幂等操作如 append、insert、move 未确认状态前禁止重放。

## 写后验证

批次成功后按结果类型合并验证，避免逐操作全表读取：

- 值：一次读取覆盖所有受影响值的最小范围；
- 样式/合并/行列：info 或样式读取；
- dropdown/对象：对应 list/get。

预期条数、结果条数或关键状态不一致即失败。只读校验可做有界退避；不得通过重复写“碰碰运气”。最终说明原子/部分模式、成功项、失败项和已验证范围。
