# form — 表单管理

## 命令一览

| 命令 | 用途 |
|------|------|
| `form list` | 列出数据表下所有表单视图 |
| `form get` | 按 viewId 取单个表单详情（list_form_views + viewIds 过滤） |
| `form create` | 创建表单视图（等价于 `view create --view-type FormDesigner`） |
| `form update` | 更新表单标题或描述 |
| `form delete` | 删除表单视图（不可逆） |
| `form field list` | 列出表单可见字段 |
| `form field update` | 更新字段必填/描述 |
| `form field hide` | 在表单中隐藏/显示字段（不影响底层数据表字段） |
| `form share get` | 获取分享配置 |
| `form share update` | 部分更新分享开关、授权、有效期和通知等配置 |
| `form questions create` | 添加题目（等价于 `field create`，命令位置上的别名） |
| `form questions delete` | 删除题目（等价于 `field delete`，命令位置上的别名） |

## 仅询问分享更新用法时

用法询问与返回值评审不能共用发现路径：问原子命令写法只执行 `dws aitable form share update --help`（禁止改查 Schema）；本专用规则优先于通用 Schema 导航。

收到仅询问用法的请求后，第一步必须立即实际执行且仅执行对应命令：原子入口用 `dws aitable form share update --help`；Shortcut 入口用 `dws schema --cli-path "aitable +form-share-update" --compact --format json`。Shortcut 名称开头的 `+` 是命令名不可省略的一部分；不得改写、试探其他拼法或改用 `--help`/`-h`。

发现门禁：即使 Skill 或参考文档已提供完整示例，回答前也必须实际执行一次且仅执行一次目标 leaf 的安全 help/schema 查询；不得仅依据 Skill 或参考文档直接作答。用户仅询问用法时，最终回答必须先给出完整命令；缺少必填 ID 时则给出带明确占位符的完整命令模板，禁止猜测。随后明确说明“未传入的分享配置保持原值”；不得执行目标写操作或声称已经执行。上述只读查询是唯一允许的命令。

查询成功后，最终回答只能包含两行纯文本：不要 Markdown 代码围栏、标题、表格、回读命令或其他内容。第一行放用户所问入口的完整命令；已有的必填值必须原样使用，缺少的值必须保留为 `<BASE_ID>`、`<TABLE_ID>`、`<VIEW_ID>` 等明确占位符。第二行先列出需要替换的占位符（没有则省略替换说明），再给出固定的未执行说明，然后立即结束：

```text
dws aitable form share update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --enabled true
请将 <BASE_ID>、<TABLE_ID>、<VIEW_ID> 替换为真实值；未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。
```

已知 ID 与占位符必须区别处理：用户明确给出的短 ID 也按原值使用，不因其长度或看起来像示例就要求替换。所有 ID 已知时，第二行必须原样为“未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。”；只有命令中确实用了占位符时才添加对应替换说明。Shortcut 的命令行必须保留 `--format json`，固定说明中的 `help/schema` 不因本次只查 schema 而改写。

两种入口都必须完整保留用户指定的配置值，尤其是表单名对应的 `--form-name`；不得因精简为两行而只留下 ID 和 `--enabled`。以下是已知 ID 的 Shortcut 用法回答示例（标题按用户输入替换，不能省略）：

```text
dws aitable +form-share-update --base-id base-123 --table-id table-456 --view-id view-789 --enabled true --form-name "报名表" --format json
未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。
```

### 返回值评审专用查询（不适用于命令用法询问）

作答前必须有本次实际执行的目标 compact Schema 查询结果；仅加载 Skill 或看到合成样本不满足此条件。先执行下表命令，再解释，不能直接根据 Skill 回答。

Help/Schema 是离线契约查询，不调用线上业务，也不读写用户的表单。因此“只分析合成返回，不要执行线上业务命令”仍允许且需要下表的本机查询，不能将其误判为线上读取而跳过；只有用户明确禁止任何命令或本机查询时才不执行，并说明本机契约未核对。

仅评审返回值/故障结果（而非询问写法）时，不套用两行命令模板，也不适用“优先 Shortcut”规则。用户指定的原子/Shortcut 入口必须原样保留，即使它们共用 Result 契约也不能互换；严格按下表查询一次后解释样本，不搜索源码/reference，不执行业务读写。

