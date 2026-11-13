# AI表格 (aitable) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断），各命令的详细参数、示例和踩坑说明在 [aitable/](./aitable/) 目录下按需加载。

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| Base 文档 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` |
| 模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` |

> **操作后链接展示**：收到目标对象的当前有效入口后，必须按下文过程要求展示具名完整链接；上表仅用于识别 URI，不作为缺链接时拼接地址的依据。
> 补充：如果 URL/节点 ID 不是来自当前 `aitable` 调用的已验证返回，而是用户直接提供，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe；`extension=dlink` 时逐跳消费目标 `linkSourceInfo`，确认最终目标为 `able` 后再按 AI 表格处理。

## 过程展示建议

- **链接保真必经步骤：**仅当存在明确识别为 JSON 格式的原始业务回执文件时，展示前必须先调用本地 JSON 解析器，从该回执文件提取每条实际待展示的业务 URL，并与实际链接严格比较；核对成功后才生成展示内容。CSV、XLS、XLSX 等非 JSON 导入附件不是业务回执，不作为 JSON 解析；没有上述 JSON 回执文件时，直接使用当前命令的结构化回执核对并展示链接，不因非 JSON 附件阻断成功结果展示。不手抄 JSON 充作源数据，不将聊天摘要当原文；源文件不可用时不伪造输入文件。只提取允许展示的业务 URL，凭据及失效入口不回显。此核对只读本地文件，不调用 DWS、网络或宿主；百分号编码中的字母大小写须原样保留，即使访问等价也不可互换。

- **入口身份：**按链接的对象类型与对象 ID 判断入口身份，不按 `relation` 的字面含义判断；身份匹配的 `links` 即使 `relation=parent` 仍是当前对象入口，只有链接身份确属父级时才称父级入口。
- **最终交付：**回复前按用户请求、已确认范围及当前状态清点全部最终目标，入口须有效且匹配身份、用途；成功回执、`relation=target` 和过程对象不自动成为最终产物。按用户及宿主规定逐目标完成交付／登记和呈现，核对正文链接，不以父级入口合并替代；普通链接不替代登记；过程已展示链接或宿主已呈现卡片，最终 Markdown 正文仍须保留最终目标的有效具名完整链接（`[对象名称](完整URL)`）；过程去重不免除此要求。缺少适用入口时说明缺口。用户要求完整汇总时保留各项完整链接及原状态。某目标恢复中不遗漏其他可交付目标。
- **过程展示：**真实业务回执返回后，对用户要求且有当前有效、角色匹配入口的对象，必须逐一展示具名完整链接；不同目标不合并为 Base，不以父级替代已有深链。仅有父级入口时明确说明，不冒充目标直达。同对象同入口在过程内可去重；内部自动对象不自动纳入，用户要求的对象不因自动创建而排除。过程展示与最终交付分别完成，形式及面板行为遵循用户与宿主要求。
- **入口与名称：**取当前对象 `url`、身份匹配的 `links` 或已确认可用的 `shareFormUrl`；`target/parent` 仅表调用关系。独立读回优先，显式 null/空值不由旧回执覆盖。业务 URL 及参数（含编码和顺序）逐字符原样保留，不截断、改写或重建；逐对象提取本条回执的完整原始 URL，直接用于该对象的链接目标；从本条回执直接复制完整 URL，不套用其他对象的 URL 替换 ID，不重新键入、解码再编码或统一大小写；同批链接各自保留编码，前一条的 `%3d` 不决定后一条的编码，如 `%3D` 不改为 `%3d`。发送前将实际待发送链接与本条回执逐字符比对，一处不同就修正；只声称原样保留不算核对。上传签名 URL、token 等凭据脱敏。名称须对应链接对象；缺名用对象类型或任务标签，仅在无法区分时附 ID，不编造名称。
- **填写者交付边界：**交付给填写者且填写入口未就绪时，只说明缺口，不展示或登记管理页链接；即使注明非填写入口，也不将管理页列为可交付内容。只有用户明确要求管理操作或接收者为管理者时，才交付当前可用的管理页，并注明“管理页，非填写入口”。
- **表单与字段：**填写者只用已确认可用的填写页；未分享或关闭时可向管理者交付可用管理页，注明“管理页，非填写入口”。分享未知或入口未就绪时说明缺口，不自动开启分享或扩大权限。字段 target 须满足 `objectType=field`、`objectId=当前 fieldId`，无自身链接时说明缺口，不展示错配链接，保留核验状态。
- **缺链与状态：**缺链仅按工具明确规则及本次真实标识回退，否则说明缺口，不拼接或臆造。业务核验、登记、展示状态分别判断，保留 pending/unknown/partial/STOP；链接不证明成功、核验、权限、分享或流程启用，回执或展示请求不证明登记、渲染或面板已打开。失败、工具不可用或状态未知时说明缺口，不为补链接或交付重放业务写入。成功登记不重复，未知登记不自动重发，仅按宿主查询／安全恢复契约处理；展示去重不裁剪业务回执及 `links`。
- **预发交付：**只使用可信完整预发 URL，前缀不证明环境映射；缺少时保留业务结果并说明未完成，不拼接、换域或改交生产链接。
- **回执转述边界：**明确的合成演练或回执转述，以用户原意为准；准备性 Schema 查询不将其转为契约评审。核验状态按给定回执的字段语义转述，不以本 Agent 是否亲自执行或独立读回改写该字段；纯 JSON 也保持指定字段语义。转述不等于真实实跑或独立核验；真实写入仍须按结果契约读回，缺失或 pending/unknown 的状态不得擅自提升为 verified。

