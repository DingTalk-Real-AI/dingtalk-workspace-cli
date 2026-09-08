# AITable 低频原子能力索引

> 返回入口：[DingTalk AITable Skill](../SKILL.md)

本文件只用于根 Skill 和精确操作 Reference 都未覆盖的低频底层能力。Base/Table 创建、应用模式、记录 CRUD、筛选排序、视图、导入导出和 Dashboard 等已覆盖能力必须返回根 Skill；本文件不直接导航到其他 AITable Reference。

## 使用边界

1. 只有任务确实需要 Shortcut 未发布的底层字段、原始响应或运维控制时才读取本文件；
2. 若任务已由根 Skill 或某个精确 Reference 覆盖，立即返回根 Skill 重新选路，不在本文件继续导航到另一个 Reference；
3. 已知命令且参数完整时直接执行；只有 leaf 参数或安全语义不确定时才读精确 Schema，只有 Cobra flag 不确定时才读该 leaf Help；
4. 名称或 URL 目标仍必须解析为当前 profile 下的唯一稳定 ID，禁止选择第一个候选；
5. 原子写 leaf 的 confirmation 与对应 Golden Shortcut 不一致时停止，以 Runtime gate 和精确 leaf Schema 为准；
6. 完成后保留稳定 ID、验证证据、partial failure、checkpoint 和真实错误。

## 返回规则

Base、Table、应用模式 App/Page/Widget、普通 Field、普通 Record、View、Dashboard、筛选排序、导入导出等已覆盖能力全部返回根 Skill；本索引不重复维护高频路由，也不作为 Reference 之间的中转站。

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
| `app get` / `app page list` / `app widget list` | 当前 Base 下的 `appId` / `pageId` / `widgetId`；应用页面 `pageId` 同时是对应 Dashboard ID |

`baseId` / `tableId` / `fieldId` / `recordId` / `viewId` / `appId` / `pageId` / `widgetId` 是不同类型，不得轮流代入试错；唯一例外是应用页面 `pageId` 与其对应 Dashboard ID 同值。Base 复制目标按根 Skill 的 Golden Route 解析。

> **写 record 时**：`record create / update` 对 singleSelect/multipleSelect 传 option **name**。**做 filter 时**：先用本命令或 `field get` 将用户输入唯一解析到现有选项，优先传稳定 option **id**；不要直接透传模糊名称。

### psql (PostgreSQL 只读查询) → 详见 [aitable-psql.md](./aitable/aitable-psql.md)

| 命令模式 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `psql -l` | 列出可查询的 PostgreSQL 逻辑表 | `-d <baseId>` | 数据查询前的表发现；输出为 psql 文本，不加 `--format json` |
| `psql -t` | 查看逻辑列名和 PostgreSQL 类型 | `-d <baseId>` `-t <tableId>` | SQL 前核对列；全部属性列加 `--all-properties` |
| `psql -c` | 执行一条只读 PostgreSQL SELECT | `-d <baseId>` `-c <SQL>` | SQL 的 `FROM` / `JOIN` 自动确定主表；支持单表及同 Base 多表 JOIN；只读；输出为 psql 文本 |

## 故障处理

