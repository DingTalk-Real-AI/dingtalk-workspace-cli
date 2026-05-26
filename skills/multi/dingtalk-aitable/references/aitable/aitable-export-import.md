# export & import — 导入导出

## 导出数据（两阶段轮询）

`export data` 为异步任务：首次调用可能只返回 `taskId`，需要继续轮询。

> ⚠️ **`--format` 冲突警告**：`export data` 的 `--format` 是**导出格式**（excel/attachment 等），不是全局输出格式。**此命令禁止追加全局 `--format json`**，否则会覆盖导出格式导致 `INVALID_EXPORT_FORMAT` 错误。输出默认就是 JSON，无需额外指定。

```bash
# 第一步：创建任务（按 scope 传必要参数）——注意：不要加 --format json！
dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --format excel --timeout-ms 1000

# 第二步：拿 taskId 继续轮询，直到返回 downloadUrl
dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000
```

### 参数约束

| scope | 必传参数 |
|-------|----------|
| `all` | 只需 `--base-id` |
| `table` | 必须 `--table-id` |
| `view` | 必须 `--table-id` + `--view-id` |

## 导入文件（三步流程）

当用户要求将 Excel（`.xlsx`）或 CSV 文件完整导入 AI 表格时，**不需要自己解析文件内容**，直接使用文件级导入。

> **无需手动解析 CSV/Excel 再逐条 record create**，效率极低且容易出错。

```bash
# 第 1 步：申请上传凭证
dws aitable import upload --base-id <BASE_ID> \
  --file-name data.xlsx --file-size <字节数> --format json
# → 返回 uploadUrl 和 importId

# 第 2 步：上传文件到 OSS（注意：Content-Type 必须设为空）
curl -X PUT "<uploadUrl>" -H "Content-Type:" --data-binary @data.xlsx

# 第 3 步：触发导入
dws aitable import data --import-id <importId> --format json
# → 返回 status: success 和新建的 tableIds
```

### 步骤说明

| 步骤 | 命令 | 说明 |
|------|------|------|
| 申请上传凭证 | `import upload --base-id <ID> --file-name <名称> --file-size <字节>` | `--file-size` 必须与实际文件大小一致 |
| 上传文件 | HTTP PUT（curl 等） | **必须** 带 `-H "Content-Type:"` 将 Content-Type 设为空，否则 OSS 返回 403 |
| 触发导入 | `import data --import-id <ID>` | 同步等待，大多一次调用即返回结果；超时可用相同 importId 重试 |

### 适用/不适用场景

- **适用**：xlsx / csv 文件级别导入。导入后每个 Sheet 自动建表
- **不支持**：追加到已有表、选择部分字段导入
- **不适用**：需要写入已有表的特定列 → 应使用 `record create`
