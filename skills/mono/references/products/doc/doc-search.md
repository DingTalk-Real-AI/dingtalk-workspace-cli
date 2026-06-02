# doc search（搜索文档）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
>
> **同任务常配合**：[`doc-list.md`](./doc-list.md)（目录遍历，互补于关键字搜索）/ [`doc-info.md`](./doc-info.md)（拿到 nodeId 后查元信息）/ [`doc-read.md`](./doc-read.md)（拿到 nodeId 后读取正文）

## 命令格式

```
Usage:
  dws doc search [flags]
Example:
  dws doc search --query "会议纪要"
  dws doc search
  dws doc search --extensions pdf,docx
  dws doc search --query "方案" --created-from 1700000000000 --created-to 1710000000000
  dws doc search --creator-uids uid1,uid2
  dws doc search --workspace-ids wsId1,wsId2
Flags:
      --query string              搜索关键词 (不传则返回最近访问)
      --extensions strings         按文件扩展名过滤，不含点号，逗号分隔 (如 pdf,docx,png)。
                                    钉钉在线文档: adoc(文字) axls(表格) appt(演示文稿) awbd(白板) adraw(画板) amind(脑图) able(多维表格) aform(收集表)
                                    常见附件: pdf docx doc xlsx xls pptx ppt csv txt md json xml zip rar png jpg jpeg gif mp4 mp3
                                    以上仅为参考，extensions 为开放参数，服务端支持的扩展名不限于此。不确定文件后缀时，建议不传 --extensions 让搜索返回所有类型，再从结果中按文件名后缀筛选
      --created-from int          创建时间起始 (毫秒时间戳，含)
      --created-to int            创建时间截止 (毫秒时间戳，含)
      --visited-from int          访问时间起始 (毫秒时间戳，含)
      --visited-to int            访问时间截止 (毫秒时间戳，含)
      --creator-uids strings      按创建者用户 ID 过滤，逗号分隔
      --editor-uids strings       按编辑者用户 ID 过滤，逗号分隔
      --mentioned-uids strings    按 @提及的用户 ID 过滤，逗号分隔
      --workspace-ids strings     按知识库 ID 过滤，支持知识库 URL，逗号分隔
      --limit int             每页数量 (默认 10，最大 30)
      --cursor string         分页游标 (从上次结果的 nextPageToken 获取)
```

## 关键说明

- 不传 `--query` 时返回最近访问列表，适合"最近文档"类意图。
- `--extensions` 是开放参数，传入服务端不识别的扩展名时不会报错（可能也搜不到）；优先通过文件名后缀人工筛选。
- 多个时间戳为毫秒时间戳，注意单位（不是秒）。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| 文档 `nodeId` / URL | [`doc-read.md`](./doc-read.md) / [`doc-info.md`](./doc-info.md) / [`doc-update.md`](./doc-update.md) / [`doc-file-ops.md`](./doc-file-ops.md) 的 `--node` |
| `createTime` / `creatorUid` | 创建时间与创建者过滤的二次检索 |

## 常用模板

```bash
# 关键字搜索（最常用）
dws doc search --query "项目周报" --format json

# 仅最近访问（不传 --query）
dws doc search --format json

# 按扩展名过滤（在线文档族 + 常见办公附件）
dws doc search --extensions adoc,axls,able,docx,xlsx,pdf

# 按创建时间窗口（毫秒时间戳）
dws doc search --query "方案" --created-from 1700000000000 --created-to 1710000000000

# 按创建者过滤（多个 uid 逗号分隔）
dws doc search --creator-uids uid1,uid2

# 按知识库范围过滤（支持知识库 URL）
dws doc search --workspace-ids wsId1,wsId2

# 翻页
dws doc search --query "周报" --limit 30 --cursor <nextPageToken>
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-list.md`](./doc-list.md)（目录遍历替代路径）
- [`./doc-info.md`](./doc-info.md)（URL → nodeId 提取）