- `unknown command` / `unknown flag`：读取精确 leaf Help，最多做一次有证据的修正；
- confirmation 或参数约束不清：读取精确 leaf Schema，以 Runtime gate 为准；
- `partial_success`：保留已完成项和 checkpoint，只执行结果给出的继续或恢复命令；
- 写入结果为 `unknown`：先按稳定 ID 或业务唯一键回读，未确认前不重试非幂等写；
- `retryable=false` 或 ID 类型错误：停止，不换同义原子命令或其他 ID 类型试错；
- 部分成功：保留 completed/failed/unknown 明细，不表述为完整成功。
| 命令 | 用途 | 必读 reference | 路由提醒 |
|------|------|----------------|----------|
| `record query` | 查询/搜索记录 | [aitable-record-query.md](./aitable/aitable-record-query.md) | 先 `field get` 拿 fieldId 与类型；完整结果必须用 `--all --page-limit 0` 自动翻页；filters 中的实体展示名须先解析为稳定 ID/结构化值 |
| `record get` | 按 ID 取记录（`record query --record-ids` 的窄别名） | [aitable-record-query.md](./aitable/aitable-record-query.md) | 已知 recordId 时首选；必填 `--record-ids`（单次最多 100 条）；未暴露 filters/sort/query/cursor/limit |
| `record create` | 新增记录 | [aitable-record-create.md](./aitable/aitable-record-create.md) | cells key 必须是 fieldId 不是字段名；单次最多 100 条 |
| `record update` | 更新记录（每条独立 cells） | [aitable-record-update.md](./aitable/aitable-record-update.md) | 需先 query 拿 recordId；`cells` key 支持 fieldId 或当前表内唯一字段名，推荐 fieldId；`--records` 是 `[{recordId,cells},...]` 数组 |
| `record batch-update` | 批量更新（同一 cells 应用到多条 recordId） | [aitable-record-update.md](./aitable/aitable-record-update.md)、[aitable-cell-value.md](./aitable/aitable-cell-value.md) | 适合"统一标记完成/统一改负责人"等共享 patch 场景；`--cells` 是 JSON object（key=fieldId，value 按字段类型见 cell-value.md），与 record update 的单条 cells 结构完全一致；必填 `--record-ids` `--cells`；单次最多 100 条 |
| `record delete` | 删除记录 | [aitable-record-delete.md](./aitable/aitable-record-delete.md) | 不可逆，需先 query 确认 |
| `record history-list` | 查询单条记录的变更历史 | [aitable-record-history.md](./aitable/aitable-record-history.md) | 必填 `--record-id`；分页 `--offset --limit`，limit 范围 [1,50] 默认 20 |
| `record query-empty` | 查询完全没填用户字段的空行 | [aitable-record-query.md](./aitable/aitable-record-query.md) | 一页扫描 `--limit` [1,100] 默认 100；扫完前需用 `--cursor` 翻页（nextCursor 为空才表扫完） |
| `record share-url` | 批量获取记录分享链接 | [aitable-record-share.md](./aitable/aitable-record-share.md) | 必填 `--record-ids`（CSV，单次最多 20 条）；可选 `--view-id` 带视图上下文 |
| `record upsert` | 批量创建或更新（按 recordId 是否存在自动拆分） | [aitable-record-upsert.md](./aitable/aitable-record-upsert.md) | --records 同 record update 格式；带 recordId 走 update，不带走 create；单次最多 100 |
| `record primary-doc-get` | 查询记录的主键文档 nodeId | [aitable-primary-doc.md](./aitable/aitable-primary-doc.md) | 返回的 nodeId 可直接用于 `dws doc read/update --node` |
| `record primary-doc-create` | 为记录创建主键文档（幂等） | [aitable-primary-doc.md](./aitable/aitable-primary-doc.md) | fieldId 必须是 primaryDoc 类型；已存在则返回已有 nodeId |

### view (视图管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `view get` | 获取视图配置（不传子命令） | `--base-id` `--table-id` | 不传 `--view-ids` 返回全部视图 |
| `view get <attr>` | 获取视图某个属性 | `--view-id` | 12 个：card/timebar/aggregate/filter/sort/group/visible-fields/field-widths（详见 [aitable-view-config.md](./aitable/aitable-view-config.md)）+ lock/frozen-cols/row-height/fill-color-rule（详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)） |
| `view list` | 列出全部视图（`view get` 的别名） | `--base-id` `--table-id` | 与 `view get` 完全等价 |
| `view create` | 创建视图 | `--base-id` `--table-id` `--view-type` | 类型: Grid/Kanban/Gantt/Calendar/Gallery/FormDesigner；用 `--config` 传 visibleFieldIds/filter/sort/group；**Gantt 创建后必须 `view update timebar` 绑定日期字段** |
| `view update` | 整体更新视图 / 多属性合并更新 | `--base-id` `--table-id` `--view-id` | 可传 `--name --desc --config '{...}'`，**`--config` 路径继续保留** |
| `view update <attr>` | 按属性局部更新（推荐）| `--view-id` + typed flag / `--json` | 12 个：card/timebar/aggregate/field-widths/visible-fields/filter/sort/group/name + frozen-cols/row-height/fill-color-rule |
| `view lock [--off]` | 锁定/解锁视图 | `--base-id` `--table-id` `--view-id` | 默认锁定；`--off` 解锁。详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md) |
| `view duplicate` | 复制视图 | `--base-id` `--table-id` `--view-id` | 可选 `--new-name`；保留源视图全部配置。详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md) |
| `view delete` | 删除视图 | `--base-id` `--table-id` `--view-id` | 不可删最后一个/锁定视图 |

