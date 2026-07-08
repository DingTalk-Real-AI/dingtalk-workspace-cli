# doc list（已弃用 → drive list / wiki node list）

> **弃用提示**：`dws doc list` 已迁移，不再是 `doc` 子命令。它虽仍能跑，但每次都会打印弃用警告：
> `'dws doc list' is deprecated, use 'dws drive list --workspace <workspaceId>' or 'dws wiki node list --workspace <workspaceId>'`。
> 遍历文件夹/知识库请改用下面的命令，命令详情见 [`../drive.md`](../drive.md)。

## 改用什么

| 旧命令（弃用） | 改用 | 场景 |
|--------------|------|------|
| `dws doc list` | `dws drive list` | 遍历「我的文档」/钉盘根目录 |
| `dws doc list --folder <id>` | `dws drive list --folder <id>` | 遍历指定文件夹 |
| `dws doc list --workspace <id>` | `dws drive list --workspace <id>` 或 `dws wiki node list --workspace <id>` | 遍历知识库 |

`drive list` 的完整参数（`--folder` / `--workspace` / `--limit` / `--cursor` / `--order-by` 等）见 [`../drive.md` §获取文件/文件夹列表](../drive.md)。

## 上下文传递

从返回里取 `nodes[].nodeId`（folder 类型的 `nodeId` 可继续作 `--folder`），传给 `doc read` / `doc info` / `doc update` 等内容级命令的 `--node`。

## 参考

- [`../drive.md`](../drive.md)（列表 / 搜索 / 上传下载等文件管理命令的归属）
- [`./doc-info.md`](./doc-info.md)（拿到 nodeId 后查元信息）
