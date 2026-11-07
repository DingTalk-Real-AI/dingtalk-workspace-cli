# field — 字段管理

仅在 AI 字段、关联字段、lookup/filterUp 或其他复杂 config 结构不确定时读取。普通 text/number/date/singleSelect/currency 字段的 create/update/delete 使用根 Skill 次级直达。

本文是字段复杂配置的终点；命令参数已覆盖时，不再读取其他 AITable Reference 或 Help。

## field get — 获取字段详情

```
Usage:
  dws aitable field get [flags]
Example:
  dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --field-ids fld1,fld2
Flags:
      --base-id string     Base ID (必填)
      --field-ids string   字段 ID 列表，逗号分隔，单次最多 10 个
      --table-id string    Table ID (必填)
```

返回字段的完整配置（含 options 等）。不要假设未指定 `--table-ids` 的 `table get` 枚举结果含字段；字段目录和配置以 `field get` 返回为准。

## field create — 创建字段

```
Usage:
  dws aitable field create [flags]
Example:
  dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --name "状态" --type "singleSelect" --config '{"options":[{"name":"待办"},{"name":"进行中"},{"name":"已完成"}]}'

  # 或者使用批量创建模式:
  dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --fields '[{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"}]}}]'
Flags:
      --base-id string    Base ID (必填)
      --name string       要创建的单字段名称（与 --type 配合使用，替代 --fields）
      --type string       要创建的单字段类型（需要配合 --name，参考 table create 的内置类型）
      --config string     单字段配置 JSON（需要配合 --name/--type，结构参考 table create）
      --ai-config string  单字段 AI 配置 JSON（需要配合 --name/--type）
      --fields string     批量新增字段 JSON 数组，单次最多 15 个；每个字段的配置写在其 config/aiConfig 内
      --table-id string   Table ID (必填)
```

`field create` 有且只有两种输入模式：

- 单字段模式：必须同时传 `--name` 和 `--type`；`--config`、`--ai-config` 只作为该字段的附加配置。
- 批量模式：只传 `--fields`；字段配置写在数组内各对象的 `config` / `aiConfig` 中。

两种模式严格互斥。`--fields` 不能与 `--name`、`--type`、`--config` 或 `--ai-config` 混用；单独传 `--config` 也会报错，不会被静默忽略。

例如创建单选字段时，单字段模式的 `--config` 是一个配置对象；批量模式则把同一对象放入对应字段元素的 `config`：

```bash
# 单字段模式
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "部门" --type singleSelect \
  --config '{"options":[{"name":"技术部"},{"name":"产品部"}]}'

# 批量模式
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --fields '[{"fieldName":"部门","type":"singleSelect","config":{"options":[{"name":"技术部"},{"name":"产品部"}]}}]'
```

人民币货币字段使用真实 `currency` 枚举和配置，不使用 `select` 或 `config.symbol`：

```bash
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "预计成交额" --type currency \
  --config '{"currencyType":"CNY","formatter":"FLOAT_2"}' --format json
```

允许部分成功，返回结果逐项标明成功/失败状态。

### AI 字段创建示例

内置 AI 插件先用 `field ai-plugins --base-id <BASE_ID> --table-id <TABLE_ID>` 只读查询真实 `pluginId`、`fieldTypes`、`configSchema` 与 `configurationMode/reason`，不猜插件 ID。工作流凭据需要现有 UI 绑定时，按返回限制处理，不把凭据传入 CLI。

配置复用 `field create --ai-config`、`field update --ai-config` 与 `+field-update --ai-config`。配置使用 `{"type":"builtin","pluginId":"<已确认插件ID>","formData":{},"referFieldIds":["<FIELD_ID>"],"autoRecompute":true}`，无需通用 AI 的 `prompt` / `outputType`。由 MCP 校验与转换，CLI 不自行构造 `decoratorConfigs`。回执 `verificationStatus=pending` 时继续 `field get` 只读核对，不能视为已生效或再次写入。

快递/OCR 等已支持的内置模板可能要求基础字段类型 `object`，按 `ai-plugins` 返回的 `fieldTypes` 选择；这不表示可以创建任意普通 object 字段。

下例是通用 AI 配置：

```bash
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "AI摘要" --type text \
  --ai-config '{
    "outputType":"text",
    "prompt":[
      {"type":"text","value":"请将下面内容总结成不超过80字的中文摘要："},
      {"type":"fieldRef","fieldId":"fld_content"}
    ],
    "autoRecompute":true,
    "enableWebSearch":false,
    "enableThinking":true
  }' --format json
```

说明：
- `outputType` 与字段类型需一致（如 `outputType=text` 配 `--type text`）
- `prompt` 里通过 `fieldRef` 引用已有字段
- `autoRecompute=true` 表示引用字段变化后自动重算
- **AI 字段的 prompt 必须至少包含一个 `fieldRef` 引用**，纯文本 prompt 会被后端拒绝

### 字段编组

字段编组作用于列，不是按记录值分组。入口须已在目标 MCP 部署：

| 入口 | 参数与行为 |
|---|---|
| `field-group list` | `--base-id`、`--table-id`；查询组名、说明和组内字段 ID |
| `field-group create` | 加 `--name`、`--field-ids F1,F2`（1～100 项），可选 `--description`；清空旧组时须 `--confirm-empty-group=true` |
| `field-group update` | 加 `--group-id` 和 `--name` / `--description` 至少一项；显式空说明清除，未传配置保留 |
| `field-group ungroup` | 加 `--group-id`；取消编组，保留字段与数据 |
| `field-group move-fields` | 加 `--field-ids`；传 `--target-group-id` 移入目标组，省略则移出成为普通列 |

