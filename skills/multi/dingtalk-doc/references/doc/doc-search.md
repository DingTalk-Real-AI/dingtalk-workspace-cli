# doc search（已弃用 → drive search / wiki node search）

> **弃用提示**：`dws doc search` 已迁移，不再是 `doc` 子命令。它虽仍能跑，但每次都会打印弃用警告：
> `'dws doc search' is deprecated, use 'dws drive search' or 'dws wiki node search --workspace <id>'`。
> 搜索文档/文件请切到 `dingtalk-drive` skill，用下面的命令。

## 改用什么

| 旧命令（弃用） | 改用 | 场景 |
|--------------|------|------|
| `dws doc search --query "<关键词>"` | `dws drive search --query "<关键词>"` | 全局搜文档/文件（跨钉盘+文档空间聚合） |
| `dws doc search --extensions pdf,docx` | `dws drive search --query "..." --extensions pdf,docx` | 按扩展名过滤 |
| `dws doc search --workspace-ids <id>` | `dws wiki node search --workspace <id> --query "<关键词>"` | 指定知识库内搜索 |

`drive search` 的完整参数（`--query` / `--extensions` / `--created-from` / `--creator-uids` / `--limit` / `--cursor` 等）见 `dingtalk-drive` skill。

## 上下文传递

拿到返回里的文档 `nodeId` / URL 后，`doc read` / `doc info` / `doc update` / `doc block` 等内容级命令照常用。

## 参考

- `dingtalk-drive` skill（搜索 / 列表 / 上传下载等文件管理命令的归属）
- [`./doc-info.md`](./doc-info.md)（URL → nodeId 提取）