> **优先用 `view get <attr>` / `view update <attr>` 子命令**：每个属性独立命令，typed flag 友好，agent 不必拼 JSON。**`view update --config '{...}'` 仍可用**，适合一次性多属性更新或脚本场景。

> **属性按 attr 分类，决定该读哪份子文档**：
> - card / timebar / aggregate / filter / sort / group / visible-fields / field-widths → [aitable-view-config.md](./aitable/aitable-view-config.md)
> - lock / frozen-cols / row-height / fill-color-rule / duplicate → [aitable-view-extras.md](./aitable/aitable-view-extras.md)
> 后一类**不能**塞进 `view update --config '{...}'`，必须用各自专属子命令；如果错传 `flags` / `frozenColCount` / `cellHeight` / `conditionalFormats` 等 key 进 `--config`，CLI 会在 stderr 提示应改用的命令。

> **`view update --config` 支持的 9 个 key**：
> `visibleFieldIds` / `filter` / `sort` / `group` / `fieldWidths`(Grid) / `aggregate`(Grid) / `kanbanCard`(Kanban) / `ganttTimebar`(Gantt) / `galleryCard`(Gallery)。
> filter/sort/group 必须传**数组**格式（与 `record query --filters` 的对象格式不同；CLI 会自动容错）。其他 key 会被服务端忽略并打 warning。

### form (表单管理) → 详见 [aitable-form.md](./aitable/aitable-form.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `form list` | 列出表单视图 | `--base-id` `--table-id` | 详情见 [aitable-form.md](./aitable/aitable-form.md) |
| `form get` | 按 viewId 取单个表单详情 | `--base-id` `--table-id` `--view-id` | — |
| `form create` | 创建表单视图 | `--base-id` `--table-id` `--name` | — |
| `form update` | 更新表单配置 | `--base-id` `--table-id` `--view-id` | title/name/description 至少一项 |
| `form delete` | 删除表单 | `--base-id` `--table-id` `--view-id` | 不可逆 |
| `form field list/update/hide` | 表单字段管理 | — | 详情见子文档 |
| `form questions create/delete` | 题目管理（=field create/delete） | — | 详情见子文档 |
| `form share get/update` | 表单分享配置 | — | 详情见子文档 |

> **创建表单**有两种等价方式：`form create --name "..."`（推荐）或 `view create --view-type FormDesigner --name "..."`。

### workflow (自动化工作流) → 详见 [aitable-workflow.md](./aitable/aitable-workflow.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `workflow edit-example` | 获取编辑文档与 DSL 示例 | 无 | create/update 前优先调用，内容由服务端提供 |
| `workflow create` | 创建并发布自动化工作流 | `--base-id` `--dsl` | 按子文档 Demo 组装 DSL；必须检查返回的 `data.valid` / `issues`；create 不自动重试 |
| `workflow update` | 更新并发布已有自动化工作流 | `--base-id` `--workflow-id` `--dsl` | 先 get 留底；提交完整目标 DSL；必须检查 `data.valid` / `issues` |
| `workflow list` | 列出 Base 下所有工作流 | `--base-id` | 支持 `--limit [1,100]` / `--offset >=0`；list 出参字段叫 `flowId` |
| `workflow get` | 获取单个工作流详情（含 flowSchema） | `--base-id` `--workflow-id` | `--workflow-id` 接受 list 里的 `flowId`（同值） |
| `workflow enable` | 启用工作流 | `--base-id` `--workflow-id` | 返回 `{enabled: true}` 是动作确认；要确认真启用看 list 的 `status` |
| `workflow disable` | 禁用工作流（高危） | `--base-id` `--workflow-id` `--yes` | 影响业务自动化，建议二次确认；status 变 STOP |

> 创建/更新的 `--dsl` 使用钉钉 AI 表格 `workflow-dsl/v1`；完整格式和最小 Demo 见 [aitable-workflow.md](./aitable/aitable-workflow.md)。删除工作流暂未开放。

### dashboard & chart → 详见 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)

