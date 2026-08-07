# doc create（创建文档）

> 本文件自包含普通 Markdown 创建契约，不要递归预读 `doc.md`、style 或 update reference。仅当用户要求复杂版式并实际选择 JSONML 时，读取 [`doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)；需要文档骨架建议时才读取对应 style 章节。

## 创建路由前置判断（必看）

> `dws doc create` 只能创建在线文字文档（adoc），**不要**用它承接所有「新建 xxx」请求。收到「创建/新建」类需求时，必须先按文件类型分流：
>
> - 用户说「创建表格 / 新建表格 / 建个电子表格 / 在线表格 / 销售数据表」等 → 走 [`dws sheet create`](../../../dingtalk-misc/references/sheet/sheet-workbook.md#创建钉钉表格文档)（钉钉在线电子表格 `axls`），**不要**走 `doc create`
> - 用户说「创建多维表格 / 新建 AI 表格 / 建个 base / 数据库表」等 → 走 [`dws aitable base create`](../../../dingtalk-aitable/references/aitable.md#base-base-管理)（多维表格 `able`），**不要**走 `doc create`
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

- **标题优先级**：`--name` 是文档外壳标题，默认可视作 H1；但它不能替代用户显式要求的正文一级标题。用户说“正文写 `# ...`”“正文先起个一级标题”时，必须在初始内容中保留该 `#` H1；用户未要求正文 H1 时，正文默认从 `##` 开始以避免重复。
- 不传 `--folder` 和 `--workspace` 时，默认创建在「我的文档」根目录。
- `--folder` 仅接受文档文件夹 `nodeId` / `dentryUuid` / alidocs 文件夹 URL；**禁止**传入 drive `dentryId`、`parentId`、`spaceId` 这类纯数字 ID。
- 输入方式选择见 [`./doc-update.md` §内容写入管道](./doc-update.md)（与 update 共用）。短文本字面量可 `--content`，多行/表格/特殊字符必须 `--content-file` 或 `--content -`。
- Markdown 超过 10,000 个 Unicode 字符时，CLI 自动按结构分片：第一片随 `create` 写入并取得 `nodeId`，后续片自动 append；调用方不要手动预分片。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `nodeId` | [`./doc-update.md`](./doc-update.md) / [`./doc-block.md`](./doc-block.md) / [`./doc-media.md`](./doc-media.md) 的 `--node` |
| `docUrl` | 最终交付给用户的链接；缺失时用 [`./doc-info.md`](./doc-info.md) 补查 |
| `chunksWritten` | 判断是否触发自动分片；> 1 时重点检查章节顺序 |

同一请求后续出现“这篇/刚才那篇/上次那篇”时，直接续用本次 create 返回的 `nodeId`；禁止先搜索同名文档再把后续操作指向旧节点。

## 显式操作序列

用户点名 `block list`、插入、追加、更新等后续动作时，必须按原顺序逐项执行。`doc create` 只写用户指定的初始内容，不能为了减少调用把后续标题、列表或段落提前塞进 create。例：`创建 → 查看块结构 → 末尾插入段落` 必须真实执行 create、block list、block insert 三步。

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

# 创建空文档（用户明确需要空文档时）
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
