# 场景索引

## 场景索引

> 任务驱动的入口：知道任务但不确定要读哪些命令文件时，按本表**一次性 Read 文件组**，不要逐个跳。

| 任务场景 | 一次性读取 | 主命令 |
|---------|-----------|--------|
| 定位 nodeId / URL 解析 | [`doc-info.md`](./doc-info.md)（搜索请用 `dws drive search` / `dws wiki node search`；遍历请用 `dws drive list` / `dws wiki node list`） | `drive search` / `wiki node search` |
| 阅读已有文档 | [`doc-info.md`](./doc-info.md) + [`doc-read.md`](./doc-read.md) | read |
| 创建新文档 | [`doc-create.md`](./doc-create.md) + [`doc-update.md`](./doc-update.md)（写入管道）+ [`style/doc-create-workflow.md`](./style/doc-create-workflow.md) + [`style/doc-style-guideline.md`](./style/doc-style-guideline.md) | create |
| 创建文档且包含图片/截图/图文并茂 | [`doc-create.md`](./doc-create.md) + [`doc-media.md`](./doc-media.md) + [`style/doc-create-workflow.md`](./style/doc-create-workflow.md) + [`style/doc-style-guideline.md`](./style/doc-style-guideline.md) | create → media insert |
| 局部改写 / 段落替换（保真） | [`doc-read.md`](./doc-read.md) + [`doc-update.md`](./doc-update.md) + [`doc-block.md`](./doc-block.md) + [`format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) + [`style/doc-update-workflow.md`](./style/doc-update-workflow.md) | block update |
| 整篇 overwrite（可选并发检查） | [`doc-read.md`](./doc-read.md) + [`doc-update.md`](./doc-update.md) + [`format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md) + [`style/doc-update-workflow.md`](./style/doc-update-workflow.md) | update |
| 插入富 block（callout / 分栏 / 表格） | [`doc-block.md`](./doc-block.md) + [`format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) + [`style/doc-style-guideline.md`](./style/doc-style-guideline.md) | block insert |
| 上传图片 / 附件 | [`doc-media.md`](./doc-media.md) | media insert |
| 评论 / 划词评论（含 @人） | [`doc-comment.md`](./doc-comment.md)（+ `dws contact user search` 取 mention 用 userId） | comment create |
| 文档分享 / 节点级权限 | [`drive.md`](../../../dingtalk-drive/references/drive.md)（已迁移：`dws drive permission add/update/list/remove`） | `drive permission` |
| 导出 PDF / DOCX / Markdown | [`doc-info.md`](./doc-info.md) + [`doc-export.md`](./doc-export.md) | export |
| 导入本地文件为在线文档 | [`doc-info.md`](./doc-info.md) + [`doc-import.md`](./doc-import.md) | import |
| 文件下载 / 上传 / 移动 / 重命名 / 复制 | [`drive.md`](../../../dingtalk-drive/references/drive.md)（已迁移：`dws drive upload/download/copy/move/rename/delete`） | `drive *` |
| 版本管理（保存/列出/回滚） | 本文件 §版本管理 | `version save/list/revert` |
