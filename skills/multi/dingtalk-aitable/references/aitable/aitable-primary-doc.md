# 主键文档管理

## 适用场景

当需要为 AI 表格中的记录创建或查询关联的主键文档时使用。主键文档是 primaryDoc 类型字段对应的钉钉在线文档，可通过 `dws doc` 进行内容读写。

## 命令

### 查询主键文档

```bash
dws aitable record primary-doc-get --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID
```

**参数：**
- `--base-id`（必填）：Base ID
- `--table-id`（必填）：Table ID
- `--record-id`（必填）：Record ID

**返回：** `data.nodeId` — 主键文档的 nodeId，可直接传给 `dws doc read/update` 的 `--node` 参数。若该记录尚未创建主键文档，`nodeId` 为 null。

### 创建主键文档

```bash
dws aitable record primary-doc-create --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --record-id RECORD_ID
```

**参数：**
- `--base-id`（必填）：Base ID
- `--table-id`（必填）：Table ID
- `--field-id`（必填）：主键字段 ID，必须是 primaryDoc 类型（通过 `dws aitable table get` 查看字段类型）
- `--record-id`（必填）：Record ID

**返回：** `data.nodeId` — 创建或已存在的主键文档 nodeId。

**幂等性：** 若该记录已有主键文档，直接返回已有文档的 nodeId，不会重复创建。

## 注意事项

- `fieldId` 必须是 primaryDoc 类型，否则返回 `INVALID_FIELD_TYPE` 错误
- 传入不存在的 `recordId` 会返回 `RECORD_NOT_FOUND` 错误
- 创建后可通过 `dws doc update --node <nodeId>` 写入文档内容，或 `dws doc read --node <nodeId>` 读取

## 典型工作流

```bash
# 1. 查询表结构，拿到 primaryDoc 字段的 fieldId
dws aitable table get --base-id BASE_ID --table-ids TABLE_ID

# 2. 为某条记录创建主键文档
dws aitable record primary-doc-create --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --record-id RECORD_ID

# 3. 拿到返回的 nodeId，用 dws doc 写入内容
dws doc update --node <data.nodeId> --content "# 项目方案\n\n文档正文内容..."
```
