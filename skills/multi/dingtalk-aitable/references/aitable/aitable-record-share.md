# 行记录分享链接（record share-url）

按 recordId 批量获取记录的分享链接，把某行单独发给同事查看。

## 命令

```
dws aitable record share-url \
  --base-id BASE_ID --table-id TABLE_ID \
  --record-ids rec1,rec2,rec3 \
  [--view-id VIEW_ID]
```

| flag | 说明 |
|------|------|
| `--base-id` | 所属 Base ID（必填，可用 `--base` 别名） |
| `--table-id` | 所属 Table ID（必填） |
| `--record-ids` | 目标 Record ID 列表，CSV 逗号分隔，**单次最多 20 条**（必填） |
| `--view-id` | 视图 ID（可选）。带上后链接打开会落在该视图上下文里 |

## 返回结构

```jsonc
{
  "data": {
    "items": [
      { "recordId": "rec1", "shareUrl": "https://..." },
      { "recordId": "rec2", "shareUrl": "https://..." }
    ]
  }
}
```

`shareUrl` 为 null 表示该条获取失败（不影响其他条目）。

## 典型用法

```bash
# 一次拿一条记录的链接
dws aitable record share-url --base-id BASE --table-id TBL --record-ids rec1

# 批量拿，配合 jq 过滤出 url
dws aitable record share-url --base-id BASE --table-id TBL --record-ids rec1,rec2,rec3 --format json \
  | jq '.data.items[] | {recordId, shareUrl}'

# 带视图上下文（链接打开时落在指定视图）
dws aitable record share-url --base-id BASE --table-id TBL --record-ids rec1 --view-id viw_VIP
```

## 注意事项

- **单次最多 20 条**，超出请客户端拆批。
- 该链接是分享链接（不是源文档链接），打开后看到的是该 record 的只读详情页。
- 取消单条分享 / 关闭整表分享当前 CLI 不支持，需要在 AI 表格 Web 端操作。
