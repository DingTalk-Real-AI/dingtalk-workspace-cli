# dingtalk-doc 低频能力索引

本页只在根 Skill 的 Golden Route 和精确任务 Reference 都无法选路时加载。它不是创建、读取或更新任务的前置必读，也不要求预加载样式、JSONML 或完整产品帮助。

## 高频入口

| 意图 | 推荐命令 | 精确 Reference |
|---|---|---|
| 搜索在线文字文档 | `dws doc +search --query <关键词>` | [doc-info.md](doc/doc-info.md) |
| 读取正文或局部内容 | `dws doc +fetch --node <ID或URL>` | [doc-read.md](doc/doc-read.md) |
| 创建并写入 | `dws doc +create` | [doc-create.md](doc/doc-create.md) |
| 追加、覆盖、block 编辑 | `dws doc +update` | [doc-update.md](doc/doc-update.md) |
| 重要更新与恢复点 | `dws doc +checkpoint-update` | [doc-update.md](doc/doc-update.md) |
| 导出本地文件 | `dws doc +export` | [doc-export.md](doc/doc-export.md) |
| 导入为在线对象 | `dws doc +import` | [doc-import.md](doc/doc-import.md) |
| 评论聚合与操作 | `dws doc +review/+comment-*` | [doc-comment.md](doc/doc-comment.md) |
| 媒体插入、列表、下载 | `dws doc +media-*` | [doc-media.md](doc/doc-media.md) |

命令已选定但参数不确定时读取精确 leaf Schema；只有 Cobra flag 与 Schema 冲突时读取精确 leaf Help。不要加载 `dws doc --help` 或完整 Catalog 代替选路。

## 模板

只有名称时先只读搜索：

```bash
dws doc +template-search --query "周报" --source PUBLIC --format json
```

来源按用户原话守门：“我的模板/我这边”只查 `MY`，明确“公开/钉钉模板库”才查 `PUBLIC`；不得为了凑结果跨来源扩展。未指定来源时保持默认 `MY`。

- `selection.status=resolved`：取唯一候选的 `templateId`。
- `selection.status=not_found`：报告零命中后停止；不得改用语义不相干的热门模板，更不得擅自创建文档。
- `selection.status=selection_required`：展示候选并要求用户选择，禁止默认第一项。

若返回 `hasMore=true`，沿原 query/source 使用 cursor 继续搜索；只有服务端返回完整结果后才能判断零命中或完整候选集。

选定后只创建一次：

```bash
dws doc +create-from-template --template-id <TEMPLATE_ID> --name "我的周报" --format json
```

禁止通过实际创建多个候选文档来预览模板。`+create-from-template --query` 仅保留兼容，不能作为新的 Agent Golden Route。

## 历史版本

```bash
dws doc +version-save --node <DOC_ID> --format json
dws doc +version-list --node <DOC_ID> --limit 20 --format json
dws doc +version-revert --node <DOC_ID> --version <N> --format json
```

`+version-save/list/revert` 分别用于快照、浏览和恢复，命中后直接执行，不预读 Help。`+history-*` 仅兼容已有调用，不用于新的 Agent 选路。重要内容更新优先使用 `+checkpoint-update`，不要手工编排保存、写入和回读。回滚必须确认，以 leaf Schema 与 Runtime gate 为准。

## 权限与分享

- 查询或聚合权限：`+inspect --include-permissions`。
- 新增、变更、移除权限：`+access-grant/+access-change/+access-revoke`。
- 授权后发链接：`+grant-and-share`。
- 姓名、群聊或组织 profile 多候选时必须停止消歧。

## 高级原子能力

以下能力在 shortcut 未公开所需参数时才使用原子 leaf：

- 特殊 JSONML block、白板或样式字段
- 需要原始 MCP 响应的诊断
- shortcut 明确返回 capability unavailable 的低频操作

进入高级通道前只读取对应 leaf Schema 和一个精确 Reference。原子命令不是 shortcut 失败后的自动兜底，不能用来绕过权限、确认、类型或路径检查。

## 本地与跨产品边界

- 普通文件上传、下载、目录和文件树：`dingtalk-drive`
- 知识库空间与节点层级：`dingtalk-wiki`
- 原生 Markdown 文件：`dingtalk-misc`
- `axls` / `able`：对应电子表格或多维表 Skill

导出或媒体错误必须保留稳定 ID 后停止。禁止隐式执行 `curl/wget`、`pip/brew install`、Python Office 库、本地 OCR 或手写 HTTP 来伪造 DWS 任务结果。