## 命令索引表

### base (Base 管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `base list` | 列出最近访问的 Base | — | 仅返回最近访问过的，优先用 `base search` |
| `base search` | 按名称搜索 Base（别名 `aitable search`） | — | `--query` help 标必填但实际可省略：不传时返回最近访问的 Base 列表。`--keyword` 是 `--query` 的隐藏别名，同义 |
| `base get` | 获取 Base 信息（含 tables 列表） | `--base-id` | 用户给 URL 时提取末尾 ID |
| `base copy` | 复制整个 Base | `--base-id` | `--target-folder-id` 可传文件夹 ID 或 URL，省略时使用源 Base 工作区根目录；`--only-struct` 仅复制结构 |
| `base get-primary-doc-id` | 获取某记录的主键文档 ID | `--base-id` `--table-id` `--record-id` | 等价 `record primary-doc-get` 的取 ID 视角 |
| `base create` | 创建 Base | `--name` | 创建后直接用返回的 baseId |
| `base update` | 更新 Base 名称 | `--base-id` `--name` | — |
| `base delete` | 删除 Base | `--base-id` | 不可逆 |

### app (应用模式管理)

> 应用模式是 AI 表格面向使用者的 App：一个 Base 只有一个 App，App 下按导航顺序组织 Page，Page 中放置 Widget。应用 Page 的 `pageId` 同时是对应 Dashboard ID，但 `appId`、`pageId`、`widgetId` 不能互换。

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `app get` | 获取 App 信息 | `--base-id` | 无 App 时会幂等初始化默认 App，并返回 `created=true` |
| `app update` | 更新 App 名称、图标、外观、导航或主题 | `--base-id` + 至少一个修改项 | `--icon` 是完整 JSON 对象；主题类字符串按服务端枚举透传 |
| `app page list/get` | 列出页面 / 获取页面详情 | `--base-id`；get 另需 `--page-id` | list 无 App 时会初始化默认 App；最多 100 个 Page |
| `app page create` | 创建应用页面及同 ID Dashboard | `--base-id` `--name` | 非幂等，不自动重试；`--before-page-id` 省略时追加到末尾 |
| `app page update/move` | 更新页面元数据 / 调整导航顺序 | `--base-id` `--page-id` | `--hidden-menu=false` 可明确重新显示；move 省略目标时移到末尾 |
| `app page delete` | 删除页面及其全部 Widget | `--base-id` `--page-id` `--yes` | 不可逆；保留 App 本体 |
| `app widget list/get` | 列出 / 获取页面 Widget | `--base-id` `--page-id`；get 另需 `--widget-id` | list 只读已有 App；单页最多返回 1000 个 Widget |
| `app widget create` | 创建 Widget 并写入页面布局 | `--base-id` `--page-id` `--config` `--layout` | config 需 `chartType`；layout 需 `x/y/w/h`，根布局是 48 列；非幂等，不自动重试 |
| `app widget update` | 更新 Widget 名称、完整配置或布局 | `--base-id` `--page-id` `--widget-id` + 至少一个修改项 | config/layout 均为全量对象，先 get 再更新 |
| `app widget delete` | 删除 Widget 及其布局项 | `--base-id` `--page-id` `--widget-id` `--yes` | 不可逆 |

