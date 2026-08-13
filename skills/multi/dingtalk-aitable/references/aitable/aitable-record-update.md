# record update — 更新记录

> 加载边界：仅在 recordId 和待修改 fieldId 已确定后读取。只提交用户要求变更的 cells；写前读取字段类型/只读性，写后使用返回 recordId 回读。未返回 ID、字段未变化或状态未知都不能声称成功。

## 命令格式

```
Usage:
  dws aitable record update [flags]
Example:
  dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]'
Flags:
      --base-id string        Base ID (必填)
      --records string        待更新记录 JSON 数组，单次最多 100 条；cells key 支持 fieldId 或当前表内唯一字段名，推荐 fieldId (必填，与 --records-file 二选一)
      --records-file string   从文件读取 records JSON（替代 --records，适合超长数据或 Windows 环境）
      --table-id string       Table ID (必填)
```

只需传入需修改的字段，未传入的保持原值。每条记录必须含 recordId 和 cells。

## cells key：自动化与 Agent 路径使用 fieldId

公开稳定的 Agent/自动化写法是 `fieldId`：它不受字段重命名或重名影响，并可通过 `field get` 获取。

Runtime 仍兼容当前表内唯一字段名，但这属于便捷兼容路径：重名或重命名会改变解析结果，不能用于脚本、跨步骤调用或从旧上下文重放。字段名与 fieldId 同时出现时也不要依赖覆盖顺序。

```bash
# 推荐：fieldId
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]' --format json

```

## 推荐参数形式

公开、稳定的批量入口是 `--records`（或 `--records-file`），格式为 JSON 数组；即使只改一条记录，推荐也包在数组里。CLI 仍保留隐藏的 `--record-id` + `--cells` 兼容入口，但它不会出现在常规帮助中，自动化脚本应优先使用 `--records`。

| 不推荐或无效写法 | 推荐写法 |
|---|---|
| `--record-id recXXX --cells '{"fldX":"值"}'`（隐藏兼容入口） | `--records '[{"recordId":"recXXX","cells":{"fldX":"值"}}]'` |
| `--id recXXX --data '{"fldX":"值"}'` | 同上 |
| `--record-id recXXX --field fldX --value "新值"` | 同上 |

## 单条更新模板（直接复制）

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"<RECORD_ID>","cells":{"<FIELD_ID>":"新值"}}]' --format json

# 从更新响应的 data.recordIds[] 提取成功记录 ID，并回读确认真实值
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> \
  --record-ids <RECORD_ID> --format json
```

更新响应不返回“受影响字段”；以 `data.recordIds[]` 确定成功记录，再用查询回读验证。

## 引号转义提示

- Linux/macOS：外层用单引号 `'[...]'`，内部 JSON 用双引号即可
- Windows PowerShell：外层用双引号 `"[...]"`，内部双引号需转义为 `\"`
- 或将 JSON 写入临时文件，用 `--records-file ./records.json` 规避转义
