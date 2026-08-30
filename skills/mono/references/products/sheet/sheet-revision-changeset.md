# Sheet revision 与 changeset

## 产品语义

revision-get / changeset-get 用于编辑审计和前向语义变化，不是历史快照，也不能 revert。需要保存、列出或恢复在线历史版本时进入 sheet-version 阶段。changeset 不能代替当前值回读。

## 获取区间

    dws sheet revision-get --node <NODE_ID_OR_URL> --format json

从成功 envelope 的 data.revision 读取整数。要观察后续变化，保存该 revision，再执行：

    dws sheet changeset-get       --node <NODE_ID_OR_URL>       --start-revision <START>       --end-revision <END>       --format json

省略 end-revision 表示查询到请求开始时固定下来的最新 revision。参数硬约束：

- start/end 都是非负整数，revision 0 是合法空工作簿基线。
- 查询区间是 `(startRevision, endRevision]`：不包含 start，包含 end；start=end 合法并返回空 changesets。
- 单次跨度最多 20，即 end-start<=20。更大范围必须拆成首尾连续、无遗漏无重叠的分段；任一段失败或不完整，都不能宣称整个跨度完整。
- start/end 必须来自当前文档、当前 profile 的真实返回；不要使用时间戳、猜测值或另一个文档的 revision。

## 解读顺序

1. 先看请求区间和返回终点，确认没有查询错文档或区间。
2. 查看 detailsStatus 与 containsIncompleteChanges。
3. 按 changesets 顺序读取事件类型，再读取每项 changes。
4. 结合 targets 的 role、range/relative offset 判断来源、目标和受影响区域。
5. 最后回读当前范围，确认最终状态。

detailsStatus=COMPLETE 且 containsIncompleteChanges=false 才能把明细称为完整。PARTIAL、UNAVAILABLE、缺失明细或未知变更类型都应明确标注“审计信息不完整”，不能当作无变化。

## 关键字段

- 事件类型常见 EDIT、UNDO、STATE_RESET。UNDO 表示撤销事件，不代表简单删除上一条；STATE_RESET 可能使此前增量解释失效。
- isSelfEdit=false 只表示不是当前请求用户提交，不能据此归因到某个其他用户、系统或自动化。
- STATE_RESET 的 targetStatus=UNAVAILABLE 时不得猜 targetRevision；即使目标已知，也必须回读当前工作簿。
- changes 描述单元格、范围、工作表、行列、分组、数据验证等语义变化。
- targets 中 SOURCE、DESTINATION、AFFECTED 是角色，不是最终值；相对偏移必须结合该 change 的基准解释。
- 字段为 null、缺失和显式 clear 含义不同，不能统一归为“空”。
- changeset 记录操作语义，后续编辑、撤销或重置可能已经改变最终状态。

## 错误与完成条件

非零退出、ok=false、坏 JSON、revision 类型错误、区间不一致或 incomplete 状态均不得伪装成空 changesets。只有明确成功、完整并覆盖请求区间时，空 changesets 才能解释为该区间未返回可见变化。

最终报告应区分：观察区间、完整性、发生过的操作、以及当前回读状态。涉及关键数据时，用 csv-get/range read 回读最小范围；涉及结构对象时用 info 或对应 list/get。分段结果只在所有段连续、完整且终点覆盖目标 end 时合并。