### table (数据表管理)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `table get` | 获取表结构（字段+视图目录） | `--base-id` | 不传 `--table-ids` 返回全部表 |
| `table list` | 获取数据表（`table get` 的别名） | `--base-id` | 与 `table get` 等价 |
| `table create` | 创建数据表 | `--base-id` `--name` | `--fields` 为 JSON 数组；**可传空数组 `[]`**（默认值即 `[]`），此时服务端自动补一个名为"标题"的 primaryDoc 首列；单次最多 15 个字段 |
| `table update` | 重命名表 | `--base-id` `--table-id` `--name` | — |
| `table delete` | 删除表 | `--base-id` `--table-id` | 不可逆 |

### field (字段管理) → 详见 [aitable-field.md](./aitable/aitable-field.md)、[field-properties](./aitable/aitable-field-properties.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `field get` | 获取字段完整配置 | `--base-id` `--table-id` | 按需展开少量字段 |
| `field list` | 获取字段信息（`field get` 的别名） | `--base-id` `--table-id` | 与 `field get` 等价 |
| `field create` | 创建字段 | `--base-id` `--table-id` + (`--name --type` 或 `--fields`) | 支持单字段/批量模式 |
| `field update` | 更新字段名/配置 | `--base-id` `--table-id` `--field-id` | 不可变更字段类型 |
| `field delete` | 删除字段 | `--base-id` `--table-id` `--field-id` | 不可逆 |
| `field search-options` | 搜索单选/多选字段的选项 | `--base-id` `--table-id` `--field-id` | 仅 singleSelect/multipleSelect；`--keyword` 模糊过滤，不传返回全部 |

### supabase api-key (访问密钥管理) → 详见 [aitable-api-key.md](./aitable/aitable-api-key.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `supabase api-key create` | 创建 SQL Sheet API Key | `--base-id` | MANAGER；确认后执行；完整凭据仅创建时返回 |
| `supabase api-key list` | 查询有效凭据 | `--base-id` | MANAGER；返回 0 或 1 个凭据的元数据 |
| `supabase api-key revoke` | 撤销指定凭据 | `--base-id --key-id` | MANAGER；确认目标 keyId 后执行；重复撤销同样成功 |

### psql (PostgreSQL 只读查询) → 详见 [aitable-psql.md](./aitable/aitable-psql.md)

| 命令模式 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `psql -l` | 列出可查询的 PostgreSQL 逻辑表 | `-d <baseId>` | 数据查询前的表发现；输出为 psql 文本，不加 `--format json` |
| `psql -t` | 查看逻辑列名和 PostgreSQL 类型 | `-d <baseId>` `-t <tableId>` | SQL 前核对列；全部属性列加 `--all-properties` |
| `psql -c` | 执行一条只读 PostgreSQL SELECT | `-d <baseId>` `-c <SQL>` | SQL 的 `FROM` / `JOIN` 自动确定主表；支持单表及同 Base 多表 JOIN；只读；输出为 psql 文本 |

### record (记录管理)

