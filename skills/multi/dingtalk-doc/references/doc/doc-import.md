# 本地文件导入为在线文档 (doc import)

## 使用场景

用户说"导入 Word/Excel/Markdown/xmind 到钉钉文档"、"把本地文件转成在线文档"、"导入到知识库/文件夹"时，使用 `dws doc import`。

不要先读取文件内容再调用 `doc create` 或 `doc update`。`doc import` 会按文件格式走导入任务，保留更完整的原始结构。

## 命令

```bash
dws doc import --file ./report.docx --format json
dws doc import --file ./notes.md --folder <FOLDER_ID> --format json
dws doc import --file ./data.xlsx --workspace <WORKSPACE_ID> --format json
dws doc import --file ./draft.md --name "项目周报" --format json
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
| `--task-id` | `import get` 查询导入任务时必填 |

支持格式：docx、doc、xlsx、xls、md、txt、xmind、mark。文件大小上限 20MB。

## 工作流

1. 确认本地文件存在且格式受支持。
2. 如果用户指定目标知识库或文件夹，传 `--workspace` 或 `--folder`；未指定时导入到默认位置。
3. 执行 `dws doc import --file ... --format json`。
4. 正常情况下 CLI 会自动提交、上传并轮询导入任务。
5. 如果命令超时或中断，从输出中提取 `taskId`，再执行 `dws doc import get --task-id <TASK_ID> --format json`。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `doc import` | `nodeId` / `documentUrl` / `documentName` / `documentType` | 后续 `doc read` / `doc info` / `sheet` 操作 |
| `doc import` 中断 | `taskId` | `doc import get --task-id` 查询任务状态 |

## 注意事项

- `--folder` 和 `--workspace` 都是目标位置参数；用户明确指定文件夹时优先使用 `--folder`。
- 导入 Excel 后通常得到在线表格，后续数据读取和编辑走 `dws sheet ...`。
- 导入 Markdown 或 Word 后通常得到在线文档，后续内容读取和编辑走 `dws doc ...`。