移空源组时必须显式 `--confirm-empty-group=true`，沿用页面自动取消空组的处理；`--yes` 只确认写操作，不替代这个业务确认。取消或移出后的普通列继续遵循当前视图列顺序与隐藏规则，不额外创建“未分组”组。变更 `verificationStatus=pending` 时保留回执，按 `meta.operation.next_command` 只读核对，不能重放写入。

`field-group list` 与 `field ai-plugins` 的读取失败在 `error.upstream_code` 保留 MCP 错误码：`InvalidArgument`、`InvalidRequest.ResourceNotFound`、`InvalidRequest.Forbidden` 须先修正请求、资源标识或访问条件；`InvalidRequest.DataNotReady`、`500` 在 `error.retryable=true` 时可等待后重新读取。重试标记仅为 true 时出现在 CLI 回执中，不能从错误文案推断。CLI 不自动重试这两个入口，也不将读取重试提示用于重放写操作。

### 关联字段与跨表引用字段

创建 `lookup`（关联引用）和 `filterUp`（查找引用）字段时，config 格式有严格要求：

#### bidirectionalLink / unidirectionalLink（关联字段）

```bash
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "关联客户" --type bidirectionalLink \
  --config '{"linkedTableId":"<目标表tableId>","multiple":true}' --format json
```

#### lookup（关联引用，通过已有关联字段取值）

**前置条件**：本表必须已有一个 bidirectionalLink 或 unidirectionalLink 类型的关联字段。

```bash
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "客户城市" --type lookup \
  --config '{"associateField":"<本表关联字段的fieldId>","valuesField":"<关联目标表中要取值的字段fieldId>","aggregator":"CONCATENATE"}' --format json
```

config 必填字段：
- `associateField`：**本表中**已有的关联字段（bidirectionalLink/unidirectionalLink）的 fieldId
- `valuesField`：**关联目标表中**要取值的字段 fieldId
- `aggregator`：聚合方式，可选 `SUM`|`AVERAGE`|`COUNT`|`MAX`|`MIN`|`CONCATENATE`

> 常见错误：`associateField` 不是目标表的 tableId，也不是目标表的字段 ID，而是**本表中关联字段自身的 fieldId**。

#### filterUp（查找引用，无需关联字段，直接跨表取值）

```bash
# 基本用法：字段对常量匹配
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "客户总金额" --type filterUp \
  --config '{"targetSheet":"<目标表tableId>","filters":[{"fieldId":"<目标表字段Id>","operator":"equal","value":"匹配值","link":"AND"}],"valuesField":"<目标表中要取值的字段Id>","aggregator":"SUM"}' --format json

# 进阶用法：字段对字段动态匹配（currentSheetFieldId）
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "本城市订单金额" --type filterUp \
  --config '{"targetSheet":"<目标表tableId>","filters":[{"fieldId":"<目标表字段Id>","operator":"equal","currentSheetFieldId":"<本表字段Id>","link":"AND"}],"valuesField":"<目标表中要取值的字段Id>","aggregator":"SUM"}' --format json
```

config 必填字段：
- `targetSheet`：目标表的 tableId
- `filters`：至少一条筛选规则
  - `fieldId`：目标表中用于匹配的字段 fieldId
  - `operator`：仅支持 `equal`、`contain`（不支持 not_equal/not_contain）
  - `value`：常量匹配值（与 `currentSheetFieldId` 二选一）
  - `currentSheetFieldId`：本表中用于动态匹配的字段 fieldId（与 `value` 二选一，实现每行按本表字段值去目标表筛选）
  - `link`：多条件时的逻辑关系，`AND` 或 `OR`（单条件时可省略，多条件时建议显式指定；所有 filter 的 link 必须统一）
- `valuesField`：目标表中要取值的字段 fieldId
- `aggregator`：聚合方式，可选 `SUM`|`AVERAGE`|`COUNT`|`MAX`|`MIN`|`CONCATENATE`

## field update — 更新字段

```
Usage:
  dws aitable field update [flags]
Example:
  dws aitable field update --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --name "新字段名"
  dws aitable field update --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --config '{"options":[{"name":"A"},{"name":"B"}]}'
Flags:
      --base-id string    Base ID (必填)
      --config string     字段配置 JSON (不修改时省略)
      --ai-config string  AI 配置 JSON (不修改时省略)
      --field-id string   Field ID (必填)
      --name string       新字段名称 (不修改时省略)
      --table-id string   Table ID (必填)
```

- 不可变更字段类型
- 更新 singleSelect/multipleSelect 的 options 时需传入完整列表，已有选项应回传原 id
- `--name` / `--config` / `--ai-config` 至少传一个

## field delete — 删除字段

```
Usage:
  dws aitable field delete [flags]
Example:
  dws aitable field delete --base-id <BASE_ID> --table-id <TABLE_ID> --field-id <FIELD_ID> --yes
Flags:
      --base-id string    Base ID (必填)
      --field-id string   待删除字段 ID (必填)
      --table-id string   Table ID (必填)
```

不可逆。禁止删除主字段和最后一个字段。
