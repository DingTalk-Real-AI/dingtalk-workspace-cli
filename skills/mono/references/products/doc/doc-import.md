# 本地文件导入为在线文档 (doc import create)

## 使用场景

用户说"导入 Word/Excel/Markdown/xmind 到钉钉文档"、"把本地文件转成在线文档"、"导入到知识库/文件夹"时，使用 `dws doc import create`。

不要先读取文件内容再调用 `doc create` 或 `doc update`。`doc import create` 会按文件格式走导入任务，保留更完整的原始结构。

## 命令

```bash
dws doc import create --file ./report.docx --format json
dws doc import create --file ./notes.md --folder <FOLDER_ID> --format json
dws doc import create --file ./data.xlsx --workspace <WORKSPACE_ID> --format json
dws doc import create --file ./draft.md --name "项目周报" --format json
dws doc import create --file ./report.docx --async --format json
```

```bash
dws doc import get --task-id <TASK_ID> --format json
```

## 参数

| 参数 | 说明 |
|------|------|
| `--file` | 本地文件路径，必填 |
| `--folder` | 目标文件夹 ID 或 URL，可选 |
| `--workspace` | 目标知识库 ID 或 URL，可选 |
| `--name` / `-n` | 导入后的文档名称；不传时使用文件名 |
| `--async` | 完成会话创建、文件上传和导入确认后返回 `PENDING`，不轮询 |
| `--task-id` | `import get` 查询导入任务时必填 |

支持格式：docx、doc、xlsx、xls、md、txt、xmind、mark。文件大小上限 20MB。

## 工作流

1. 确认本地文件存在且格式受支持。
2. 如果用户指定目标知识库或文件夹，传 `--workspace` 或 `--folder`；未指定时导入到默认位置。
3. 执行 `dws doc import create --file ... --format json`；需要立即返回任务 ID 时加 `--async`。
4. 无论同步还是异步，CLI 都会创建导入会话、上传文件并确认导入，因此异步模式仍会创建在线文档。
5. 异步模式只输出一个 `PENDING` TaskResult，从 `TaskResult.id` 取得任务 ID；CLI 不调用查询接口。
6. 同步模式继续轮询直到完成；如果超时或中断，保存提示中的任务 ID。
7. 使用 `dws doc import get --task-id <TASK_ID> --format json` 查询；`SUCCESS` 的 `resultUrl` 是创建后的文档链接，`FAILED` / `TIMEOUT` 会先输出 TaskResult 再返回错误。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `doc import create`（同步） | `nodeId` / `documentUrl` / `documentName` / `documentType` | 后续 `doc read` / `doc info` / `sheet` 操作 |
| `doc import create --async` | `TaskResult.id` | `doc import get --task-id` 查询任务状态 |
| `doc import create` 超时或中断 | 提示中的任务 ID | `doc import get --task-id` 查询任务状态 |

## 注意事项

- `--folder` 和 `--workspace` 都是目标位置参数；用户明确指定文件夹时优先使用 `--folder`。
- `--format json` 不会把进度行混入标准输出，适合自动化解析。
- 导入 Excel 后通常得到在线表格，后续数据读取和编辑走 `dws sheet ...`。
- 导入 Markdown 或 Word 后通常得到在线文档，后续内容读取和编辑走 `dws doc ...`。
