---
name: dingtalk-doc
description: 钉钉文档内容操作（内容层）。Use when 用户说 写文档/读文档/创建文档内容/编辑文档内容/文档块/分块编辑/Markdown 写入/评论/媒体附件/导出文档/导出 PDF/导出 Markdown/版本历史/保存版本/回滚版本。搜文件/列文件/上传下载/复制移动/重命名/权限管理走 dingtalk-drive；知识库/空间/节点创建列表搜索走 dingtalk-wiki；文档里嵌入的电子表格/多维表格先用本 skill 提取 token 再切到 dingtalk-sheet/dingtalk-aitable。命令前缀：dws doc。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[doc.md](references/doc.md)；剧本：[04-document.md](references/04-document.md)。
> 高频模块：[阅读/元信息](references/doc/doc-info-index.md) · [创建/写入/块编辑](references/doc/doc-write-index.md) · [媒体/评论/导出](references/doc/doc-media-index.md) · [导入/版本/模板](references/doc/doc-extra-index.md) · [JSONML](references/format/doc-jsonml-index.md)。

## 参数硬约束

- 创建文档只用 `--name`，不要写 `--title`。
- 目标文件夹只用 `--folder <文档文件夹nodeId或URL>`，不要写 `--parent` / `--parent-node` / `--parent-id`。
- 目标知识库只用 `--workspace <workspaceId或URL>`，不要写 `--space-id` / `--spaceId`。
- 文档内容只用 `--content` / `--content-file`，不要写 `--markdown`。
- 复杂内容（换行、表格、代码块、长 Markdown）先写临时 `.md`，再用 `--content-file`，不要把大段 Markdown 塞进命令行。
- 每次 `create` / `update` / `block insert` / `media insert` 后必须 `dws doc read` 或 `dws doc block list` 回读关键内容。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service doc --format json` 读取参数、约束、风险和示例，并以 `dws doc <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws doc +comment-create` | write | 在文档上创建一条评论 |
| `dws doc +comment-list` | read | 查询文档评论列表 |
| `dws doc +comment-reply` | write | 回复文档中的一条评论 |
| `dws doc +copy` | write | 复制文档/文件到指定文件夹或知识库 |
| `dws doc +doc-append` | write | 在文档末尾追加一段文本（安全追加，不改动原有内容） |
| `dws doc +export-get` | read | 根据 jobId 查询文档导出任务结果 |
| `dws doc +export-submit` | read | 提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId |
| `dws doc +find-doc` | read | 按关键词搜索云文档并投影关键字段（只读） |
| `dws doc +list` | read | 列出文件夹或知识库下的直接子节点 |
| `dws doc +move` | write | 移动文档/文件到指定文件夹或知识库 |
| `dws doc +search` | read | 按关键词搜索有权限的文档 (不传则返回最近访问) |
| `dws doc +share-doc` | write | 按姓名把文档链接私信发给某人（自动解析 userId） |
| `dws doc +template-list` | read | 获取文档模板列表 |
| `dws doc +template-search` | read | 根据关键词搜索文档模板 |
| `dws doc +version-list` | read | 查看文档历史版本列表 |
| `dws doc +version-revert` | high-risk-write | 回滚文档到指定历史版本 |
| `dws doc +version-save` | write | 手动保存文档版本快照 |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建文档（短内容）" | `dws doc create --name "<标题>" --content "<内容>"` |
| "创建+写入（长内容自动分块）" | `python scripts/doc_create_and_write.py --name "<标题>" --content "<内容>" [--mode append\|overwrite]` |
| "搜文档 / 找文档" | `dws drive search --query "<关键词>"` → 取 `nodeId` 后 `dws doc read --node <nodeId>` |
| "读文档内容" | `dws doc read --node <nodeId>` |
| "更新文档内容 / 分块追加" | `dws doc update --node <nodeId> --content "<分块>" --mode append` |
| "删除块" | `dws doc block delete`（需用户确认） |
| "更新文档评论" | `dws doc comment update --node <nodeId> --comment-key <key> --content "<内容>"` |
| "删除文档评论" | `dws doc comment delete --node <nodeId> --comment-key <key> --yes`（需用户确认） |
| "导出 docx / markdown / pdf" | `dws doc export --node <nodeId> --export-format <docx|markdown|pdf> --output <path>` |
| "导入本地文件为在线文档" | `dws doc import --file <path> --folder <FOLDER_NODE_ID> --name "<标题>" --format json`（详见 `references/doc/doc-import.md`） |
| "查模板 / 套用模板创建文档" | `dws doc template list|search|apply`（详见 `references/doc/doc-template.md`） |
| "保存版本 / 查看版本历史 / 回滚版本" | `dws doc version save/list/revert` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 nodeId/blockId。每条命令必须带 `--format json`，执行后必须按"验证"步回读真实字段。文件类操作（上传/下载/复制/移动）切 `dingtalk-drive`；知识库节点管理切 `dingtalk-wiki`。

### SOP-1 查找并读取文档（query-doc）

**触发**：查文档/读文档/某文档在哪/搜文档内容。

1. **定位（必须）**：`dws drive search --query "<关键词>" --format json` → 取 `nodeId`（拿不准是文档还是文件时，先 `dws doc info --node <nodeId> --format json` 判类型）。
2. **读取（必须）**：`dws doc read --node <nodeId> --format json`；大文档**必须**只抽取用户需要的章节，**禁止**无差别全文展开。

**禁止**：跳过 `drive search` 直接猜 nodeId、把整篇文档原样贴给用户。

### SOP-2 创建文档并写入（create-doc）

**触发**：新建文档/写一篇/建文字文档。

1. **执行（必须）**：`dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --content-file <tmp.md> --format json`（长/多行内容用 `--content-file`，不要用 `--content` 拼长串）。
2. **验证（必须）**：从返回取 `nodeId`，立即 `dws doc read --node <nodeId> --format json` 回读正文；仅需确认类型、名称或补取链接时再用 `doc info`。

**完成条件**：创建结果返回真实 `nodeId`，且正文回读成功；`--folder` 只能使用文档文件夹 nodeId，不能传空间 ID。

### SOP-3 覆盖/追加内容（write-content）

**触发**：覆盖写/追加内容/改文档正文。

1. **执行（必须）**：覆盖 `dws doc update --node <nodeId> --mode overwrite --content-file <tmp.md> --yes --format json`；追加 `--mode append`。破坏性覆盖**必须**带 `--yes`，且建议先 `--dry-run` 预览。
2. **验证（必须）**：写后 `dws doc read --node <nodeId> --format json` 抽取受影响段落核对。

**禁止**：不加 `--yes` 反复重试覆盖、跳过 `--dry-run` 直接覆盖未确认的长文档。

### SOP-4 导出 / 下载（export-doc）

**触发**：导出文档/下载文档/转 PDF·Markdown。

1. **判类型（必须）**：先 `dws doc info --node <nodeId> --format json`；在线文档 → `dws doc export --node <nodeId> --export-format <docx|markdown|pdf> --output <path> --format json`；普通文件 → 切 `dingtalk-drive` 用 `dws drive download --node <nodeId> --output <path> --format json`。

**禁止**：不分类型一律走 `doc export`（普通文件会失败）、跳过 `doc info` 判断。

### SOP-5 块级编辑（block-edit）

**触发**：插引用块/代码块/表格/分栏/图片/附件，或删除某块。

1. **固定顺序（必须）**：`dws doc block list --node <nodeId> --format json` → 选真实 `blockId` → `dws doc block insert|update|delete --node <nodeId> --block-id <blockId> ...` → 再次 `doc block list` 验证。
2. **删除（必须）**：删除块**必须**已有用户明确删除意图或二次确认。
3. **复杂块（必须）**：插入引用/代码/表格/分栏/附件/图片前，先按 [写入索引](references/doc/doc-write-index.md) 与 [JSONML 索引](references/format/doc-jsonml-index.md) 确认对应命令和元素结构；写后按第 1 步回读验证。

**禁止**：编造 blockId、未确认就删除。命令帮助只用于确认参数，操作结果必须来自实际命令与回读。
### SOP-6 导入本地文件为在线文档（import-file）

**触发**：导入 Word / Excel / Markdown / 本地文件为在线文档。

1. **判类型（必须）**：确认用户意图是“导入为在线文档”，不是“上传到钉盘”。仅上传存储时切 `dingtalk-drive`。
2. **执行（必须）**：`dws doc import --file <path> --folder <FOLDER_NODE_ID> --name "<标题>" --format json`；复杂参数和限制见 [doc-import.md](references/doc/doc-import.md)。
3. **验证（必须）**：拿到返回 `nodeId` 后执行 `dws doc info --node <nodeId> --format json`，必要时 `dws doc read --node <nodeId> --format json` 抽样核对内容。

**禁止**：把上传文件到钉盘误当成 doc import；不知道目标文件夹 nodeId 时先切 `dingtalk-drive`/`dingtalk-wiki` 查询。


## 多步文档短路径

- 在目标文件夹创建文字文档：`dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --content-file <tmp.md> --format json`。拿到 `nodeId` 后立即回读。
- 块级编辑固定顺序：`doc block list --node <nodeId>` → 选 `blockId` → `doc block insert/update/delete` → `doc block list` 验证。删除块必须已有用户明确删除意图或二次确认。
- 插入引用块、代码块、表格、分栏、附件、图片时，按 [写入索引](references/doc/doc-write-index.md) 选择命令与结构，并在写后回读。
- 多个子文档、附件或块操作按依赖顺序执行；每一步复用上一条命令返回的真实 ID，最后统一回读受影响内容。
- 用户说"读取并下载/导出"时，先 `doc info --node ... --format json` 判断类型：在线文档用 `doc export`，普通文件切到 `dingtalk-drive` 用 `drive download`。
- 所有 dws 命令带 `--format json`；仅参数不确定时查 `--help`，并以实际命令结果完成验证。

## 危险操作

`block delete` 和 `comment delete` 不可逆，必须确认再加 `--yes`。

## 跨产品协作

- 文件存储 / 上传下载 → 切到 `dingtalk-drive`
- 知识库空间管理 → 切到 `dingtalk-wiki`
- 数据表 → 切到 `dingtalk-aitable`
- 长篇报告生成（多源采集 + 写文档）→ 此 skill 提供 `doc_create_and_write.py` 脚本
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