| 命令 | 用途 |
|------|------|
| `dashboard get/create/update/delete` | 仪表盘管理 |
| `dashboard config-example` | 查看仪表盘配置模板 |
| `dashboard arrange` | 自动重排仪表盘图表布局（智能填满网格，避免空缺） |
| `chart get/create/update/delete` | 图表管理 |
| `chart widgets-example` | 查看图表 widgets 配置模板 |

### export & import → 详见 [aitable-export-import.md](./aitable/aitable-export-import.md)

| 命令 | 用途 |
|------|------|
| `export data` | 导出数据（异步两阶段轮询） |
| `import upload` | 申请文件导入上传凭证 |
| `import data` | 触发导入 |

### attachment → 详见 [aitable-attachment.md](./aitable/aitable-attachment.md)

| 命令 | 用途 | 路由提醒 |
|------|------|----------|
| `attachment upload` | 准备附件上传凭证 | 不要用钉盘 drive 上传！ |

### template (模板搜索)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `template search` | 搜索模板 | `--query` |

### advperm (高级权限/自定义角色) → 详见 [aitable-advperm.md](./aitable/aitable-advperm.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `advperm enable` | 开启 Base 高级权限总开关 | `--base-id` | 不开启时角色规则不生效 |
| `advperm disable` | 关闭 Base 高级权限总开关（高危） | `--base-id` `--yes` | 关闭后全员回退默认权限 |
| `advperm role-list` | 列出 Base 下所有角色 | `--base-id` | 同时返回自定义角色和系统角色；`roleType == "custom"` 是自定义，前缀 `system_` 是系统角色 |
| `advperm role-get` | 获取单角色完整配置 | `--base-id` `--role-id` | 含 subRoles 与字段/行级规则 |
| `advperm role-create` | 创建自定义角色 | `--base-id` `--name` | 可选 `--sub-roles` 同时指定子角色权限规则 |
| `advperm role-update` | 增量更新自定义角色（PATCH） | `--base-id` `--role-id` | 未传字段不变；`--sub-roles` 按 (targetId,targetType) 合并 |
| `advperm role-delete` | 删除自定义角色 | `--base-id` `--role-id` `--yes` | 不可逆；系统角色禁删；**调用者必须是该 AI 表格的管理员/Owner**，非管理员会得到 401 AUTH_ERROR |

> **角色 CRUD 已全支持**：create/get/list/update/delete 都可走 CLI。
> 所有写命令（enable/disable/role-create/role-update/role-delete）需要 Base 管理员权限；非管理员只能调 `role-list` / `role-get`（只读）。
> "角色 ↔ 成员"绑定当前 CLI 不支持，仍需在 AI 表格 Web 端 → Base 设置 → 高级权限面板手动完成。

### section (文件夹与节点管理)

> 用于在 Base 的导航树中组织 table / dashboard / 表单视图 / 文档等节点（类似文件夹）。
> 操作前建议先用 `section list-nodes` 拿到 nodeId / sectionId 与父级关系。

#### 创建文件夹
```
Usage:
  dws aitable section create [flags]
Example:
  dws aitable section create --base-id <BASE_ID> --name 我的文件夹
  dws aitable section create --base-id <BASE_ID> --name 子文件夹 --parent-section-id <SECTION_ID> --index 0
Flags:
      --base-id string             Base ID (必填)
      --name string                文件夹名称 (必填)
      --parent-section-id string   父文件夹 ID；不传或空字符串表示创建在 Base 根目录下
      --index int                  在父文件夹下的目标位置（0-based）；不传则追加到末尾
```

返回 `data.sectionId` 与 `data.name`。

#### 重命名文件夹
```
Usage:
  dws aitable section rename [flags]
Example:
  dws aitable section rename --base-id <BASE_ID> --section-id <SECTION_ID> --new-name 新名称
Flags:
      --base-id string      Base ID (必填)
      --section-id string   目标文件夹 ID (必填)
      --new-name string     新的文件夹名称 (必填)
```

#### 删除文件夹
```
Usage:
  dws aitable section delete [flags]
Example:
  dws aitable section delete --base-id <BASE_ID> --section-id <SECTION_ID>
Flags:
      --base-id string      Base ID (必填)
      --section-id string   目标文件夹 ID (必填)
```

> **注意**：删除不可逆；删除前可先用 `section list-empty` 确认是否为空文件夹。