| 用户指定入口 | 唯一契约查询 |
|---|---|
| `form share get` | `dws schema --cli-path "aitable form share get" --compact --format json` |
| `form share update` | `dws schema --cli-path "aitable form share update" --compact --format json` |
| `+form-share-get` | `dws schema --cli-path "aitable +form-share-get" --compact --format json` |
| `+form-share-update` | `dws schema --cli-path "aitable +form-share-update" --compact --format json` |

不得用搜索源码或 reference 代替本机契约查询。get 只诊断分享配置，不返回 `cpSynced`；不能建议“通过 get 回读 cpSynced 后确认闭环”。未验证的 CP 应继续标为未确认并交由服务端诊断，不能把 get 的成功或 UUID 非空当作恢复证明。

结果判断：分享开关依据 `enabled`，保留 UUID 不代表开启；UUID/封面为空就如实报告，不能推测唯一成因或拼装封面 URL。get 的成功不证明 CP 同步。update 仅在必需字段完整且类型正确、`cpSynced=true` 时成功；缺失/false/类型异常均不能确认闭环。统一部分失败无顶层 error：原始响应在 `data.succeeded[0].response`，该阶段仅表示收到回执；失败原因在 `data.failed[0].error`，含 `execution_started=true`。不要自行重放写入或补偿 CP。

解释边界：`status` 未公布枚举含义时保留原始数值，不把 0/1 自行翻译成未发布/已发布；`cpSynced=false` 只支持“CP 终态未确认”，不证明外部用户必定无法访问。UUID/封面空值不能证明此前从未创建，短 ID 不能仅因长度被判为占位符。


## 建议操作顺序

```bash
# 1) 列出数据表下的表单视图
dws aitable form list --base-id BASE_ID --table-id TABLE_ID --format json

# 2) 查看单个表单详情
dws aitable form get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json

# 3) 查看表单字段配置
dws aitable form field list --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json

# 4) 查看分享配置
dws aitable form share get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json
```

开启并取得可发送链接的最短闭环使用 canonical shortcut：

```bash
dws aitable form create --base-id BASE_ID --table-id TABLE_ID --name "表单名" --format json
dws aitable +form-share-update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --enabled true --form-name "表单名" --format json
dws aitable +form-share-get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --format json
```

从最后一次返回直接读取 `data.shareFormUuid`，分享地址为 `https://alidocs.dingtalk.com/i/form/{shareFormUuid}`。字段已存在时不要再调用 Help、Catalog、`form get`，也不要换 `--verbose`、`raw`、`pretty` 重复请求。用户还要求发送时，把该完整 URL 交给 Chat 的发送命令并检查真实发送回执。

## 要点

- **创建表单**有两种等价方式：
  - `form create --name "表单名"`（推荐，语义清晰）
  - `view create --view-type FormDesigner --name "表单名"`（底层一致）
- `form update` 支持 `--title` 与 `--name` 两个等价参数；至少需传一项
- `form field update` 必须传 `--required` 或 `--field-description` 至少一项
- `form field hide` 仅控制字段在表单中的可见性，不影响底层数据表字段
- **题目管理**与字段管理本质相同（题目 = 表格字段）：
  - `form questions create` 与 `field create` 入参完全一致（`--fields` JSON 或 `--name --type`）
  - `form questions delete` 与 `field delete` 入参完全一致（必传 `--field-id`）
  - 设置必填要在 create 后用 `form field update --required true` 单独调一次

## form 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `form list` | 列出表单视图 | `--base-id` `--table-id` | 返回 viewId/name/title/createdAt |
| `form get` | 按 viewId 取单个表单 | `--base-id` `--table-id` `--view-id` | 内部基于 list_form_views 过滤 |
| `form create` | 创建表单视图 | `--base-id` `--table-id` `--name` | viewType=FormDesigner |
| `form update` | 更新表单 | `--base-id` `--table-id` `--view-id` | `--title`/`--name`（等价）和 `--description` 至少传一项；同时传 title/name 时 title 优先 |
| `form delete` | 删除表单 | `--base-id` `--table-id` `--view-id` `--yes` | 不可逆 |

## form field 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `form field list` | 列出表单字段 | `--base-id` `--table-id` `--view-id` | 返回 fieldId/name/type/required/hidden/description（hidden=true 的字段不在此返回） |
| `form field update` | 更新表单字段 | `--base-id` `--table-id` `--view-id` `--field-id` | `--required` 或 `--field-description` 至少一项 |
| `form field hide` | 切换字段隐藏 | `--base-id` `--table-id` `--view-id` `--field-id` `--hidden` | `--hidden true` 隐藏 / `--hidden false` 显示 |

