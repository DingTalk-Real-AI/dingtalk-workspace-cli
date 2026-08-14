# AI 表格错误恢复指南

> 加载边界：仅在真实命令或业务返回失败后读取，不做预防性全量加载。先保留原错误、阶段、对象 ID 和已成功 ledger；只有能证明未写入且契约允许时才重试，状态未知和需确认写操作禁止自动重放。

> 当 CLI 命令返回错误时，按本文档的映射表判断恢复动作。

## 1. 错误响应结构

```json
{
  "status": "error",
  "summary": "Failed to create records",
  "trace_id": "2104a64c17790723347215232e085e"
}
```

- `status: "error"` 表示操作失败
- `summary` 包含错误摘要信息
- `trace_id` 用于问题追踪
- 命令退出码为 0 但 `status != "success"`、必需 ID 缺失或逐项结果含失败，也属于业务失败

## 2. 常见错误与恢复动作

### 2.1 记录操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `Failed to create records` | cellValue 格式错误或字段类型不匹配 | 先 `field get` 确认字段类型，再按 [cell-value](./aitable-cell-value.md) 规范重构值 |
| `record not found` | record-id 不存在或已删除 | 用 `record query` 重新查询确认目标记录 |
| rating 字段写入超出 max | 值超出字段配置范围 | 检查字段 config 的 min/max，确保值在范围内 |
| singleSelect 写入对象格式但 id 不存在 | option id 无效 | 改用 name 字符串写入（推荐），或先 `field get` 获取有效 option id |

### 2.2 字段操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `Failed to create field` | config 格式错误或必填项缺失 | 检查 [field-properties](./aitable-field-properties.md) 中该类型的必填 config |
| `field not found` | field-id 不存在 | 用 `field get` 获取最新字段列表 |
| formula 创建失败 | 公式语法错误或引用字段名不匹配 | 先 `field get` 确认字段精确名称，再检查公式语法（见 [formula-guide](./aitable-formula-guide.md)） |
| 删除主字段失败 | 主字段（第一列）不可删除 | 改为更新字段名或类型，不能删除 |

### 2.3 Base/Table 操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `base not found` | base-id 错误或无权限 | 确认 base-id 正确；尝试 `base list` 或 `base search` 重新定位 |
| `table not found` | table-id 错误 | 用 `table get --base-id <baseId>` 不带 table-ids 查看所有表 |
| 表名重复 | 同 Base 下已存在同名表 | 使用创建返回的真实 `tableId/tableName`；禁止继续按原名猜测目标 |

### 2.4 视图操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `view not found` | view-id 错误 | 用 `view get --base-id <baseId> --table-id <tableId>` 查看所有视图 |
| 删除最后一个视图 | 表至少保留一个视图 | 不可删除唯一视图 |

### 2.5 filters/sort 错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| filters 无效被忽略 | 根节点不是 and/or，或 operands 格式错误 | 确保 filters 根节点是 `{"operator":"and"/"or", "operands":[...]}` 结构 |
| sort 无效 | fieldId 不存在 | 先 `field get` 确认字段 ID |
| 筛选结果为空 | 条件过严或字段值不匹配 | 放宽条件验证；singleSelect/multipleSelect 筛选值使用 option name 字面量 |

### 2.6 导入导出错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| 导出任务超时 | 数据量大，异步任务未完成 | 用 `export data --task-id <taskId>` 轮询直到完成 |
| 导入文件格式错误 | 不支持的文件格式或文件损坏 | 确认文件为 .xlsx 格式且未加密 |
| 批处理只成功一部分 | 某批或某个字段返回失败 | 保留成功 ID，输出失败 batch/item ledger，以非零状态结束；不要整批重放 |

## 3. 状态感知恢复策略

不要用一个全局次数限制处理所有错误。先判断上一次调用是否可能产生副作用，再决定
探测、继续或停止。

| 已观察状态 | 恢复动作 |
|---|---|
| 只读调用失败 | 根据精确错误或 leaf 契约做有界探测；目标和父容器保持不变 |
| 写入前参数校验失败 | 确认无副作用后修正 payload/flag；不原样重试 |
| 网络超时 / 5xx，写入状态未知 | 先回读目标 Base/Table，确定未生效后才考虑重试 |
| 已返回新对象或记录 ID | 绑定该 ID，只继续尚未完成的步骤 |
| 异步任务未完成 | 继续查询同一个 taskId/importId，直到契约规定的终态或总超时 |
| 并发冲突且明确未写入 | 降低并发或串行重试；再次冲突时停止并报告 |
| 删除或高影响操作状态未知 | 停止并回读，不自动重放 |

### 3.2 不可重试的错误（立即停止）

| 错误类型 | 原因 | 处理方式 |
|---------|------|---------|
| 权限不足 / 403 | 用户对该 Base 无权限 | 停止操作，提示用户确认权限 |
| 参数格式错误 | 请求结构不合法 | 只在校验明确未写入且有新契约证据时修正，不要原样重试 |
| 资源不存在 / 404 | ID 错误或资源已删除 | 重新查询定位资源 |
| 配额超限 / 429 | API 调用频率过高 | 等待后重试，并降低并发 |

### 3.3 重试前检查清单

在重试前，先确认：
1. ❓ 错误是暂时性的还是永久性的？
2. ❓ 参数有没有明显错误需要修正？
3. ❓ 上次调用是否可能已经写入？
4. ❓ 是否已经获得对象 ID，应该从后续缺口继续？
5. ❓ 新尝试是否基于新证据，而不是重复同一猜测？

当没有新证据、开始重复同一错误、目标发生变化或副作用无法判断时停止。Base 搜索
失败时，`base list` 只能帮助确认对象存在，不能作为搜索完成证据。

## 4. 调试技巧

### 4.1 使用 --verbose 获取详细信息

```bash
dws aitable record create \
  --base-id <baseId> \
  --table-id <tableId> \
  --records '[...]' \
  --verbose --format json
```

`--verbose` 会输出请求/响应的详细信息，帮助定位问题。

## 5. 错误预防最佳实践

1. **写记录前先读字段结构** — `field get` 确认字段类型和 ID
2. **写字段前先读 field-properties** — 确认 config 的必填项和格式
3. **formula 字段先确认引用字段名** — `[字段名]` 必须精确匹配
4. **options 更新传完整列表** — 更新 singleSelect/multipleSelect 的 options 是全量覆盖
5. **大批量操作分批执行** — 单次最多 100 条记录
6. **使用 --format json** — 确保输出可解析，方便错误判断
