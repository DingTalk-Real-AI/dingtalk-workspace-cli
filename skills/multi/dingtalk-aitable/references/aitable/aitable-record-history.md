# 行记录变更历史（record history-list）

按 recordId 查询单条记录的全部变更历史，用于审计、回溯字段变更、定位操作人。

## 命令

```
dws aitable record history-list \
  --base-id BASE_ID --table-id TABLE_ID --record-id REC_ID \
  [--offset N] [--limit M]
```

| flag | 说明 |
|------|------|
| `--base-id` | 所属 Base ID（必填，可用 `--base` 别名） |
| `--table-id` | 所属 Table ID（必填） |
| `--record-id` | 目标记录 ID（必填，单条；不支持批量） |
| `--offset` | 分页偏移量，默认 0 |
| `--limit` | 每页返回数量，范围 [1, 50]，默认 20 |

## 返回结构

```jsonc
{
  "data": {
    "histories": [
      {
        "type": "field_change",           // 变更类型
        "action": "update",               // 操作动作: create / update / delete
        "newValue": "{\"...\":\"...\"}",  // 变更后的值（JSON 字符串）
        "oldValue": "{\"...\":\"...\"}",  // 变更前的值（JSON 字符串）
        "operateTime": 1733123456789,     // 操作时间（毫秒级时间戳）
        "typeChangedFields": "{...}",     // 类型变更的字段信息（JSON 字符串）
        "version": 7                      // 版本号（单调递增）
      }
    ]
  }
}
```

`newValue` / `oldValue` / `typeChangedFields` 是 JSON 字符串（不是 JSON 对象），需要二次 `JSON.parse` 才能拿到结构化值。

## 字段含义速查

| 字段 | 用途 |
|------|------|
| `type` | 高层分类：`record_create` / `field_change` / `record_delete` 等。先按 type 过滤大类。 |
| `action` | 三态：`create` / `update` / `delete`。比 type 粗，但便于按"动作"统计。 |
| `version` | 单调递增整数；同一 record 越新值越大。**用作"上一条 vs 这一条"的稳定排序键**。 |
| `operateTime` | 毫秒时间戳；可格式化成可读时间。多条同 version 的极端场景用 operateTime 兜底排序。 |

## 典型用法

### 1. 看一条记录被改过几次

```bash
dws aitable record history-list --base-id BASE --table-id TBL --record-id REC --format json \
  | jq '.data.histories[] | {version, action, operateTime}'
```

### 2. 翻页拉全量历史

```bash
# 第 1 页（最新 20 条）
dws aitable record history-list --base-id BASE --table-id TBL --record-id REC --limit 50 --offset 0

# 第 2 页
dws aitable record history-list --base-id BASE --table-id TBL --record-id REC --limit 50 --offset 50
```

`limit` 上限 50，需要更多请增加 `offset` 翻页。

### 3. 回溯某字段最近一次值

```bash
dws aitable record history-list --base-id BASE --table-id TBL --record-id REC --limit 50 --format json \
  | jq '[.data.histories[] | select(.action == "update")][0].oldValue'
```

### 4. 找出删除事件（如果存在 delete history）

```bash
dws aitable record history-list --base-id BASE --table-id TBL --record-id REC --format json \
  | jq '.data.histories[] | select(.action == "delete") | {version, operateTime}'
```

## 注意事项

- 一次只能查一条 record；如需批量审计多条记录请循环调用。
- 仅返回**字段值变更**与**记录生命周期事件**；视图、字段定义、表结构变更不在此 history 里。
- 历史保留时长由 server 决定，过老的记录可能不再返回。

## 与其他 record 命令的关系

- 想看记录"现在长什么样" → `record query` / `record get`
- 想看记录"过去长什么样、什么时候改的" → `record history-list`（本命令）
- 想看"这张表整体改过什么" → 当前 CLI 不支持表级 history；只能逐 record 查
