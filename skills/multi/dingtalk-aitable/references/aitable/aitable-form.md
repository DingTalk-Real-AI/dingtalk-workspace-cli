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
| `form share update` | 开启/关闭分享 |
| `form questions create` | 添加题目（等价于 `field create`，命令位置上的别名） |
| `form questions delete` | 删除题目（等价于 `field delete`，命令位置上的别名） |

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
| `form share update` | 开启/关闭分享 | `--base-id` `--table-id` `--view-id` `--enabled` | `--enabled true` 开启 / `--enabled false` 关闭。注意：UI 上"发布并分享"按钮是另一概念，本命令只切换内部 enabled 标志，开启后需在 UI 刷新页面才会看到分享面板 |

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

# 6) 开启分享（注意：开启后需 UI 刷新页面才会看到分享面板）
dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID \
  --enabled true --format json
```

## 返回结构补充

- `form list` 返回 `data.formViews[]`，**每条含** `viewId/name`（+ `createdAt`）；**新建表单没有 `title` 字段且 `createdAt=0`**，只有在 `form update` 碰过之后才会出现 `title` 和真实 `createdAt`。`shareFormUuid` 不在此返回，请用 `form share get` 单独获取。
- `form get` 的 `data` **就是命中的那一条表单对象**（如 `{viewId, name, createdAt, title?, shareFormUuid?}`），不是 `formViews` 数组。Agent 直接读 `data.viewId` / `data.name` 即可，**不要**再取 `data.formViews[0]`。服务端的 viewIds 过滤参数当前不生效，CLI 在客户端按 viewId 精确筛出单条；传了不存在的 viewId 会返回 `form view <id> not found in table` 错误。
- `form field list` 仅返回**未隐藏**的字段；`hidden=true` 的字段不在此返回，如需查看全部字段请用 `field get`。
