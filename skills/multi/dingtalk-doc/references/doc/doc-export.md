# doc export（在线文档导出为 docx / markdown / pdf）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> **路由前置判断**：用户说「下载/导出」时**必须**先用 [`./doc-info.md`](./doc-info.md) `info --node <ID> --format json` 查 `contentType`：
> - `contentType` 为 `ALIDOC`（在线文档）→ **必须用 `export`**，禁止用 `download`
> - `contentType` 为 `DOCUMENT`/`IMAGE`/`VIDEO` 等（已有文件）→ 用 `dws drive download`（详见 [`../drive.md`](../drive.md)）
>
> `drive download` 只能下载**已有文件**（原样下载），`export` 是将**在线文档格式转换**后导出为 docx、markdown 或 pdf，两者完全不同。

---

## doc export / doc export create

```
Usage:
  dws doc export create [flags]
  dws doc export [flags]  # 历史兼容入口
Example:
  dws doc export create --node "https://alidocs.dingtalk.com/i/nodes/xxx" --output ./exported.docx
  dws doc export create --node <DOC_ID> --export-format markdown --output ./exported.md
  dws doc export create --node <DOC_ID> --export-format pdf --async --format json
Flags:
      --node string           要导出的文档标识，支持文档 URL 或 dentryUuid (必填)
      --output string         本地保存路径，文件路径或目录（同步模式必填；--async 时可省略）
      --export-format string  导出格式: docx / markdown / md / pdf (默认 docx)
      --async                 异步模式：提交成功后立即返回任务，不轮询或下载
```

- 默认同步模式：提交导出任务 → 渐进式退避轮询（最多约 5 分钟）→ 成功后下载到 `--output`。
- `--async` 模式：提交成功后只输出一个 `PENDING` 任务结果，其中 `id` 是后续查询使用的 task ID；不会轮询或下载。
- `doc export create` 是公开主入口；`doc export` 保留为兼容入口，行为相同。

---

## doc export get（查询任务）

```
Usage:
  dws doc export get [flags]
Example:
  dws doc export get --task-id <TASK_ID>
  dws doc export get --job-id <JOB_ID>
Flags:
      --task-id string  导出任务 ID（与 --job-id 二选一）
      --job-id string   导出任务 ID（历史参数；与 --task-id 二选一）
```

用于查询 `--async` 返回的任务，或同步导出超时、中断后遗留的任务。`--task-id` 是新流程推荐写法；公开的历史 `--job-id` 继续可用，两者不可同时传入。

## 关键说明

- 默认同步模式会自动提交、轮询和下载；`--async` 只提交任务并立即返回，不会下载文件。
- 返回任务结果中的 `id` 可直接传给 `dws doc export get --task-id <TASK_ID>`。
- `export` 支持钉钉在线文档（alidocs，`contentType=ALIDOC`）导出为 `docx`、`markdown`（`md`）或 `pdf`；在线表格导出请使用其他命令。
- 同步模式下 `--output` 必填，既可以是文件完整路径，也可以是目录；异步模式可省略。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `localPath` | 用户可访问的本地文件路径 |
| 任务结果的 `id` | `export get` 的 `--task-id` |

## 常用模板

```bash
# 同步导出（最常用）
dws doc export create --node <DOC_ID> --output ./exported.docx

# 导出为 markdown
dws doc export create --node <DOC_ID> --export-format markdown --output ./exported.md

# 异步提交（返回 TaskResult.id，不轮询或下载）
dws doc export create --node <DOC_ID> --export-format pdf --async --format json

# 查询异步、超时或中断任务
dws doc export get --task-id <TASK_ID> --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-info.md`](./doc-info.md)（前置：判断 contentType=ALIDOC 才走 export）
- [`../drive.md`](../drive.md)（非 ALIDOC 文件用 `dws drive download`）