#### 调整文件夹顺序
```
Usage:
  dws aitable section reorder [flags]
Example:
  dws aitable section reorder --base-id <BASE_ID> --section-id <SECTION_ID> --target-index 0
Flags:
      --base-id string      Base ID (必填)
      --section-id string   目标文件夹 ID (必填)
      --target-index int    目标位置（0-based）(必填)
```

> 在**当前父文件夹下**调整展示顺序。跨父级移动请用 `section move-node`。

#### 列出空文件夹
```
Usage:
  dws aitable section list-empty [flags]
Example:
  dws aitable section list-empty --base-id <BASE_ID>
Flags:
      --base-id string   Base ID (必填)
```

返回 `data.items: [{sectionId, name, parentSectionId}]` 与 `data.total`，用于清理或诊断导航树（parentSectionId 为空串表示在根目录下）。

#### 列出全部节点
```
Usage:
  dws aitable section list-nodes [flags]
Example:
  dws aitable section list-nodes --base-id <BASE_ID>
Flags:
      --base-id string   Base ID (必填)
```

返回 `data.items: [{nodeId, nodeType, parentSectionId, name?}]` 与 `data.total`，涵盖文件夹 / AI 表格 / 表单视图 / 仪表盘 / 文档 / 查询视图。

> **与其他命令的关联**：是 `section move-node` / `section reorder` 的前置定位命令——先用它拿到 nodeId 与 parentSectionId。

#### 移动节点
```
Usage:
  dws aitable section move-node [flags]
Example:
  dws aitable section move-node --base-id <BASE_ID> --node-id <NODE_ID> --new-parent-section-id <SECTION_ID>
  dws aitable section move-node --base-id <BASE_ID> --node-id <NODE_ID> --new-parent-section-id "" --target-index 0
Flags:
      --base-id string                 Base ID (必填)
      --node-id string                 要移动的节点 ID（文件夹/AI表格/表单视图/仪表盘/文档/查询视图）(必填)
      --new-parent-section-id string   目标父文件夹 ID；空字符串表示移到 Base 根目录 (必填)
      --target-index int               Base 内节点的全局位置（0-based）；不传则不调整
```

> 服务端自动识别节点类型，无需区分文件夹与非文件夹。返回 `data.nodeId / newParentSectionId / nodeType`。
> 对文件夹节点带 `--target-index` 时会先 move 再 reorder，中间失败会返回 `MOVE_OK_REORDER_FAILED`，可用 `section reorder` 重试。

## 复杂操作

### 仪表盘 / 图表（建议顺序）

```bash
# 1) 先看配置模板（JSONC）
dws aitable dashboard config-example --format json
dws aitable chart widgets-example --format json

# 2) 先拿 dashboard，再拿 chart 详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

要点：

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用。
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错。
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断。

### 导出数据（两阶段轮询）

`export data` 常见为异步任务：首次调用可能只返回 `taskId`，需要继续轮询。

```bash
# 第一步：创建任务（按 scope 传必要参数）
dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --format excel --timeout-ms 1000

