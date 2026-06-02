# doc list（遍历文件列表）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
>
> **同任务常配合**：[`doc-search.md`](./doc-search.md)（关键字检索更精准）/ [`doc-info.md`](./doc-info.md)（拿到 nodeId 后查元信息）

## 命令格式

```
Usage:
  dws doc list [flags]
Example:
  dws doc list
  dws doc list --folder <DOC_FOLDER_NODE_ID>
  dws doc list --workspace <WS_ID> --limit 20
Flags:
      --folder string       文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string    知识库 ID
      --limit int       每页数量 (默认 50，最大 50)
      --cursor string   分页游标 (从上次结果的 nextPageToken 获取)
```

## 关键说明

- 不传任何 flag 时遍历"我的文档"根目录。
- `--folder` 仅接受文档文件夹 `nodeId` / `dentryUuid` / alidocs 文件夹 URL；**禁止**传入 drive `dentryId`、`parentId`、`spaceId` 这类纯数字 ID。
- 单页最多 50 条，需翻页时使用 `nextPageToken` → `--cursor`。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `nodes[].nodeId` | [`doc-read.md`](./doc-read.md) / [`doc-info.md`](./doc-info.md) / [`doc-update.md`](./doc-update.md) / [`doc-file-ops.md`](./doc-file-ops.md) 等所有 `--node` 入参 |
| folder 类型的 `nodeId` | 当前 `list --folder` 递归遍历；[`doc-create.md`](./doc-create.md) / [`doc-file-ops.md`](./doc-file-ops.md) 的 `--folder` |

## 常用模板

```bash
# 浏览"我的文档"根目录
dws doc list --format json

# 浏览指定文档文件夹（folder nodeId 或 alidocs 文件夹 URL）
dws doc list --folder <DOC_FOLDER_NODE_ID> --format json
dws doc list --folder "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# 浏览指定知识库根目录（取出 workspaceId 后）
dws doc list --workspace <WS_ID> --limit 20 --format json

# 翻页
dws doc list --folder <DOC_FOLDER_NODE_ID> --cursor <nextPageToken> --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-search.md`](./doc-search.md)（关键字检索路径）
- [`./doc-info.md`](./doc-info.md)（拿到 nodeId 后查元信息）