## form questions 子命令

`form questions create/delete` 与 `field create/delete` 入参、行为完全一致，只是命令位置归属于 `form` 命令组，方便从表单视角操作题目。

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `form questions create` | 添加题目 | `--base-id` `--table-id` + (`--fields` 或 `--name --type`) | 入参与 `field create` 完全一致 |
| `form questions delete` | 删除题目 | `--base-id` `--table-id` `--field-id` `--yes` | 入参与 `field delete` 完全一致；不可逆；批量需多次调用 |

## form share 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `form share get` | 获取分享配置 | `--base-id` `--table-id` `--view-id` | 返回服务端真实的 enabled/status/shareFormUuid/formCover；只读，不修复 CP |
| `form share update` | 部分更新分享配置 | `--base-id` `--table-id` `--view-id` + 至少一个配置参数 | 成功返回真实 shareFormUuid/status/formCover/cpSynced；只有 cpSynced=true 才完成闭环；未传字段保持原值 |

## 完整工作流示例

> **占位符约定**：
> - `BASE_ID` 来自 `dws aitable base list` / `base search` 返回的 `data.bases[].baseId`
> - `TABLE_ID` 来自 `dws aitable base get --base-id BASE_ID` 返回的 `data.tables[].tableId`
> - `VIEW_ID` 来自步骤 1 `form create` 返回的 `data.viewId`
> - `FIELD_ID` 来自步骤 2 `form questions create` 返回的 `data.results[].fieldId`

```bash
# 1) 创建表单 → 取返回的 data.viewId 作为 VIEW_ID
dws aitable form create --base-id BASE_ID --table-id TABLE_ID --name "员工信息收集" --format json

# 2) 添加题目 → 取返回的 data.results[].fieldId 作为 FIELD_ID
dws aitable form questions create --base-id BASE_ID --table-id TABLE_ID \
  --fields '[{"fieldName":"姓名","type":"text"},{"fieldName":"邮箱","type":"text"}]' --format json

# 3) 配置表单标题与描述
dws aitable form update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --title "员工信息收集" --description "请填写您的基本信息" --format json

# 4) 设置题目必填（FIELD_ID 来自步骤 2）
dws aitable form field update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --field-id FIELD_ID --required true --format json

# 5) 隐藏不需要的题目
dws aitable form field hide --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --field-id FIELD_ID --hidden true --format json

# 6) 开启分享；把已知表单标题同时传给分享配置，并检查返回的 shareFormUuid/status/formCover/cpSynced
dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --enabled true --form-name "员工信息收集" --format json
```

## 返回结构补充

- `form list` 返回 `data.formViews[]`，**每条含** `viewId/name`（以及服务端当前可用的 `title/createdAt`）；`shareFormUuid` 不在此返回，请用 `form share get` 单独获取。
- `form get` 的 `data` 是客户端按 `viewId` 精确筛出的单个表单对象，不是 `formViews` 数组。Agent 直接读取 `data.viewId` / `data.name`；不存在的 `viewId` 会明确失败。
- `form field list` 仅返回**未隐藏**的字段；`hidden=true` 的字段不在此返回，如需查看全部字段请用 `field get`。

## MCP 交互注意事项

- `form field hide` 当前每次只接收一个 `fieldId`。多字段必须在同一 Base 写队列中逐个串行设置，全部完成后统一回读一次；不传数组，不并发写。
- 新建表单首次开启分享时，复用 `form create`/`form update` 中已知的标题，通过 `--form-name` 与 `--enabled true` 同时传入；不要为取标题额外调用 `form get`。已有分享仅调整其他配置时，不覆盖原名称。
- `form share update` 的成功结果已经来自服务端回读并完成 CP 投影校验；必须检查 `shareFormUuid/status/formCover/cpSynced`，只有 `cpSynced=true` 才能确认闭环完成。部分失败或 `cpSynced=false` 不得描述为成功。
- DWS 不自行调用第二个 View 更新命令补偿 CP，也不拼装封面 URL。旧服务端发布窗口内 `formCover` 可能为空，应如实说明；稍后诊断当前配置时可用 `form share get`。
- “已开启分享”不等于“已允许匿名/免登录/组织外提交”；需按用户意图显式传入 `--anonymous-submit` 和 `--auth-type-code/--auth-data`。写超时时先查询真实状态，不原样重放 mutation。
