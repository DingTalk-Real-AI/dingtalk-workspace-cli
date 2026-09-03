# AITable 低频原子能力索引

> 返回入口：[DingTalk AITable Skill](../SKILL.md)

本文件只用于根 Skill 和精确操作 Reference 都未覆盖的低频底层能力。Base/Table 创建、记录 CRUD、筛选排序、视图、导入导出和 Dashboard 等已覆盖能力必须返回根 Skill；本文件不直接导航到其他 AITable Reference。

## 使用边界

1. 只有任务确实需要 Shortcut 未发布的底层字段、原始响应或运维控制时才读取本文件；
2. 若任务已由根 Skill 或某个精确 Reference 覆盖，立即返回根 Skill 重新选路，不在本文件继续导航到另一个 Reference；
3. 已知命令且参数完整时直接执行；只有 leaf 参数或安全语义不确定时才读精确 Schema，只有 Cobra flag 不确定时才读该 leaf Help；
4. 名称或 URL 目标仍必须解析为当前 profile 下的唯一稳定 ID，禁止选择第一个候选；
5. 原子写 leaf 的 confirmation 与对应 Golden Shortcut 不一致时停止，以 Runtime gate 和精确 leaf Schema 为准；
6. 完成后保留稳定 ID、验证证据、partial failure、checkpoint 和真实错误。

## 返回规则

Base、Table、普通 Field、普通 Record、View、Dashboard、筛选排序、导入导出等已覆盖能力全部返回根 Skill；本索引不重复维护高频路由，也不作为 Reference 之间的中转站。

## 低频底层命令族

下表只是最后回退的导航，不是可预加载的命令目录。命中后若参数或安全语义仍不确定，才读取精确 leaf Schema。

| 原子命令或命令族 | 仅用于 |
|---|---|
| `base get-primary-doc-id` | 统一 Base/Record 路由未投影所需底层主文档 ID 时 |
| `record get` | 必须获取单条记录原始响应，且 `+record-query --record-ids` 不能交付所需字段时 |
| `field search-options` | 只需在已知选项字段中搜索选项，不需要完整字段配置时 |

## 低频批处理脚本

只有根 Skill 已定位到对应低频任务、且原生命令需要重复编排时才使用；脚本参数以 `--help` 和脚本内校验为准，不因脚本存在而跳过目标解析、确认或结果验证。

| 脚本 | 仅用于 |
|---|---|
| `python scripts/aitable_export_via_task.py <baseId> --scope all` | 已由根 Skill 选中导出任务，需要轮询 `taskId` 并下载结果时；表或视图范围使用稳定 `tableId` / `viewId` |
| `python scripts/bulk_add_fields.py <baseId> <tableId> fields.json` | 已完成字段类型与配置校验后批量创建大量字段；少量普通字段走根 Skill 次级直达 |

## 稳定 ID 传递

| 来源 | 只可用于 |
|---|---|
| `+url-resolve` / `+resolve-base` / `+base-search` 唯一结果 | 当前 profile 下的 `baseId` |
| `+resolve-table` / `+list-tables` | 当前 Base 下的 `tableId` |
| `field list` / `+field-get` | 当前 Table 下的 `fieldId` |
| `record create` / `+record-query` | 当前 Table 下的 `recordId` |
| `view create` / `+view-get` | 当前 Table 下的 `viewId` |
| Dashboard 或 Chart 创建结果 | 当前 Base 下的 `dashboardId` / `chartId` |

`baseId` / `tableId` / `fieldId` / `recordId` / `viewId` 是不同类型，不得轮流代入试错。Base 复制目标按根 Skill 的 Golden Route 解析。

## 故障处理

- `unknown command` / `unknown flag`：读取精确 leaf Help，最多做一次有证据的修正；
- confirmation 或参数约束不清：读取精确 leaf Schema，以 Runtime gate 为准；
- `partial_success`：保留已完成项和 checkpoint，只执行结果给出的继续或恢复命令；
- 写入结果为 `unknown`：先按稳定 ID 或业务唯一键回读，未确认前不重试非幂等写；
- `retryable=false` 或 ID 类型错误：停止，不换同义原子命令或其他 ID 类型试错；
- 部分成功：保留 completed/failed/unknown 明细，不表述为完整成功。
