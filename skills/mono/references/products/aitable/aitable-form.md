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

收到仅询问用法的请求后，第一步必须立即实际执行且仅执行对应命令：原子入口用 `dws aitable form share update --help`；Shortcut 入口用 `dws schema --cli-path "aitable +form-share-update" --compact --format json`。Shortcut 名称开头的 `+` 是命令名不可省略的一部分；不得改写、试探其他拼法或改用 `--help`/`-h`。

发现门禁：即使 Skill 或参考文档已提供完整示例，回答前也必须实际执行一次且仅执行一次目标 leaf 的安全 help/schema 查询；不得仅依据 Skill 或参考文档直接作答。用户仅询问用法时，最终回答必须先给出完整命令；缺少必填 ID 时则给出带明确占位符的完整命令模板，禁止猜测。随后明确说明“未传入的分享配置保持原值”；不得执行目标写操作或声称已经执行。上述只读查询是唯一允许的命令。

查询成功后，最终回答只能包含两行纯文本：不要 Markdown 代码围栏、标题、表格、回读命令或其他内容。第一行放用户所问入口的完整命令；已有的必填值必须原样使用，缺少的值必须保留为 `<BASE_ID>`、`<TABLE_ID>`、`<VIEW_ID>` 等明确占位符。第二行先列出需要替换的占位符（没有则省略替换说明），再给出固定的未执行说明，然后立即结束：

```text
dws aitable form share update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --enabled true
请将 <BASE_ID>、<TABLE_ID>、<VIEW_ID> 替换为真实值；未传入的分享配置保持原值。本次仅查询 help/schema，未执行写操作。
```

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
| `form list` | 列出表单视图 | `--base-id` `--table-id` | 每条返回 viewId/name；新建表单**无 title 且 createdAt=0**，改过（form update）后才出现 title 和真实 createdAt |
| `form get` | 按 viewId 取单个表单 | `--base-id` `--table-id` `--view-id` | 客户端按 viewId 过滤后 `data` **即该表单对象**（不是 formViews 数组）；viewId 不存在返回 `form view ... not found` 错误 |
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
| `form share get` | 获取分享配置 | `--base-id` `--table-id` `--view-id` | 返回 enabled/status/shareFormUuid |
| `form share update` | 部分更新分享配置 | `--base-id` `--table-id` `--view-id` + 至少一个配置参数 | 开关用 `--enabled`；访问范围用 `--auth-type-code/--auth-data`；还可更新提交次数、有效期、名称描述、匿名提交、回填和通知配置；未传字段保持原值 |

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

# 6) 开启分享；把已知表单标题同时传给分享配置
dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --enabled true --form-name "员工信息收集" --format json
```

## 返回结构补充

- `form list` 返回 `data.formViews[]`，**每条含** `viewId/name`（+ `createdAt`）；**新建表单没有 `title` 字段且 `createdAt=0`**，只有在 `form update` 碰过之后才会出现 `title` 和真实 `createdAt`。`shareFormUuid` 不在此返回，请用 `form share get` 单独获取。
- `form get` 的 `data` **就是命中的那一条表单对象**（如 `{viewId, name, createdAt, title?, shareFormUuid?}`），不是 `formViews` 数组。Agent 直接读 `data.viewId` / `data.name` 即可，**不要**再取 `data.formViews[0]`。服务端的 viewIds 过滤参数当前不生效，CLI 在客户端按 viewId 精确筛出单条；传了不存在的 viewId 会返回 `form view <id> not found in table` 错误。
- `form field list` 仅返回**未隐藏**的字段；`hidden=true` 的字段不在此返回，如需查看全部字段请用 `field get`。

## MCP 交互注意事项

- `form field hide` 当前每次只接收一个 `fieldId`。多字段必须在同一 Base 写队列中逐个串行设置，全部完成后统一回读一次；不传数组，不并发写。
- 新建表单首次开启分享时，复用 `form create`/`form update` 中已知的标题，通过 `--form-name` 与 `--enabled true` 同时传入；不要为取标题额外调用 `form get`。已有分享仅调整其他配置时，不覆盖原名称。
- 分享开启后回读 `enabled/status/shareFormUuid`。“已开启分享”不等于“已允许匿名/免登录/组织外提交”；需按用户意图显式传入 `--anonymous-submit` 和 `--auth-type-code/--auth-data`，再通过 `form share get` 回读确认。
- 分享和字段 mutation 回执不是最终状态；必须独立读回，写超时时不原样重放。
