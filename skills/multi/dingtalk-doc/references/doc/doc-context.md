# 上下文传递表

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `drive search` / `wiki node search` | 文档 `nodeId` / URL | doc read / info / update 等所有 `--node` 入参 |

| `drive list` / `wiki node list` | `nodes[].nodeId` | doc read / info / update / block 操作的 --node |
| `create` | `nodeId` | update / block 操作的 --node |
| `block list` | `blockId` | block insert 的 --ref-block, block update/delete 的 --block-id |
| `read --content-format jsonml` | `revision` | update --content-format jsonml 的 --revision（可选，并发检查时使用） |
| `media insert` | `resourceId` | 附件已插入文档，可通过 block list 查看附件块 |
| `media download` | 附件下载链接 `downloadUrl` | 下载文档中的附件资源 |
| `block list` | attachment 块的 `resourceId` | media download 的 --resource-id |
| `comment list` | `commentList[].commentKey` | comment reply 的 --comment-key |
| `comment create` | `commentKey` | comment reply 的 --comment-key |
| `comment create-inline` | `commentKey` | comment reply 的 --comment-key |
| `block list` | `blocks[].element.id` | comment create-inline 的 --block-id |
| `block list` | `blocks[].element.paragraph.text` | 计算 create-inline 的 --start / --end 偏移量 |
| `contact user search` | `userId` | comment create/reply/create-inline 的 --mention；drive permission 的 --user |
| `version list` | `version`（版本号） | version revert 的 --version |
| `version save` | 版本信息 | 确认保存成功 |
| `import` | `documentUrl` / `documentName` / `documentType` | 导入完成后的在线文档地址和名称 |
| `import` (中断) | `taskId` | `import get` 的 `--task-id` |