| 命令 | 用途 | 必读 reference | 路由提醒 |
|------|------|----------------|----------|
| `record query` | 查询/搜索记录 | [aitable-record-query.md](./aitable/aitable-record-query.md) | 先 `field get` 拿 fieldId 与类型；完整结果必须用 `--all --page-limit 0` 自动翻页；filters 中的实体展示名须先解析为稳定 ID/结构化值；`--query`（隐藏别名 `--keyword`）全文搜索 |
| `record list` | 获取记录（`record query` 的别名） | [aitable-record-query.md](./aitable/aitable-record-query.md) | 与 `record query` 等价 |
| `record get` | 按 ID 取记录（`record query --record-ids` 的窄别名） | [aitable-record-query.md](./aitable/aitable-record-query.md) | 已知 recordId 时首选；必填 `--record-ids`（单次最多 100 条）；未暴露 filters/sort/query/cursor/limit |
| `record stats` | 不分组的服务端聚合 | [aitable-record-stats.md](./aitable/aitable-record-stats.md) | statsType 大写；最多 20 项，同字段不可重复；全量统计省略 limit |
| `record group-stats` | 分组、去重和高级服务端聚合 | [aitable-record-stats.md](./aitable/aitable-record-stats.md) | statsType 大写；group/sort 为 JSON 数组字符串；最多 1000 个分组 |
| `record query-empty` | 查询完全没填用户字段的空行 | [aitable-record-query.md](./aitable/aitable-record-query.md) | `--base-id` `--table-id`；`--limit` 扫描预算 [1,100]；扫完前需用 `--cursor` 翻页（nextCursor 为空才表扫完，成功空页属正常） |
| `record create` | 新增记录 | [aitable-record-create.md](./aitable/aitable-record-create.md) | cells key 支持 fieldId 或当前表内唯一字段名，推荐 fieldId；单次最多 100 条 |
| `record update` | 更新记录 | [aitable-record-update.md](./aitable/aitable-record-update.md) | 需先 query 拿 recordId；`cells` key 支持 fieldId 或当前表内唯一字段名，推荐 fieldId；`--records` 是 `[{recordId,cells},...]` 数组 |
| `record batch-update` | 把同一份 cells 批量应用到多条记录 | [aitable-record-update.md](./aitable/aitable-record-update.md) | `--record-ids`（≤100）+ `--cells` 共享 patch |
| `record upsert` | 批量创建或更新（有 recordId 走更新，无则创建） | [aitable-record-upsert.md](./aitable/aitable-record-upsert.md) | `--records`/`--records-file`；单次最多 100 条 |
| `record delete` | 删除记录 | [aitable-record-delete.md](./aitable/aitable-record-delete.md) | 不可逆，需先 query 确认 |
| `record share-url` | 批量获取记录分享链接 | [aitable-record-share.md](./aitable/aitable-record-share.md) | `--record-ids`（逗号分隔，单次最多 20 条） |
| `record history-list` | 查询单条记录的变更历史 | [aitable-record-history.md](./aitable/aitable-record-history.md) | `--record-id` 单条；`--offset`/`--limit`（≤50） |
| `record primary-doc-get` | 查询记录的主键文档 nodeId | [aitable-primary-doc.md](./aitable/aitable-primary-doc.md) | 无文档时返回 `no record` 错误 |
| `record primary-doc-create` | 为记录创建主键文档 | [aitable-primary-doc.md](./aitable/aitable-primary-doc.md) | 幂等；`--field-id` 须 primaryDoc 类型 |

含创建的 `+record-upsert` 写前自动保存原操作号与恢复文件；超时不重放，使用原 Base/Table/clientToken 和回执 `writeIntentFiles` 执行 `+record-write-result --write-intent <原文件>`。unknown 表示尚未确认；创建组核对原 token，更新组另按原 recordId 读回。不得省略恢复文件或改写身份绕过校验；此自动落盘保证不适用于原子 `record create/upsert`。

### comment (记录评论) → 详见 [aitable-comment.md](./aitable/aitable-comment.md)

