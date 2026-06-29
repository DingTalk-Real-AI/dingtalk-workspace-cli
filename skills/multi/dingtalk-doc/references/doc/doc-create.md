# doc create（创建文档）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
> 2. [`./style/doc-create-workflow.md`](./style/doc-create-workflow.md) — 创建工作流（标题、位置、骨架、回读校验）
> 3. [`./style/doc-style-guideline.md`](./style/doc-style-guideline.md) — 排版规范（草稿元素清单、骨架样板）
> 4. [`./doc-update.md` §内容写入管道](./doc-update.md#内容写入管道createupdate-共用) — 长内容自动分片、`--content-file` vs `--content` 选择
> 5. [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) — 仅当使用 `--content-format jsonml` 时必读

## 创建路由前置判断（必看）

> `dws doc create` 只能创建在线文字文档（adoc），**不要**用它承接所有「新建 xxx」请求。收到「创建/新建」类需求时，必须先按文件类型分流：
>
> - 用户说「创建表格 / 新建表格 / 建个电子表格 / 在线表格 / 销售数据表」等 → 走 [`dws sheet create`](../sheet.md#创建钉钉表格文档)（钉钉在线电子表格 `axls`），**不要**走 `doc create`
> - 用户说「创建多维表格 / 新建 AI 表格 / 建个 base / 数据库表」等 → 走 [`dws aitable base create`](../aitable.md#创建-ai-表格)（多维表格 `able`），**不要**走 `doc create`
> - 用户说「创建文档 / 新建文档 / 写篇文档 / 会议纪要 / 周报 / 方案」等文字型内容 → 才走 `dws doc create`
>
> 一句话口诀：表格 → sheet/aitable；文档 → doc。

## 命令格式

```
Usage:
  dws doc create [flags]
Example:
  dws doc create --name "项目周报"
  dws doc create --name "Q1 总结" --content "# Q1 总结" --folder <DOC_FOLDER_NODE_ID>
  dws doc create --name "知识库文档" --workspace <WS_ID>
  dws doc create --name "周报" --content-file ./weekly.md --folder <DOC_FOLDER_NODE_ID>
  cat report.md | dws doc create --name "月报" --content -
Flags:
      --name string           文档名称 (必填)
      --folder string         目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string      目标知识库 ID
      --content string        文档初始内容（短文本字面量）；传 - 表示从 stdin 读取
      --content-file string   从文件读取文档内容（UTF-8）。推荐长/多行/表格内容使用
      --content-format string         内容格式: 默认为 markdown，可选 jsonml
      --fix-jsonml              启用 JSON 语法修复（括号/逗号补全），推荐 agent 调用时使用
```

## 关键说明

- **`--name` 是 H1**：正文从 `##` 开始；正文内不要再写 `#` 一级标题（除非确需且已说明动机）。
- 不传 `--folder` 和 `--workspace` 时，默认创建在「我的文档」根目录。
- `--folder` 仅接受文档文件夹 `nodeId` / `dentryUuid` / alidocs 文件夹 URL；**禁止**传入 drive `dentryId`、`parentId`、`spaceId` 这类纯数字 ID。
- 输入方式选择见 [`./doc-update.md` §内容写入管道](./doc-update.md#内容写入管道createupdate-共用)（与 update 共用）。短文本字面量可 `--content`，多行/表格/特殊字符必须 `--content-file` 或 `--content -`。
- 长内容（>30000 字符）CLI 自动分片：先创建空文档拿 `nodeId`，再按 markdown 标题边界切分后逐片 append；调用方无需手动编排。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `nodeId` | [`./doc-update.md`](./doc-update.md) / [`./doc-block.md`](./doc-block.md) / [`./doc-media.md`](./doc-media.md) 的 `--node` |
| `docUrl` | 最终交付给用户的链接；缺失时用 [`./doc-info.md`](./doc-info.md) 补查 |
| `chunksWritten` | 判断是否触发自动分片；> 1 时重点检查章节顺序 |

## 回读验收（必读）

CLI **不会**自动回读校验。**每次创建后**都必须执行 `doc read --node <nodeId>` 校验关键标题、段落首句、表格表头是否完整。详见 [`./style/doc-create-workflow.md` «回读验收»](./style/doc-create-workflow.md)。

## 常用模板

```bash
# 默认创建到「我的文档」根目录（推荐文件路径）
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --content-format markdown

# 创建到指定文件夹
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --folder <DOC_FOLDER_NODE_ID> --content-format markdown

# 创建到知识库
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --workspace <WS_ID> --content-format markdown

# 创建空文档（仅取 nodeId 后再分步写入，适合 >200KB 兜底）
dws doc create --name "<文档名>" [--folder <ID> | --workspace <ID>] --content-format markdown

# 短纯文本字面量（< 2KB 且无换行/表格才允许）
dws doc create --name "<文档名>" --content "短内容" --content-format markdown

# stdin（heredoc / pipe）
cat report.md | dws doc create --name "月报" --content - --content-format markdown

# JSONML 起稿（决策型 / 对展示效果有要求时直接用 JSONML 构造）
# 详见 doc-create-workflow.md §JSONML 起稿判定
dws doc create --name "<文档名>" --content-file /tmp/<name>.json --content-format jsonml

# JSONML 创建到指定文件夹
dws doc create --name "<文档名>" --content-file /tmp/<name>.json --content-format jsonml --folder <DOC_FOLDER_NODE_ID>
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-update.md`](./doc-update.md)（写入管道、长 markdown、追加段落、回读补救）
- [`./style/doc-create-workflow.md`](./style/doc-create-workflow.md)（创建流程 + 回读验收）
- [`./style/doc-style-guideline.md`](./style/doc-style-guideline.md)（草稿排版规范）
- [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) / [`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md)（JSONML 节点结构与范例）
