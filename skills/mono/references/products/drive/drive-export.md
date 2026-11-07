# drive 通用导出（export / export get）

> **前置条件（MUST READ）：** 执行本组命令前，必须先用 Read 工具读取以下文件：
> 1. [`../drive.md`](../drive.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

---

## 通用导出（export）

```
Usage:
  dws drive export [flags]
Example:
  dws drive export --node <DOC_ID> --output ./exported.docx
  dws drive export --node <DOC_ID> --export-format pdf --output ./exported.pdf
  dws drive export --node <DOC_ID> --output ~/downloads/
  dws drive export --node <DOC_ID> --async
Flags:
      --node string          要导出的文档标识，支持 URL 或 dentryUuid (必填)
      --export-format string 导出格式: docx (默认) / xlsx / markdown (别名 md) / pdf / pptx
      --output string        本地保存路径，文件路径或目录 (可选)
      --async                异步模式：仅提交任务并立即返回 taskId，不轮询、不下载
```

一体化命令，CLI 内部自动完成三阶段：提交导出任务 → 渐进式退避轮询（2s×5 → 5s×5 → 10s×10 → 15s×10，上限 30 次约 5 分钟）→ 成功后自动下载。**只需一条命令，无需手动编排轮询。**

- **格式自动探测**：未指定 `--export-format` 时按文档扩展名选择默认格式——adoc→docx、axls→xlsx、appt→pptx；探测失败回退 docx。
- **`--output` 可选**：未指定时导出成功后仅返回 `taskId` 与 `downloadUrl`（链接有时效性，请尽快下载）。传目录时自动推断文件名并对齐扩展名。
- **`--async`**：只提交任务立即返回 taskId（TaskResult JSON），后续用 `dws drive task get --type export --id <taskId>` 查询。
- 轮询超时或 Ctrl-C 中断时输出 taskId，服务端任务不会中止，之后可手动查询。

## export get（手动兜底查询）

```
Usage:
  dws drive export get --task-id <TASK_ID>
Example:
  dws drive export get --task-id <TASK_ID>
Flags:
      --task-id string   导出任务 ID (必填)
```

仅在 `dws drive export --async` 提交后或轮询超时/中断后，用于手动查询导出任务状态（仅查询一次，不轮询）。非导出任务类型请改用 `dws drive task get`。

## 导出命令选择

| 场景 | 优先命令 |
|------|---------|
| 明确是在线文档（adoc），导出 docx/markdown/pdf | `dws doc export` |
| 明确是在线表格（axls），导出 xlsx | `dws sheet export` |
| 文档类型不确定，或需要导出 xlsx/pptx | `dws drive export`（通用入口） |

## 参考

- [`../drive.md` §意图判断](../drive.md#意图判断)（如何路由到本组命令）