| 命令 | 用途 | 必填参数 | 路由提醒 |
|------|------|----------|----------|
| `comment list` | 分页查询记录评论与回复 | `--base-id --table-id --record-id` | 空 `data.comments` 不代表结束；仅 `meta.pagination.endpoint_exhausted=true` 时停止，否则按 `meta.pagination.next_token` 续页 |
| `comment create` | 创建评论话题 | 定位参数 + `--content` 或 `--rich-content` | 非幂等；未知状态先 list 对账 |
| `comment reply` | 回复已有评论 | 定位参数 + `--topic-id --comment-key` + 正文 | 标识必须来自同一记录真实返回；非幂等 |
| `comment update` | 完整替换本人评论正文 | 定位参数 + `--topic-id --comment-key` + 正文 | 仅纯文本会移除旧 @和图片；无 CAS |
| `comment delete` | 删除本人评论 | 定位参数 + `--topic-id --comment-key` | 不可恢复；确认后再追加 `--yes`，关联回复处理由服务端决定 |

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
| `workflow edit-example` | 获取编辑文档与 DSL 示例 | 无 | create/update 前优先调用，内容由服务端提供 |
| `workflow create` | 创建并发布工作流 | `--base-id` `--dsl` | `--dsl` 为完整 workflow-dsl/v1；非幂等，不自动重试 |
| `workflow update` | 更新并发布工作流 | `--base-id` `--workflow-id` `--dsl` | 全量替换，先 get 留底；检查 `data.valid/issues` |
| `workflow list` | 列出 Base 下所有工作流 | `--base-id` | 支持 `--limit [1,100]` / `--offset >=0`；list 出参字段叫 `flowId` |
| `workflow get` | 获取单个工作流详情（含 flowSchema） | `--base-id` `--workflow-id` | `--workflow-id` 接受 list 里的 `flowId`（同值） |
| `workflow enable` | 启用工作流 | `--base-id` `--workflow-id` | 返回 `{enabled: true}` 是动作确认；要确认真启用看 list 的 `status` |
| `workflow disable` | 禁用工作流（高危） | `--base-id` `--workflow-id` | 影响业务自动化，必须先取得用户明确确认；status 变 STOP |
| `workflow run` | 立即执行工作流（需确认） | `--base-id` `--workflow-id`；记录触发另需 `--table-id` `--record-ids` | 返回 `executionId`；不确定时先用 history 核对，避免重复执行 |
| `workflow history` | 查询工作流执行历史 | `--base-id` `--workflow-id` | 支持 status、Unix 毫秒时间范围和 page/size；`instanceId` 对应 run 的 `executionId` |

> 当前支持创建、更新、查询、启停、手动执行和执行历史；删除工作流仍未开放。

### dashboard & chart → 详见 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)

新建/重构分析仪表盘、选图/指标/布局不明、图表批建恢复决策或异常复验时按专项处理，仅统计步骤可补读 record-stats 或 psql；普通 Dashboard CRUD 直接使用下表命令，不加载分析闭环。

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
| `+role-members-add` | 向自定义角色追加成员 | `--base-id` `--role-id` `--members` `--yes` | members 为最多 200 项的 JSON array，id/memberType 均为 String；用户填 uid（不是 userId）和 `"2"`；返回写后成员列表 |
| `+role-members-update` | 完整替换或清空成员 | `--base-id` `--role-id` `--members` `--default-role-mode` `--yes` | DWS 强制完整 members 与 mode=0/1/2；1 还需 `--default-role-id`；`[]` 清空，不接受 null，默认角色按回读值传入 |
| `+role-copy` | 复制角色配置（不复制成员） | `--base-id` `--source-role-id` `--yes` | 系统角色不可复制；沿用 roleType/flowType/subRoles，name 省略或空字符串使用「源角色名 副本」 |
| `+role-query-user` | 查询当前或指定用户命中的角色 | `--base-id` | targetUid 可选，默认自己，不替换调用身份；返回 enabled 和 roles 配置（不含成员），高级权限关闭时 roles 通常为空 |

> 成员完整字符串编码和类型表见高级权限 reference；不能用 user/conv 或数字 2，可选 memberIdBelongOrgId 为 number（Java Long 整数），同文档组织省略由服务端回填 docOrgId，跨组织填成员所属组织 ID。roleId/sourceRoleId/defaultRoleId/targetUid 均为 long 形态 String。
> **角色 CRUD 与成员操作均已支持**：create/get/list/update/delete、成员追加/完整替换、角色复制和用户角色查询都可走 CLI。
> 所有写命令需要 Base 管理员权限；非管理员只能调只读命令。
> `--base-id` 是公开参数；不要在 Agent 命令中使用隐藏兼容别名 `--base`。

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

### 仪表盘 / 图表（按需配置）

新建/重构分析仪表盘、图表批建恢复决策、Chart config 或 Dashboard 完整 config/arrange，按需读取 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)；普通 Dashboard CRUD 直接使用命令，不进入复杂配置流程。

已有合法 config 且指标/布局语义明确时不重复读模板；需要构造配置时仅获取所需类型模板，Dashboard 完整 config 结构缺失时才读取对应模板。具体读取路径以专项指南为准，不预读两份全量模板。

要点：

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用。
- `dashboard share get` 可能返回 `404`（`retryable:true`，从未分享甚至刚开启分享后立即查都可能 404），按可重试错误处理，不要误判为参数拼错；行为不稳定，别当"是否已分享"的唯一判据。
- `chart share get` 稳定返回 `success + data`（含 `enabled`），从未分享时 `enabled=false`，不会 404。
- `dashboard share update` 开 ORG 分享后 `shareType` 回显 `"[1]"`（服务端已知问题）；`chart share update` 正确回显 `ORG`。详见 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)。

