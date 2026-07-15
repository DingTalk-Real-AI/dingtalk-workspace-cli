# 导入本地表格（import）

## 使用场景

用户说“导入 Excel”“把 xlsx 变成在线表格”“上传本地表格并在线编辑”时，使用 Agent 可发现入口 `dws sheet import create`。它只新建在线电子表格，不覆盖已有表格。旧入口 `dws sheet import` 继续兼容人工调用。

- 支持本地 `xlsx` / `xls`，文件上限 20MB
- `--folder-token` 与 `--workspace` 至少提供一个
- CLI 内部完成创建会话、OSS 上传、确认转换和轮询，Agent 不要自行拆分或重试
- `drive upload` 只上传二进制文件，不会转换为可编辑的在线表格，不能替代本命令

## 命令

```bash
# 导入到指定文件夹
dws sheet import create \
  --file ./quote.xlsx \
  --folder-token <FOLDER_TOKEN> \
  --format json

# 导入到指定知识库，并自定义名称
dws sheet import create \
  --file ./report.xls \
  --workspace <WORKSPACE_ID> \
  --name "月度报表" \
  --format json

# 导入超时或中断后续查
dws sheet import get --task-id <TASK_ID> --format json
```

公开参数：

| 命令 | 参数 | 说明 |
|------|------|------|
| `sheet import create` | `--file` | 本地 xlsx/xls 文件（必填） |
| `sheet import create` | `--folder-token` | 目标文件夹 ID 或 URL，与 `--workspace` 至少传一个 |
| `sheet import create` | `--workspace` | 目标知识库 ID 或 URL，与 `--folder-token` 至少传一个 |
| `sheet import create` | `--name`, `-n` | 导入后的名称；默认取文件名并去掉扩展名 |
| `sheet import get` | `--task-id` | 导入任务 ID |

## 返回与续查

成功时返回：

```json
{
  "success": true,
  "taskId": "<TASK_ID>",
  "documentUrl": "<DOCUMENT_URL>",
  "documentName": "月度报表",
  "documentType": "1",
  "nodeId": "<NODE_ID>"
}
```

轮询达到上限时命令以退出码 0 返回业务态，避免 Agent 重复创建文档：

```json
{
  "success": false,
  "timed_out": true,
  "taskId": "<TASK_ID>",
  "status": "processing",
  "next_command": "dws sheet import get --task-id <TASK_ID>"
}
```

检测到 `timed_out:true` 后，只执行 `next_command` 续查。不要重新执行 `sheet import create`。

## 边界

- 本命令接受的是本地文件路径；它把 xlsx/xls 转换成新的 axls 在线表格
- 已经存在于钉盘或文档中的 xlsx 节点不能直接传给工作表/单元格命令；先用 `doc download` 下载，需要在线编辑时再执行 `sheet import create`
- 向已有在线表格写数据应使用 `range update`、`append`、`csv-put` 或 `table-put`
- md/doc/docx 等文字文档导入使用 `dws doc import`
