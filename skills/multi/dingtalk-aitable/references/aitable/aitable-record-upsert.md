# 行记录 Upsert（record upsert）

按 `recordId` 是否存在，自动把入参拆分到 update 链路或 create 链路：批次混合"已存在改 + 新出现建"时用，省掉客户端按 ID 分批的逻辑。

## 命令

```
dws aitable record upsert \
  --base-id BASE_ID --table-id TABLE_ID \
  --records '[{"recordId":"<可选>","cells":{...}}, ...]'
```

| flag | 说明 |
|------|------|
| `--base-id` | 必填（可用 `--base` 别名） |
| `--table-id` | 必填 |
| `--records` | 待 upsert 的记录 JSON 数组，**单次最多 100 条**（必填）|
| `--records-file` | 从文件读入（命令行 JSON 太长时用），与 `--records` 互斥优先级更高 |

## --records 结构

每项 JSON：

```jsonc
{
  "recordId": "rec1",                  // 可选；带 → update，缺省 → create
  "cells": {                            // 必填；key 是 fieldId，value 按字段类型
    "fldTitleId": "新标题",
    "fldNumberId": 42
  }
}
```

`cells` 写入格式与 `record create` / `record update` **完全一致**（key 必须是 fieldId 不是字段名；按字段类型见 [aitable-cell-value.md](./aitable-cell-value.md)）。

## 返回结构

```jsonc
{
  "data": {
    "createdRecordIds": ["recX", "recY"],   // 不带 recordId 的项产出
    "updatedRecordIds": ["recA", "recB"]    // 带 recordId 的项产出
  }
}
```

`createdRecordIds` 顺序对应入参里**不带 recordId**的项（按出现顺序汇总），同理 `updatedRecordIds` 对应**带 recordId**的项。

## 典型用法

```bash
# 1) 全部新建：所有项都不带 recordId
dws aitable record upsert --base-id BASE --table-id TBL --records '[
  {"cells":{"fldTitleId":"任务1","fldStatusId":"待办"}},
  {"cells":{"fldTitleId":"任务2","fldStatusId":"待办"}}
]'

# 2) 全部更新：所有项都带 recordId
dws aitable record upsert --base-id BASE --table-id TBL --records '[
  {"recordId":"rec1","cells":{"fldStatusId":"已完成"}},
  {"recordId":"rec2","cells":{"fldStatusId":"已完成"}}
]'

# 3) 混合：第 1 条更新（带 recordId），第 2 条创建（不带）
dws aitable record upsert --base-id BASE --table-id TBL --records '[
  {"recordId":"rec1","cells":{"fldStatusId":"已完成"}},
  {"cells":{"fldTitleId":"新增任务","fldStatusId":"待办"}}
]'

# 4) 长 JSON 用文件
dws aitable record upsert --base-id BASE --table-id TBL --records-file ./batch.json
```

## 与 record create / record update 的关系

| 场景 | 命令 |
|------|------|
| 确定全是新增 | `record create` |
| 确定全是更新（每条独立 cells） | `record update` |
| 确定全是更新（共享同一 cells） | `record batch-update` |
| **不确定有没有，按 recordId 自动分流** | `record upsert`（本命令） |

`record upsert` 的 `--records` 入参格式与 `record update` 完全相同，唯一差别是 `recordId` 字段在 upsert 里是可选的。如果批次确定全是更新或全是新建，用专用命令更清晰；批次混合时（典型场景：定时同步外部数据，源里既有已存在的也有新出现的），用 upsert。

## 注意事项

- **单次最多 100 条**（创建 + 更新合计），超出请客户端拆批。
- `cells` 的 key 必须是 fieldId 不是字段名（先用 `record query` 或 `field get` 拿 fieldId）。
- 只读字段（formula / lookup / 系统字段）不能写入 — upsert 链路与 update 链路同样限制。