### 应用模式（建议顺序）

```bash
# 1) 获取或初始化 App，再发现 pageId
dws aitable app get --base-id <BASE_ID> --format json
dws aitable app page list --base-id <BASE_ID> --format json

# 2) 读取 Page 和 Widget；修改完整 config/layout 前先读回
dws aitable app page get --base-id <BASE_ID> --page-id <PAGE_ID> --format json
dws aitable app widget list --base-id <BASE_ID> --page-id <PAGE_ID> --format json
dws aitable app widget get --base-id <BASE_ID> --page-id <PAGE_ID> --widget-id <WIDGET_ID> --format json
```

创建 Widget 的最小形状为 `--config '{"chartType":"AI_ANALYZE"}' --layout '{"x":0,"y":0,"w":48,"h":8}'`。创建 Page/Widget 返回状态不明时先 list/get 回读，不能自动重放；删除 Page 会级联删除其中全部 Widget。

### 导出数据（两阶段轮询）

`export data` 常见为异步任务：首次调用可能只返回 `taskId`，需要继续轮询。

```bash
# 第一步：创建任务（按 scope 传必要参数）
dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --export-format excel --timeout-ms 1000 --format json

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
- 创建 → `base create`；仅新建独立空白 Base、未指定自定义表结构时，使用 `dws aitable base create --name <名称>`，保留平台默认结构，不额外创建默认同名表或写入业务记录；不以 `tables=[]` 表达空白 Base；未独立核验记录数时，不宣称已验证零记录。
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
- 总数/求和/平均值/中位数/完整率等标量统计 → `record stats`（读 [aitable-record-stats.md](./aitable/aitable-record-stats.md)）
- 分组统计/唯一实体计数/去重率 → `record group-stats`（读 [aitable-record-stats.md](./aitable/aitable-record-stats.md)）
- 已知 recordId 反查字段值 → `record get`（按 ID 取专用，等价 `record query --record-ids`）
- 添加/写入 → `record create`（读 [aitable-record-create.md](./aitable/aitable-record-create.md)）
- 修改/更新 → `record update`（读 [aitable-record-update.md](./aitable/aitable-record-update.md)）
- 删除 → `record delete`

用户说"查看可查询表/SQL 表结构/SQL 字段类型/查询前 N 条/按列查询/SQL/PostgreSQL/SELECT/JOIN/两表关联" → 读 [aitable-psql.md](./aitable/aitable-psql.md)。数据查询前的表清单和逻辑列发现走 `psql`；普通按记录 ID、关键词或 filters 查行仍走 `record query`；两表关联不得误路由为 LOOKUP/FILTER_UP 公式配置。

用户说"筛选/过滤/filter" → 读 [aitable-filter-sort.md](./aitable/aitable-filter-sort.md)

用户说"统计/分析/聚合/TOP N/全量" → 先读 [aitable-data-analysis-sop.md](./aitable/aitable-data-analysis-sop.md)，聚合参数见 [aitable-record-stats.md](./aitable/aitable-record-stats.md)

用户说"公式/formula/计算字段/派生指标" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)

用户说"查找引用/lookup/filterUp/跨表" → 读 [aitable-formula-guide.md](./aitable/aitable-formula-guide.md)（§5.4 跨表引用）

用户说"仪表盘/图表/chart" → 普通 Dashboard CRUD 直接使用命令；构建、配置、图表批建恢复决策或异常复验读 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)

用户说"应用模式/App 页面/Widget" → `app get` 后按 `app page` / `app widget` 子命令处理；不要混同普通 Base 信息或开放平台应用

用户说"附件/上传文件" → 读 [aitable-attachment.md](./aitable/aitable-attachment.md)

用户说"导入/导出/import/export" → 读 [aitable-export-import.md](./aitable/aitable-export-import.md)

用户说"模板" → `template search`

仅补确认标记时，保持目标、名称、字段、类型及其他业务参数不变，不夹带修正，不使用占位类型或重复 JSON 键。

命令报错/操作失败：`partial_success` 或未知写入结果应先保留原 ID、已完成步骤及已知副作用，对受影响的原对象只读核实后，再按当前契约和授权补齐。只读核实仍无法确定写入结果或补齐范围时，停止后续写入并说明未确认项，不重放创建。checkpoint、失败计数或输入错误不构成重放许可；不得自行新建替代 Base，或通过改名、清理旧对象掩盖问题；确需替代或清理时，说明影响并按既有规则取得相应授权。原 partial 与恢复核验结果分别说明。其他恢复细节按需读 [aitable-error-recovery.md](./aitable/aitable-error-recovery.md)。

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
| `base list/search` | `baseId` | 所有后续命令的 --base-id；访问链接使用返回的 url/links |
| `base create` | `baseId` | 后续命令；访问链接使用返回的 url/links |
| `base get` | `tables[].tableId` | --table-id |
| `table get` | `fields[].fieldId` | record 操作的 cells key, field get/update/delete |
| `record query` | `recordId` | record update/delete；按 ID 反查字段值用 `record get` |
| `template search` | `templateId` | base create --template-id，拼接模板预览 URI |

## URL/节点 ID → baseId 规范化

用户提供 `https://alidocs.dingtalk.com/i/nodes/{id}` 或来源未验证的 nodeId 时：
1. 先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 执行 `dws drive info`
2. 若为 `extension=dlink`，执行 `dws doc info` 并逐跳消费目标 `linkSourceInfo`；记录已访问 ID，失败、字段缺失或 ID 重复即停
3. 只有最终目标 `extension=able` 时，才将最终目标 nodeId 作为 `baseId`
4. 已确认的 AITable URL 如含 table/view 参数，再解析并复用这些稳定 ID

