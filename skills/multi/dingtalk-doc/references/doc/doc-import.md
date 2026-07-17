# doc import（本地文件导入为在线文档）

> **支持的文件格式**：docx, doc, xlsx, xls, md, txt, xmind, mark
> **文件大小限制**：20MB

---

## doc import（一体化命令）

```
Usage:
  dws doc import [flags]
Example:
  dws doc import --file ./report.docx
  dws doc import --file ./notes.md --folder <FOLDER_ID>
  dws doc import --file ./data.xlsx --workspace <WORKSPACE_ID>
  dws doc import --file ./draft.md --name "项目周报"
Flags:
      --file string        本地文件路径 (必填)
      --folder string      目标文件夹 ID 或 URL (可选，与 --workspace 至少传一个时优先)
      --workspace string   目标知识库 ID 或 URL (可选，不传时默认导入"知识库—我的文档"根目录)
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

```
Usage:
  dws doc import get [flags]
Example:
  dws doc import get --task-id <TASK_ID>
Flags:
      --task-id string   导入任务 ID (必填)
```

仅在 `dws doc import` 超时或中断后，用于手动查询任务状态。通常不需要调用。

## 关键说明

- `import` 是一体化命令，一条命令自动完成创建会话→上传→确认→轮询，**无需手动编排**。CLI 内部使用渐进式退避轮询（最多约 5 分钟）。
- `import` 超时或中断后，CLI 会输出 `taskId`，可用 `dws doc import get --task-id <taskId>` 手动查询任务状态。
- 支持的文件格式：docx, doc, xlsx, xls, md, txt, xmind, mark（共 8 种）。
- 文件大小限制：20MB。超过限制时 CLI 会直接报错，不会发起网络请求。
- `--folder` 和 `--workspace` 均不传时，默认导入到用户的"我的文档"根目录。
- `--folder` 优先级高于 `--workspace`：两者都传时以 `--folder` 为准。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `documentUrl` | 导入完成后的在线文档地址（可直接分享给用户） |
| `documentName` | 导入后的文档名称 |
| `documentType` | 文档类型（DOC / SHEET / MIND） |
| 中断时返回的 `taskId` | `import get` 的 `--task-id` |

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
dws doc import --file ./report.docx --format json

# 导入 Markdown 文件
dws doc import --file ./notes.md --format json

# 导入到指定文件夹
dws doc import --file ./data.xlsx --folder <FOLDER_ID> --format json

# 导入到知识库根目录
dws doc import --file ./plan.docx --workspace <WORKSPACE_ID> --format json

# 自定义文档名称
dws doc import --file ./draft.md --name "项目方案" --format json

# 导入 XMind 脑图文件
dws doc import --file ./mindmap.xmind --format json
dws doc import --file ./mindmap.xmind --workspace <WORKSPACE_ID> --format json

# 兜底：超时或中断后手动查任务
dws doc import get --task-id <TASK_ID> --format json
```

## 参考

- [`./doc-info.md`](./doc-info.md)（前置：理解文档标识）