# 第二步：拿 taskId 继续轮询，直到返回 downloadUrl
dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000
```

参数约束

- `scope=all`：只需 `base-id`
- `scope=table`：必须 `table-id`
- `scope=view`：必须同时 `table-id + view-id`

## 意图判断

用户说"表格/多维表/AI表格":
- 查看/查找/列表 → `base search`（优先）或 `base list`（仅浏览最近访问）
- 详情 → `base get`
- 创建 → `base create`
- 修改 → `base update`
- 删除 → `base delete`

用户说"数据表/子表/table":
- 查看 → `table get`
- 创建 → `table create`
- 重命名 / 改备注 / 改行命名规则 → `table update`（三选一：`--name` / `--description` / `--record-name-key`）
- 用户说"行命名规则/记录别名/卡片显示成 task/project/event 这种" → `table update --record-name-key <枚举键>`，**中文 → 枚举键**对照见 [aitable-record-name-key.md](./aitable/aitable-record-name-key.md)
- 删除 → `table delete`

用户说"字段/列/column":
- 查看 → `field get`
- 添加 → `field create`（读 [aitable-field.md](./aitable/aitable-field.md)）
- 修改 → `field update`
- 删除 → `field delete`

用户说"记录/行/数据/row":
- 查看/搜索 → `record query`（读 [aitable-record-query.md](./aitable/aitable-record-query.md)）
- 找空行 / 没填东西的行 → `record query-empty`（读 [aitable-record-query.md](./aitable/aitable-record-query.md)）
- 已知 recordId 反查字段值 → `record get`（按 ID 取专用，等价 `record query --record-ids`）
- 添加/写入 → `record create`（读 [aitable-record-create.md](./aitable/aitable-record-create.md)）
- 修改/更新（每条独立 cells） → `record update`（读 [aitable-record-update.md](./aitable/aitable-record-update.md)）
- **批量更新同一字段值**（统一标记/统一改值） → `record batch-update --record-ids ... --cells '{...}'`
- 删除 → `record delete`
- **查记录的字段变更历史 / 操作审计** → `record history-list`（读 [aitable-record-history.md](./aitable/aitable-record-history.md)）
- **取记录分享链接 / 把这行发给同事** → `record share-url`（读 [aitable-record-share.md](./aitable/aitable-record-share.md)）
- **不知道有没有 → 有就改、没有就建** → `record upsert`（读 [aitable-record-upsert.md](./aitable/aitable-record-upsert.md)）

用户说"视图/view":
- 列出/查看全部视图 → `view list`（或 `view get` 不传 --view-ids，二者等价）
- 看某个视图详情 → `view get --view-ids <ID>`
- 创建 → `view create`
- 修改（含"调整字段顺序/隐藏字段"） → `view update --config '{"visibleFieldIds":[...]}'`
- 修改某一项配置（filter/sort/group/card/timebar/aggregate 等）→ `view update <attr>`（读 [aitable-view-config.md](./aitable/aitable-view-config.md)）
- 锁定 / 冻结列 / 行高 / 数据高亮规则 / 复制视图 → 读 [aitable-view-extras.md](./aitable/aitable-view-extras.md)
- 删除 → `view delete`

用户说"锁定视图/解锁视图/lock view" → `view lock` / `view lock --off`，详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)

用户说"冻结列/冻结首列/frozen columns" → `view update frozen-cols --count N`，详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)

用户说"行高/单元格高度/紧凑模式/cell height" → `view update row-height --cell-height N`（合法档位 32/56/88/128），详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)

用户说"数据高亮/条件格式/单元格上色/fill color rule" → `view update fill-color-rule --json '[...]'`，详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)

用户说"复制视图/duplicate view" → `view duplicate --view-id ... [--new-name ...]`，详见 [aitable-view-extras.md](./aitable/aitable-view-extras.md)

用户说"查看可查询表/SQL 表结构/SQL 字段类型/查询前 N 条/按列查询/SQL/PostgreSQL/SELECT/JOIN/两表关联" → 读 [aitable-psql.md](./aitable/aitable-psql.md)。数据查询前的表清单和逻辑列发现走 `psql`；普通按记录 ID、关键词或 filters 查行仍走 `record query`；两表关联不得误路由为 LOOKUP/FILTER_UP 公式配置。

用户说"筛选/过滤/filter" → 读 [aitable-filter-sort.md](./aitable/aitable-filter-sort.md)

用户说"统计/分析/聚合/TOP N/全量" → 读 [aitable-data-analysis-sop.md](./aitable/aitable-data-analysis-sop.md)

用户说"公式/formula/计算字段/派生指标" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)

用户说"查找引用/lookup/filterUp/跨表" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)（§5.4 跨表引用）

用户说"表单/form/收集表/问卷/催办填写" → 读 [aitable-form.md](./aitable/aitable-form.md)

用户说"自动化/工作流/流程/触发/automation/workflow" → 读 [aitable-workflow.md](./aitable/aitable-workflow.md)
- 新建自动化 → 按子文档的最小 Demo 组装完整 DSL，再 `workflow create --dsl @file`
- 修改自动化 → `workflow get` 留底，按最新 DSL 文档生成完整目标 DSL，再 `workflow update --dsl @file`
- 看 Base 里有哪些流程 / 哪些在跑 → `workflow list`（看 `recordCount` / `runningCount`）
- 看某个流程具体配置（触发条件、动作步骤） → `workflow get`
- 启用流程 → `workflow enable`
- 临时停掉流程（调试 / 数据迁移）→ `workflow disable --yes`
- 删除流程：当前不支持，引导用户到 AI 表格 Web 端 → 数据表 → 自动化 面板手动完成

用户说"仪表盘/图表/chart" → 读 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)

用户说"仪表盘排版乱了/图表对不齐/重新排布/自动布局/美化仪表盘" → `dashboard arrange`（读 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)）

用户说"附件/上传文件" → 读 [aitable-attachment.md](./aitable/aitable-attachment.md)

用户说"导入/导出/import/export" → 读 [aitable-export-import.md](./aitable/aitable-export-import.md)

用户说"模板" → `template search`

用户说"高级权限/角色/权限控制/谁能看/谁能改" → 读 [aitable-advperm.md](./aitable/aitable-advperm.md)
- 开/关高级权限 → `advperm enable` / `advperm disable --yes`
- 看角色配置 → `advperm role-list` 或 `advperm role-get`
- 建角色（可同时指定子角色权限） → `advperm role-create --name ... --sub-roles '[...]'`
- 改角色名 / 改子角色权限（PATCH 语义，未传字段不变） → `advperm role-update --role-id ... [--name ...] [--sub-roles '[...]']`
- 删角色 → `advperm role-delete --yes`
- **角色 ↔ 成员绑定**：当前 CLI 不支持，仍需在 AI 表格 Web 端面板手动完成

命令报错/操作失败 → 读 [aitable-error-recovery.md](./aitable/aitable-error-recovery.md)

**关键区分**: base=表格文件, table=数据表, field=列, record=行

## 核心工作流

```bash
# 1. 搜索/列出 Base — 提取 baseId
dws aitable base search --query "项目" --format json