> 如果该 URL 来自 `dws aitable` 返回或已在当前链路 probe 过，可直接复用；
> 禁止直接把 dlink 快捷方式入口的路径段当作 baseId。明确移动、重命名或删除快捷方式入口本身时才保留顶层 nodeId，并改走对应的 Drive 入口管理命令。

## 注意事项

- 对象寻址使用真实 ID（baseId/tableId/fieldId/recordId）；记录写入的 cells key 也支持当前表内唯一字段名；`psql` 的用户 SQL 使用 `psql -l/-t` 返回的真实逻辑表名和列名
- `aitable psql` 输出 PostgreSQL 表格文本，不支持也不添加 `--format json`；其他结构化读取仍按全局规则使用 `--format json`
- records 的 cells key 支持 fieldId 或当前表内唯一字段名；推荐 fieldId，字段名须精确匹配
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

## 对象访问链接

对象访问链接复用 Web 定位参数：数据表/视图/字段使用 `sheetId/viewId/fieldId`，应用模式使用编码 `appId/pageId`，普通仪表盘和说明文档使用 `applicationId`，自动化使用 `flowId/autoOpenAutomation=true`。字段复合创建及续跑结果逐项保留 `sheetId/fieldId` 链接；展示方式参考前文“过程展示建议”。表单管理入口 `entrance=form` 与公开填写链接不同，不能因此开启分享；自动打开流程编辑器不会启用流程。字段业务 type 不作为链接对象类型。

`+url-resolve` 默认仅做本地解析，覆盖 Base、Table、View、Record、Field、Form、App/Page、Application 和 Workflow；参数冲突、缺父级或混用不同对象族时明确拒绝，不能把子对象降级为 Base 验证。`--verify` 只调用对应只读接口并核对所属关系；Application 经 `list_nsheet_nodes` 的 `nodeId/nodeType` 核对仪表盘/说明文档导航节点，不能用底层文档 UUID 或 Base 存在证明目标节点存在。自动化入口保留的后台表/视图/Application 参数不作为目标；无 flowId 时仅验证 Base，有 flowId 时验证流程。仅 App 链接暂无只读验证入口，必须明确拒绝验证，不能调用会隐式创建 App 的 `get_app/list_app_pages`；含 Page 的链接通过 `get_app_page` 同时核对 App/Page。公开表单分享 URL 不作为 Base 对象链接解析。

`+url-resolve` 使用统一 `ok/outcome` 信封，成功的稳定 ID 位于 `data.target`；仅 `data.verified=true` 表示只读验证通过。失败保留错误分类和非零退出码，不能把本地解析成功当作远程存在证明。
