# AI表格 (aitable) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断），各命令的详细参数、示例和踩坑说明在 [aitable/](./aitable/) 目录下按需加载。

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| Base 文档 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` |
| 指定数据表 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}` |
| 指定数据表+视图 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}` |
| 模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` |

> **操作后请返回文档 URI**：返回链接时必须带上当前操作的数据表 tableId，让用户点击后直接看到目标数据表，而不是落在空白的默认表。
> - 已知 tableId + viewId 时（view create 返回、view get 中提取）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}`
> - 已知 tableId 时（table create 返回、base get 中提取、record 操作所用的 tableId）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}`
> - 仅有 baseId、无明确 tableId 时（如 base list/search）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}`
>
> 补充：如果 URL 不是来自 `aitable` 命令返回，而是用户直接贴的原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe，确认是 `able` 后再按 AI 表格处理。

## 命令索引表

### base (Base 管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `base list` | 列出最近访问的 Base | — | 仅返回最近访问过的，优先用 `base search` |
| `base search` | 按名称搜索 Base | `--query` | 关键词 ≥2 字符 |
| `base get` | 获取 Base 信息（含 tables 列表） | `--base-id` | 用户给 URL 时提取末尾 ID |
| `base create` | 创建 Base | `--name` | 创建后直接用返回的 baseId；**默认新建的 base 自带一个空白「数据表」（含 3 行空记录）和一个空白仪表盘**，如需干净的空 base，传 `--template-id 1743` |
| `base update` | 更新 Base 名称 | `--base-id` `--name` | — |
| `base delete` | 删除 Base | `--base-id` | 不可逆 |

### table (数据表管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `table get` | 获取表结构（字段+视图目录） | `--base-id` | 不传 `--table-ids` 返回全部表 |
| `table create` | 创建数据表 | `--base-id` `--name` `--fields` | fields 为 JSON 数组，至少 1 个 |
| `table update` | 修改表名 / 备注 / 行命名规则 | `--base-id` `--table-id` + 三选一(`--name` / `--description` / `--record-name-key`) | `--record-name-key` 是固定枚举（如 task/project/event/customer/ji_lu 等），非字段 ID |
| `table delete` | 删除表 | `--base-id` `--table-id` | 不可逆 |

### field (字段管理) → 详见 [aitable-field.md](./aitable/aitable-field.md)、[field-properties](./aitable/aitable-field-properties.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `field get` | 获取字段完整配置 | `--base-id` `--table-id` | 按需展开少量字段 |
| `field create` | 创建字段 | `--base-id` `--table-id` + (`--name --type` 或 `--fields`) | 支持单字段/批量模式 |
| `field update` | 更新字段名/配置 | `--base-id` `--table-id` `--field-id` | 不可变更字段类型 |
| `field delete` | 删除字段 | `--base-id` `--table-id` `--field-id` | 不可逆 |

#### 搜索字段选项
```
Usage:
  dws aitable field search-options [flags]
Example:
  dws aitable field search-options --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID>
  dws aitable field search-options --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --keyword 已完成
  dws aitable field search-options --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --limit 100
Flags:
      --base-id string    Base ID (必填)
      --field-id string   目标字段 ID，必须是 singleSelect / multipleSelect 类型 (必填)
      --keyword string    模糊搜索关键词，大小写不敏感、contains 匹配 option name；不传返回全部
      --limit int         返回的最大 option 数量，默认 3000（全量），最大 3000
      --table-id string   Table ID (必填)
