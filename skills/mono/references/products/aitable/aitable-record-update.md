# record update — 更新记录

## 命令格式

```
Usage:
  dws aitable record update [flags]
Example:
  dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]'
Flags:
      --base-id string        Base ID (必填)
      --records string        待更新记录 JSON 数组，单次最多 100 条 (必填，与 --records-file 二选一)
      --records-file string   从文件读取 records JSON（替代 --records，适合超长数据或 Windows 环境）
      --table-id string       Table ID (必填)
```

只需传入需修改的字段，未传入的保持原值。每条记录必须含 recordId 和 cells。

## 高频错误 flag（LLM 极易踩坑，必读）

CLI **没有** `--record-id` 和 `--cells` 两个独立 flag，**只接受 `--records` 一个参数**，格式为 JSON 数组。
即使只改一条记录，也必须包在数组里。

| 错误（LLM 直觉） | 正确 |
|---|---|
| `--record-id recXXX --cells '{"fldX":"值"}'` | `--records '[{"recordId":"recXXX","cells":{"fldX":"值"}}]'` |
| `--id recXXX --data '{"fldX":"值"}'` | 同上 |
| `--record-id recXXX --field fldX --value "新值"` | 同上 |

## 单条更新模板（直接复制）

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"<RECORD_ID>","cells":{"<FIELD_ID>":"新值"}}]' --format json
```

## 引号转义提示

- Linux/macOS：外层用单引号 `'[...]'`，内部 JSON 用双引号即可
- Windows PowerShell：外层用双引号 `"[...]"`，内部双引号需转义为 `\"`
- 或将 JSON 写入临时文件，用 `--records-file ./records.json` 规避转义
