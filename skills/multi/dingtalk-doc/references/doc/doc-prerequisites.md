# 文档参考选择

只读取当前操作需要的参考，不要从命令 leaf 反向加载聚合 router：

| 场景 | 参考 |
|---|---|
| 解析 URL、确认 nodeId 或文档类型 | [doc-info.md](./doc-info.md) |
| 创建普通 Markdown 文档 | [doc-create.md](./doc-create.md) |
| 长内容创建、分片或正文完整性验收 | [doc-create-workflow.md](./style/doc-create-workflow.md) |
| 追加、覆盖或并发安全更新 | [doc-update.md](./doc-update.md) |
| 块级精修 | [doc-block.md](./doc-block.md) |
| 正式排版或统一风格 | [doc-style-guideline.md](./style/doc-style-guideline.md) |
| JSONML、callout、分栏或复杂表格 | [doc-jsonml-cookbook.md](./format/doc-jsonml-cookbook.md) |

阅读、评论、权限、附件和导出等场景直接选择对应命令参考，无需先加载创建/更新工作流。
