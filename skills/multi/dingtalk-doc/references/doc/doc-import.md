# doc import（本地文件导入为在线文档）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> **支持的文件格式**：docx, doc, xlsx, xls, md, txt, xmind, mark
> **文件大小限制**：20MB
> **不支持转换的格式**：白名单外的所有格式（html/pdf/zip/无扩展名等）不报错，CLI 自动改走文件上传链路，以原文件形式存入 `--folder`/`--workspace` 指定的文档空间/知识库位置（同样受 20MB 上限约束）；JSON 结果带 `fallback=upload`、`converted=false` 标记，表示产出文件对象而非在线文档；需要"可编辑在线文档"时先把内容转为 md 再导入；目标是钉盘时直接用 `dws drive upload`。

---

## doc import（一体化命令）

```
Usage:
  dws doc import [flags]
Example:
  dws doc import --file ./report.docx --workspace <WORKSPACE_ID>
  dws doc import --file ./notes.md --folder <FOLDER_ID>
  dws doc import --file ./data.xlsx --workspace <WORKSPACE_ID>
  dws doc import --file ./draft.md --workspace <WORKSPACE_ID> --name "项目周报"
Flags:
      --file string        本地文件路径 (必填)
      --folder string      目标文件夹 ID 或 URL (与 --workspace 必填其一)
      --workspace string   目标知识库 ID 或 URL (与 --folder 必填其一)
      --name string        导入后文档名称 (可选，默认取文件名不含扩展名)
```

> 注意：`--folder` 接受文件夹的 dentryUuid（nodeId）或 alidocs 文件夹 URL。
> 如果通过 `dws doc info` 获取信息，应使用返回的 `nodeId` 字段，
> 而非 `folderId`（后者是父文件夹 ID）。
> 当 URL 指向的节点 `nodeType` 不是 "folder" 时，应提示用户检查链接。

CLI 内部自动完成：创建导入会话 → 上传文件到 OSS → 确认导入触发转换 → 渐进式退避轮询（最多约 5 分钟）。
**只需一条命令，无需手动轮询。**

---

## doc import get（手动兜底查询任务）

> 任务状态：processing（转换中）/ completed（导入成功，返回 documentUrl）/ failed（导入失败）。

```
Usage:
  dws doc import get [flags]
Example:
  dws doc import get --task-id <TASK_ID>
Flags:
      --task-id string   导入任务 ID (必填)
```

仅在 `dws doc import` 超时或中断后，用于手动查询任务状态。通常不需要调用。

**输出示例**（统一 TaskResult 格式）：
```json
{"id":"task-abc123","type":"import","status":"SUCCESS","resultUrl":"https://alidocs.dingtalk.com/i/nodes/xxx","message":"","createTime":"2024-01-15T10:30:00Z"}
```

**处理中示例**：
```json
{"id":"task-abc123","type":"import","status":"PROCESSING","resultUrl":"","message":"任务处理中，请稍后查询","createTime":"2024-01-15T10:30:00Z"}
```

## 关键说明

- `import` 是一体化命令，一条命令自动完成创建会话→上传→确认→轮询，**无需手动编排**。CLI 内部使用渐进式退避轮询（最多约 5 分钟）。
- `import` 超时或中断后，CLI 会输出 `taskId`，可用 `dws doc import get --task-id <taskId>` 手动查询任务状态。
- 支持的文件格式：docx, doc, xlsx, xls, md, txt, xmind, mark（共 8 种）。
- 白名单外格式（html/pdf/zip/无扩展名等）不报错：自动改走文件上传链路，以原文件形式存入目标文档空间/知识库（stderr 有改道提示，同样受 20MB 上限约束），JSON 结果带 `fallback=upload`、`converted=false` 标记——返回文件对象而非在线文档，不得报告为"已转换"；要"可编辑在线文档"时先转 md 再导入，目标是钉盘时用 `dws drive upload`。
- 文件大小限制：20MB。超过限制时 CLI 会直接报错，不会发起网络请求。
- `--folder` 和 `--workspace` 至少传一个，不传会报错。如需导入到"我的文档"根目录，请先获取 workspaceId：`dws wiki space list --type myWikiSpace --format json`，然后传 `--workspace <workspaceId>`。
- `--folder` 优先级高于 `--workspace`：两者都传时以 `--folder` 为准。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `documentUrl` | 导入完成后的在线文档地址（可直接分享给用户） |
| `documentName` | 导入后的文档名称 |
| `documentType` | 文档类型（DOC / SHEET / MIND） |
| 超时/中断时返回的 `taskId` | `import get` 的 `--task-id`（查询后取 documentUrl 获取文档地址） |

## 输出规范

导入完成后的输出必须遵循以下规则：

- **返回在线文档 URL**：导入成功后，直接告知用户文档已导入并提供在线文档链接
- **输出格式**：`文件已导入为在线文档: <documentUrl>`
- 正确示例：`文件已导入为在线文档: https://alidocs.dingtalk.com/i/nodes/xxx`

## 格式与文档类型映射

| 文件扩展名 | 导入后文档类型 | 说明 |
|-----------|--------------|------|
| `.docx` / `.doc` | DOC（文字文档） | Word 文档导入为在线文档 |
| `.xlsx` / `.xls` | SHEET（电子表格） | Excel 文件导入为在线表格 |
| `.md` / `.txt` | DOC（文字文档） | Markdown/纯文本导入为在线文档 |
| `.xmind` | MIND（脑图） | XMind 文件导入为在线脑图 |
| `.mark` | MIND（脑图） | Mark 格式导入为在线脑图 |

## 常用模板

```bash
# 导入 Word 文档（最常用）
dws doc import --file ./report.docx --workspace <WORKSPACE_ID> --format json

# 导入 Markdown 文件
dws doc import --file ./notes.md --workspace <WORKSPACE_ID> --format json

# 导入到指定文件夹
dws doc import --file ./data.xlsx --folder <FOLDER_ID> --format json

# 导入到知识库根目录
dws doc import --file ./plan.docx --workspace <WORKSPACE_ID> --format json

# 自定义文档名称
dws doc import --file ./draft.md --workspace <WORKSPACE_ID> --name "项目方案" --format json

# 导入 XMind 脑图文件
dws doc import --file ./mindmap.xmind --workspace <WORKSPACE_ID> --format json

# 兜底：超时或中断后手动查任务
dws doc import get --task-id <TASK_ID> --format json

# 异步模式：提交后立即返回，不等待格式转换完成
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-info.md`](./doc-info.md)（前置：理解文档标识）
