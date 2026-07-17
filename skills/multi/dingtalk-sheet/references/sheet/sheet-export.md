# 导出在线电子表格（export）

## 什么时候使用

用户要把钉钉在线电子表格（`axls`）导出为 `xlsx` 时使用本命令。服务端导出会保留格式、公式、合并单元格等属性，禁止用 `range read` 读全量数据后自行拼装 xlsx。

CLI 保留两种等价的创建入口：

- Schema 稳定主入口：`dws sheet export`
- 公开等价入口：`dws sheet export create`（新流程可用更明确的 create 语义）

两者接受相同的 `--node`、`--output` 和 `--async`，也执行同一套逻辑。已有任务必须使用 `dws sheet export get` 查询，不要重新提交导出。

## 创建导出任务

```bash
# 异步提交：立即返回任务 ID，不轮询、不下载
dws sheet export create --node <NODE_ID> --async --format json

# 同步等待：完成后返回 downloadUrl
dws sheet export create --node <NODE_ID> --format json

# 同步等待并下载到本地
dws sheet export create --node <NODE_ID> --output ./report.xlsx --format json

# 历史入口继续可用
dws sheet export --node <NODE_ID> --async --format json
```

异步模式只提交一次，输出统一任务结果：

```json
{
  "id": "job-1",
  "type": "export",
  "status": "PENDING",
  "message": "任务已提交，请稍后查询"
}
```

保存 `TaskResult.id`，稍后传给 `--task-id`。异步模式不会轮询，也不会因为指定了 `--output` 而下载文件。

同步模式保持历史 JSON 合同：

```json
{
  "success": true,
  "jobId": "job-1",
  "downloadUrl": "https://example.invalid/export.xlsx",
  "outputPath": "./report.xlsx"
}
```

未指定 `--output` 时没有 `outputPath`，也不会下载；`downloadUrl` 有时效性。同步模式由 CLI 内部按渐进式退避轮询，Agent 不要在外层重复轮询。

## 查询已有任务

```bash
dws sheet export get --task-id <TASK_ID> --format json
```

查询输出统一 `TaskResult`，状态含义如下：

| 状态 | 含义 | Agent 行为 |
|---|---|---|
| `PENDING` | 已排队 | 稍后用同一 `TaskResult.id` 查询 |
| `PROCESSING` | 正在导出 | 稍后查询，不要重新 create |
| `SUCCESS` | 导出成功 | 从 `resultUrl` 取得下载链接 |
| `FAILED` | 导出失败 | 转述 `message`，不要自动重提 |
| `TIMEOUT` | 查询窗口超时 | 保留 ID，稍后继续 get，不要自动重提 |

`FAILED` 和 `TIMEOUT` 会先输出结构化 `TaskResult`，随后命令以非零状态退出，便于 Agent 同时保留失败详情和正确处理退出码。

## 参数

| 参数 | 必填 | 说明 |
|---|---:|---|
| `--node` | 是 | 在线电子表格 ID 或 URL |
| `--async` | 否 | 提交成功后立即返回 `TaskResult.id` |
| `--output` | 否 | 同步成功后的本地文件或目录；不传则只返回下载链接 |
| `get --task-id` | 是 | `create --async` 或同步中断/超时留下的任务 ID |

限制：只支持在线电子表格（`axls`）导出为 xlsx。文字文档导出使用 `dws doc export create`。
