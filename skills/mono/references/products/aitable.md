# AI表格 (aitable) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断），各命令的详细参数、示例和踩坑说明在 [aitable/](./aitable/) 目录下按需加载。

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| Base 文档 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` |
| 模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` |

> **操作后请返回文档 URI**：每次执行 base list/search/create/get 操作后，从返回数据中提取 `baseId`，拼接为 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 返回给用户。
> 补充：如果 URL 不是来自 `aitable` 命令返回，而是用户直接贴的原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe，确认是 `able` 后再按 AI 表格处理。

## 命令索引表

### base (Base 管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `base list` | 列出最近访问的 Base | — | 仅返回最近访问过的，优先用 `base search` |
| `base search` | 按名称搜索 Base | `--query` | 关键词 ≥2 字符 |
| `base get` | 获取 Base 信息（含 tables 列表） | `--base-id` | 用户给 URL 时提取末尾 ID |
| `base create` | 创建 Base | `--name` | 创建后直接用返回的 baseId |
| `base update` | 更新 Base 名称 | `--base-id` `--name` | — |
| `base delete` | 删除 Base | `--base-id` | 不可逆 |

### table (数据表管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `table get` | 获取表结构（字段+视图目录） | `--base-id` | 不传 `--table-ids` 返回全部表 |
| `table create` | 创建数据表 | `--base-id` `--name` `--fields` | fields 为 JSON 数组，至少 1 个 |
| `table update` | 重命名表 | `--base-id` `--table-id` `--name` | — |
| `table delete` | 删除表 | `--base-id` `--table-id` | 不可逆 |

### field (字段管理) → 详见 [aitable-field.md](./aitable/aitable-field.md)、[field-properties](./aitable/aitable-field-properties.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `field get` | 获取字段完整配置 | `--base-id` `--table-id` | 按需展开少量字段 |
| `field create` | 创建字段 | `--base-id` `--table-id` + (`--name --type` 或 `--fields`) | 支持单字段/批量模式 |
| `field update` | 更新字段名/配置 | `--base-id` `--table-id` `--field-id` | 不可变更字段类型 |
| `field delete` | 删除字段 | `--base-id` `--table-id` `--field-id` | 不可逆 |

### record (记录管理)

| 命令 | 用途 | 必读 reference | 路由提醒 |
|------|------|----------------|----------|
| `record query` | 查询/搜索记录 | [aitable-record-query.md](./aitable/aitable-record-query.md) | 先 `table get` 拿 fieldId；`--all` 自动翻页；filters 结构见 reference |
| `record get` | 按 ID 取记录（`record query --record-ids` 的窄别名） | [aitable-record-query.md](./aitable/aitable-record-query.md) | 已知 recordId 时首选；必填 `--record-ids`（单次最多 100 条）；未暴露 filters/sort/query/cursor/limit |
| `record create` | 新增记录 | [aitable-record-create.md](./aitable/aitable-record-create.md) | cells key 必须是 fieldId 不是字段名；单次最多 100 条 |
| `record update` | 更新记录 | [aitable-record-update.md](./aitable/aitable-record-update.md) | 需先 query 拿 recordId；只传需改字段；**没有** `--record-id` `--cells` flag |
| `record delete` | 删除记录 | [aitable-record-delete.md](./aitable/aitable-record-delete.md) | 不可逆，需先 query 确认 |

### view (视图管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `view get` | 获取视图配置 | `--base-id` `--table-id` | 不传 `--view-ids` 返回全部视图 |
| `view create` | 创建视图 | `--base-id` `--table-id` `--view-type` | 类型: Grid/Kanban/Gantt/Calendar/Gallery/FormDesigner |
| `view update` | 更新视图（**调整字段顺序的入口**） | `--base-id` `--table-id` `--view-id` | `visibleFieldIds` 重排字段顺序 |
| `view delete` | 删除视图 | `--base-id` `--table-id` `--view-id` | 不可删最后一个/锁定视图 |

> **"移动字段/调整字段顺序"** 在 AI 表格里没有 `field reorder` 命令，必须通过 `view update --config '{"visibleFieldIds":[...]}'` 完成。

> **view update --config 支持的 key 白名单**（传入其他 key 会报错）：
> - `visibleFieldIds` — 视图可见字段列表及顺序（首列字段必须保留在第一位）
> - `filter` — 筛选规则列表
> - `sort` — 排序规则列表
> - `group` — 分组规则列表
> - `fieldWidths` — 列宽映射（仅 Grid 视图有效）
>
> 不支持 `formInfo`、`requiredFields`、`conditionalRules` 等 FormDesigner 高级配置，这些 key 会被服务端忽略。

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
- 重命名 → `table update`
- 删除 → `table delete`

用户说"字段/列/column":
- 查看 → `field get`
- 添加 → `field create`（读 [aitable-field.md](./aitable/aitable-field.md)）
- 修改 → `field update`
- 删除 → `field delete`

用户说"记录/行/数据/row":
- 查看/搜索 → `record query`（读 [aitable-record-query.md](./aitable/aitable-record-query.md)）
- 已知 recordId 反查字段值 → `record get`（按 ID 取专用，等价 `record query --record-ids`）
- 添加/写入 → `record create`（读 [aitable-record-create.md](./aitable/aitable-record-create.md)）
- 修改/更新 → `record update`（读 [aitable-record-update.md](./aitable/aitable-record-update.md)）
- 删除 → `record delete`

用户说"筛选/过滤/filter" → 读 [aitable-filter-sort.md](./aitable/aitable-filter-sort.md)

用户说"统计/分析/聚合/TOP N/全量" → 读 [aitable-data-analysis-sop.md](./aitable/aitable-data-analysis-sop.md)

用户说"公式/formula/计算字段/派生指标" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)

用户说"查找引用/lookup/filterUp/跨表" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)（§5.4 跨表引用）

用户说"仪表盘/图表/chart" → 读 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)

用户说"附件/上传文件" → 读 [aitable-attachment.md](./aitable/aitable-attachment.md)

用户说"导入/导出/import/export" → 读 [aitable-export-import.md](./aitable/aitable-export-import.md)

用户说"模板" → `template search`

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
| `base get` | `tables[].tableId` | --table-id |
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
