# AI 表格 Base 与 Table 操作

仅在根 Skill 已选中 Base/Table 生命周期操作，但还缺参数、返回字段或恢复语义时读取。不要与其他 Reference、产品 Catalog 或父级 Help 连读。

## 创建入口

| 目标 | 命令 | 成功后的稳定事实 |
|---|---|---|
| 只创建空白 Base | `dws aitable base create --name <名称> --format json` | 从真实回执取 `baseId` |
| 从模板创建 Base | `dws aitable +template-search --query <关键词>` → `dws aitable base create --name <名称> --template-id <T> --format json` | 只使用唯一选定的 templateId；创建后取返回的 baseId |
| 创建 Base、Table 和字段 | `dws aitable +base-bootstrap --name <名称> --tables '<JSON>' --format json` | `result.baseId`、`result.tables[].tableId`、`result.tables[].fields[]` |
| 在已有 Base 创建一张表 | `dws aitable +table-bootstrap --base-id <B> --name <表名> --fields '<JSON>' --format json` | `result.tableId` 与 `result.fields[]` |

只创建 Base 时不得给 `+base-bootstrap` 传空 `tables`，也不得创建临时表再删除。`base create` 的可选目标参数只有当前 leaf Schema 已发布的 `--folder-id` 和 `--template-id`。

## 返回 ID 与验证

- `base create` 成功后，直接执行 `dws aitable +base-get --base-id <返回的baseId> --format json`；禁止立即用名称搜索替代稳定 ID 回读。
- `+base-bootstrap` 和 `+table-bootstrap` 已在命令内部按返回 ID 读回验证；成功后不要再执行 `base search`、`table get` 或 `field list`。直接复用回执中的 ID。
- `+base-bootstrap` 的每张表返回 `tableId/name/fieldCount/fields`；`fields` 只保留后续写记录所需的 `fieldId/fieldName/type`，不会携带完整 config。
- 创建响应缺少稳定 ID 时属于未知写入状态：按错误中的 checkpoint/next command 核验，未确认前不得重放非幂等创建。

## 结构 JSON

```json
[
  {
    "name": "任务",
    "fields": [
      {"fieldName": "标题", "type": "text"},
      {"fieldName": "状态", "type": "singleSelect", "config": {"options": [{"name": "待办"}]}}
    ]
  }
]
```

Base bootstrap 的表对象使用 `name`；字段使用 `fieldName/type/config`。已有 Base 的 table bootstrap 只传上述 `fields` 数组。

## 查看与恢复

- 已知 baseId：`+base-get --base-id <B>`；只列 Table：`+list-tables --base <B>`。
- 已知 tableId：`+table-get --base-id <B> --table-id <T>`。
- 名称查候选：`+base-search --query <关键词>`；需要唯一名称解析才用 `+resolve-base` / `+resolve-table`，两条路径不串行试探。
- `partial_success` 只从 checkpoint 继续；`retryable=false`、目标类型错误或返回 ID 不一致时停止，不换相似命令或其他 ID 类型。
