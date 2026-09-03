# AISearch 多跳短流程

仅在 AISearch 已返回候选，但稳定 ID 域或下一跳衔接仍不明确时读取。每次只推进一跳；前置证据不成立就停止。

## 精确标题 → 原对象

1. `dws aisearch enterprise --queries "<完整标题>" --types <类型> --format json`。
2. 只接受标题精确匹配且唯一的候选；0 条、多条或只有近似标题时停止。
3. 按来源提取稳定 ID：文档 `nodeId`，听记 `taskUuid`；待办优先使用结构化 `taskId`，仅在返回确实只有深链时解析其中的 `taskId`。
4. 用户要求读取时才加载一个下游 Skill，并把该 ID 原样传入。搜索 `snippet` 不能替代读取结果。

## 人员 → 组织详情 → 动态关系

1. `aisearch person` 返回的所有候选必须保留。
2. 只按用户给定的姓名、昵称或身份关系筛选；条件不满足就停止，不能另搜一个同名或相似人补链。
3. 将候选的 `userId` 传给 `dws contact user get --ids <userId> --format json`。
4. 后续需要按主管姓名再搜人时，query 必须来自 Contact 详情的真实主管显示名；下一次 Contact 的 ids 必须来自新一次人员搜索。

人员链接中的 Ding uid 只用于和同域作者标识做身份核对，不能作为 Contact `userId`。

## 创建记录 → 详情 → 二次搜索

1. 用 `dws aisearch behavior --queries "<完整标题>" --types todo --behavior-type create --format json` 定位目标。
2. 只从精确标题候选取得 `taskId`；不得使用其他创建记录、列表第一项或历史 ID。
3. 用 `dws todo +get --task-id <taskId> --format json` 回读标题和业务字段。
4. 二次 AISearch 必须使用详情确认过的完整标题；多来源可合并到 `--types document,im`。

## 完整性与停止条件

- 标题/昵称/身份不匹配、稳定 ID 缺失、多候选未消歧：停止，不加载下游 Skill。
- 详情或正文接口失败：只报告该跳失败，不从搜索摘要猜内容。
- 逐字稿、分页列表或“全部”结果只有明确 `complete=true` 或等价分页完成证据时才能称完整。
- 用户指定“如果不一致/读取失败就不要继续”时，该条件优先于补交后续内容。
