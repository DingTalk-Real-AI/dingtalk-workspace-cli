# doc export（在线文档导出为 docx）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> **路由前置判断**：用户说「下载/导出」时**必须**先用 [`./doc-info.md`](./doc-info.md) `info --node <ID> --format json` 查 `contentType`：
> - `contentType` 为 `ALIDOC`（在线文档）→ **必须用 `export`**，禁止用 `download`
> - `contentType` 为 `DOCUMENT`/`IMAGE`/`VIDEO` 等（已有文件）→ 用 `dws drive download`（详见 [`../drive.md`](../drive.md)）
>
> `drive download` 只能下载**已有文件**（原样下载），`export` 是将**在线文档格式转换**后导出为 docx，两者完全不同。

---

## doc export（一体化命令）

```
Usage:
  dws doc export [flags]
Example:
  dws doc export --node "https://alidocs.dingtalk.com/i/nodes/xxx" --output ./exported.docx
  dws doc export --node <DOC_ID> --output ~/downloads/
Flags:
      --node string           要导出的文档标识，支持文档 URL 或 dentryUuid (必填)
      --output string         本地保存路径，文件路径或目录 (必填)
      --export-format string  导出格式，当前仅支持 docx (默认)
```

CLI 内部自动完成：提交导出任务 → 渐进式退避轮询（最多约 5 分钟）→ 成功后自动下载文件。
**只需一条命令，无需手动轮询。**

---

## doc export get（手动兜底查询任务）

```
Usage:
  dws doc export get [flags]
Example:
  dws doc export get --job-id <JOB_ID>
Flags:
      --job-id string   导出任务 ID (必填)
```

仅在 `dws doc export` 超时或中断后，用于手动查询任务状态。通常不需要调用。

## 关键说明

- `export` 是一体化命令，一条命令自动完成提交→轮询→下载，**无需手动编排轮询**。CLI 内部使用渐进式退避轮询（最多约 5 分钟）。
- `export` 超时或中断后，CLI 会输出 `jobId`，可用 `dws doc export get --job-id <jobId>` 手动查询任务状态。
- `export` 当前仅支持钉钉在线文档（alidocs，`contentType=ALIDOC`）导出为 `docx`，**在线表格导出请使用其他命令**。
- `--output` 既可以是文件完整路径，也可以是目录（CLI 自动按文档名生成 `.docx`）。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `localPath` | 用户可访问的本地文件路径 |
| 中断时返回的 `jobId` | `export get` 的 `--job-id` |

## 常用模板

```bash
# 一体化导出（最常用）
dws doc export --node <DOC_ID> --output ./exported.docx

# 输出到目录（自动按文档名命名）
dws doc export --node <DOC_ID> --output ~/downloads/

# alidocs URL 直传
dws doc export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --output ./exported.docx

# 兜底：超时或中断后手动查任务
dws doc export get --job-id <JOB_ID> --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-info.md`](./doc-info.md)（前置：判断 contentType=ALIDOC 才走 export）
- [`../drive.md`](../drive.md)（非 ALIDOC 文件用 `dws drive download`）