# 2. 获取 Base 信息 — 提取 tableId
dws aitable base get --base-id <BASE_ID> --format json

# 3. 获取字段目录 — 提取 fieldId
dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --format json

# 4. 查询记录
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --format json

# 5. 新增记录 (cells 用 fieldId 作 key)
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldXXX":"值"}}]' --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `base list/search` | `baseId` | 所有后续命令的 --base-id，拼接文档 URI |
| `base create` | `baseId` | 后续命令 + 文档 URI |
| `base get` | `tables[].tableId` | --table-id，拼接指定数据表 URI |
| `table create` | `tableId` | 后续命令 + 拼接指定数据表 URI |
| `table get` | `tables[].tableId`、视图目录 | 定位数据表和视图；字段需继续调用 `field get` |
| `field get` | `fields[].fieldId` | record 操作的 cells key, field update/delete |
| `record query` | `recordId` | record update/delete；按 ID 反查字段值用 `record get` |
| `template search` | `templateId` | base create --template-id，拼接模板预览 URI |

## URL → baseId 提取

用户提供 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 链接时：
1. 提取 `/nodes/` 后的路径段作为 `baseId`
2. 去掉尾部的查询参数（`?` 及其后内容）
3. 传入 `--base-id` 参数

> 如果该 URL 来自 `dws aitable` 返回或已在当前链路 probe 过，可直接复用；
> 如果是用户直接提供的原始 `alidocs` URL，则先按 [链接规范](url-patterns.md#alidocs-url-类型探测流程) probe，确认 `extension=able` 后再继续。

## 注意事项

- 所有管理和记录写入操作使用 ID（baseId/tableId/fieldId/recordId），不使用名称；`psql` 的用户 SQL 使用 `psql -l/-t` 返回的真实逻辑表名和列名
- `aitable psql` 输出 PostgreSQL 表格文本，不支持也不添加 `--format json`；其他结构化读取仍按全局规则使用 `--format json`
- records 的 cells key 是 fieldId，不是字段名称
- cells 写入/读取格式见 [aitable-cell-value.md](./aitable/aitable-cell-value.md)
- 最佳实践见 [aitable-best-practices.md](./aitable/aitable-best-practices.md)

## 自动化脚本

| 脚本 | 场景 |
|------|------|
| [bulk_add_fields.py](../scripts/bulk_add_fields.py) | 批量添加字段 |
| [import_records.py](../scripts/import_records.py) | 从 JSON/CSV 批量导入记录 |
| [aitable_export_via_task.py](../scripts/aitable_export_via_task.py) | 文件导出（export_data 轮询 + 下载） |
| [upload_attachment.py](../scripts/upload_attachment.py) | 上传附件到 AI 表格记录 |

## 相关产品

- [doc](../../dingtalk-doc/references/doc.md) — 富文本文档编辑，不是结构化数据表格