```

仅适用于 **singleSelect / multipleSelect** 字段。其他类型（text/number/date/...）调用会返回错误。

适用场景：
- options 较多，只想要含某关键词的子集（避免 `field get` 拉取整个字段配置带回所有 options）。
- 写入 record 前预览选项 id ↔ name 的映射，确认要使用的选项确实存在。

> **写 record 时**：`record create / update` 对 singleSelect/multipleSelect 可直接传 option **name**，不需要用本命令。本命令主要用于 **filter** 写法（filters 优先用 option **id**）或选项较多需要精确定位时。

### record (记录管理)

| 命令 | 用途 | 必读 reference | 路由提醒 |
|------|------|----------------|----------|
| `record query` | 查询/搜索记录 | [aitable-record-query.md](./aitable/aitable-record-query.md) | 先 `table get` 拿 fieldId；`--all` 自动翻页；filters 结构见 reference |
| `record get` | 按 ID 取记录（`record query --record-ids` 的窄别名） | [aitable-record-query.md](./aitable/aitable-record-query.md) | 已知 recordId 时首选；必填 `--record-ids`（单次最多 100 条）；未暴露 filters/sort/query/cursor/limit |
| `record create` | 新增记录 | [aitable-record-create.md](./aitable/aitable-record-create.md) | cells key 必须是 fieldId 不是字段名；单次最多 100 条 |
| `record update` | 更新记录（每条独立 cells） | [aitable-record-update.md](./aitable/aitable-record-update.md) | 需先 query 拿 recordId；只传需改字段；`--records` 是 `[{recordId,cells},...]` 数组 |
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
| `view create` | 创建视图 | `--base-id` `--table-id` `--view-type` | 类型: Grid/Kanban/Gantt/Calendar/Gallery/FormDesigner；**Gantt 创建后必须 `view update timebar` 绑定日期字段** |
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
| `workflow list` | 列出 Base 下所有工作流 | `--base-id` | 支持 `--limit [1,100]` / `--offset >=0`；list 出参字段叫 `flowId` |
| `workflow get` | 获取单个工作流详情（含 flowSchema） | `--base-id` `--workflow-id` | `--workflow-id` 接受 list 里的 `flowId`（同值） |
| `workflow enable` | 启用工作流 | `--base-id` `--workflow-id` | 返回 `{enabled: true}` 是动作确认；要确认真启用看 list 的 `status` |
| `workflow disable` | 禁用工作流（高危） | `--base-id` `--workflow-id` `--yes` | 影响业务自动化，建议二次确认；status 变 STOP |

> **当前不支持通过 CLI 新建/修改/删除工作流**，请去 AI 表格 Web 端（数据表页面 → 自动化）配置。

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

用户说"筛选/过滤/filter" → 读 [aitable-filter-sort.md](./aitable/aitable-filter-sort.md)

用户说"统计/分析/聚合/TOP N/全量" → 读 [aitable-data-analysis-sop.md](./aitable/aitable-data-analysis-sop.md)

用户说"公式/formula/计算字段/派生指标" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)

用户说"查找引用/lookup/filterUp/跨表" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)（§5.4 跨表引用）

用户说"表单/form/收集表/问卷/催办填写" → 读 [aitable-form.md](./aitable/aitable-form.md)

用户说"自动化/工作流/流程/触发/automation/workflow" → 读 [aitable-workflow.md](./aitable/aitable-workflow.md)
- 看 Base 里有哪些流程 / 哪些在跑 → `workflow list`（看 `recordCount` / `runningCount`）
- 看某个流程具体配置（触发条件、动作步骤） → `workflow get`
- 启用流程 → `workflow enable`
- 临时停掉流程（调试 / 数据迁移）→ `workflow disable --yes`
- **新建 / 修改 / 删除流程**：当前不支持，引导用户到 AI 表格 Web 端 → 数据表 → 自动化 面板手动完成

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

# 3. 获取表结构 — 提取 fieldId
dws aitable table get --base-id <BASE_ID> --table-id <TABLE_ID> --format json

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
| `table get` | `fields[].fieldId` | record 操作的 cells key, field get/update/delete |
| `record query` | `recordId` | record update/delete；按 ID 反查字段值用 `record get` |
| `template search` | `templateId` | base create --template-id，拼接模板预览 URI |

## URL → baseId 提取

用户提供 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 链接时：
1. 提取 `/nodes/` 后的路径段作为 `baseId`
2. 去掉尾部的查询参数（`?` 及其后内容）
3. 传入 `--base-id` 参数

> 如果该 URL 来自 `dws aitable` 返回或已在当前链路 probe 过，可直接复用；
> 如果是用户直接提供的原始 `alidocs` URL，则先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe，确认 `extension=able` 后再继续。

## 注意事项

- 所有操作使用 ID（baseId/tableId/fieldId/recordId），不使用名称
- records 的 cells key 是 fieldId，不是字段名称
- cells 写入/读取格式见 [aitable-cell-value.md](./aitable/aitable-cell-value.md)
- 最佳实践见 [aitable-best-practices.md](./aitable/aitable-best-practices.md)

## 自动化脚本

| 脚本 | 场景 |
|------|------|
| [bulk_add_fields.py](../../scripts/bulk_add_fields.py) | 批量添加字段 |
| [import_records.py](../../scripts/import_records.py) | 从 JSON/CSV 批量导入记录 |
| [aitable_export_via_task.py](../../scripts/aitable_export_via_task.py) | 文件导出（export_data 轮询 + 下载） |
| [upload_attachment.py](../../scripts/upload_attachment.py) | 上传附件到 AI 表格记录 |

## 相关产品

- [doc](./doc.md) — 富文本文档编辑，不是结构化数据表格
