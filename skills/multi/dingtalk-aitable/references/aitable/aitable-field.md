# field — 字段管理

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

返回字段的完整配置（含 options 等）。在 table get 拿到字段目录后，按需展开少量字段的完整配置。

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
      --type string       要创建的单字段类型（参考 table create 字段类型）
      --config string     单字段配置，如 options（可选）
      --ai-config string  单字段 AI 配置 JSON（可选，用于创建 AI 字段）
      --fields string     批量新增字段 JSON 数组，单次最多 15 个 (与 --name/--type 二选一)
      --table-id string   Table ID (必填)
```

允许部分成功，返回结果逐项标明成功/失败状态。

### AI 字段创建示例

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
