---
name: dingtalk-doc
description: 钉钉文档（adoc）：创建、读取、编辑、块、评论、附件、白板卡片、导出、版本及Markdown/JSONML写入。原生 .md→dingtalk-misc；文件→dingtalk-drive；知识库→dingtalk-wiki；axls→dingtalk-misc，able→dingtalk-aitable。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dws-shared`](../dws-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[doc.md](references/doc.md)；剧本：[04-document.md](references/04-document.md)。

## 参数硬约束

- 创建文档只用 `--name`，不要写 `--title`。
- 目标文件夹只用 `--folder <文档文件夹nodeId或URL>`，不要写 `--parent` / `--parent-node` / `--parent-id`。
- 目标知识库只用 `--workspace <workspaceId或URL>`，不要写 `--space-id` / `--spaceId`。
- 文档内容只用 `--content` / `--content-file`，不要写 `--markdown`。
- 复杂内容（换行、表格、代码块、长 Markdown）先写临时 `.md`，再用 `--content-file`，不要把大段 Markdown 塞进命令行。
- 每次 `create` / `update` / `block insert` / `media insert` 后必须 `dws doc read` 或 `dws doc block list` 回读关键内容。
- `doc whiteboard insert`、`doc media upload` 属于远端写入；必须先获得用户确认，再加 `--yes`。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 leaf Schema（例如 `dws schema --cli-path "doc +<shortcut>" --format json`），在当前 Cobra flags 不确定时读取 `dws doc <shortcut> --help`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service doc --format json` 批量发现。

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
| "搜在线文字文档 / 找在线文字文档" | `dws drive search --query "<关键词>" --format json` → `dws drive info --node <nodeId> --format json` → 仅 `extension=adoc` 使用 `dws doc read --node <nodeId> --format json` |
| "读在线文字文档（adoc）内容" | `dws doc read --node <nodeId> --format json` |
| "更新文档内容 / 分块追加" | `dws doc update --node <nodeId> --content "<分块>" --mode append` |
| "删除块" | `dws doc block delete`（需用户确认） |
| "导出 docx / markdown / pdf" | `dws doc export --node <nodeId> --export-format <docx|markdown|pdf> --output <path>` |
| "导入本地文件为在线文档" | `dws doc import --file <path> --folder <FOLDER_NODE_ID> --name "<标题>" --format json`（详见 `references/doc/doc-import.md`） |
| "查模板 / 套用模板创建文档" | `dws doc template list|search|apply`（详见 `references/doc.md` 模板管理） |
| "保存 / 查看 / 回滚在线文字文档（adoc）版本" | `dws doc version save/list/revert` |
| "在文档里创建空白板" | `dws doc whiteboard insert --node <nodeId> --yes --format json` |
| "为白板上传 SVG/Vector 资源" | `dws doc media upload --node <nodeId> --file <path> --yes --format json` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 nodeId/blockId。结构化命令必须带 `--format json`，执行后必须按"验证"步回读真实字段。文件类操作（上传/下载/复制/移动）切 `dingtalk-drive`；知识库节点管理切 `dingtalk-wiki`。

### SOP-1 查找并读取文档（query-doc）

**触发**：查文档/读文档/某文档在哪/搜文档内容。

1. **定位（必须）**：用户已提供 URL / `nodeId` 时直接使用原值；未提供目标时才执行 `dws drive search --query "<关键词>" --format json`，再取候选结果的真实 `nodeId`。
2. **探测（必须）**：对选中的候选执行 `dws drive info --node <nodeId> --format json`，从真实返回读取 `extension`；不得因为搜索结果标题像“文档”就跳过探测。
3. **按类型读取（必须）**：
   - `extension=adoc`：`dws doc read --node <nodeId> --format json`；大文档只抽取用户需要的章节。
   - `extension=md`：切到 `dingtalk-markdown` 用 `dws markdown fetch --node <nodeId> --format json` 读取原文；仅需文件实体下载时切 `dingtalk-drive` 用 `drive download`。
   - `extension=axls`：切到 `dingtalk-misc`，读取 `references/sheet.md` 后按电子表格意图执行。
   - `extension=able`：切到 `dingtalk-aitable`。
   - `extension=xlsx` / `xls` / `xlsm` / `csv` 或其他普通文件：切到 `dingtalk-drive`；不得执行 `dws doc read`。

**禁止**：用户未提供目标时跳过搜索并猜 nodeId、未探测类型就执行 `doc read`、把整篇文档原样贴给用户。

### SOP-2 创建文档并写入（create-doc）

**触发**：新建文档/写一篇/建文字文档。

1. **执行（必须）**：`dws doc create --name "<标题>" --content-file <tmp.md> [--folder <FOLDER_NODE_ID> | --workspace <WORKSPACE_ID>] --format json`（长/多行内容用 `--content-file`，不要用 `--content` 拼长串；用户未指定位置时省略两个位置参数，创建到“我的文档”根目录）。
2. **验证（必须）**：从返回取 `nodeId`，立即 `dws doc info --node <nodeId> --format json` 回读确认。

**禁止**：创建后不回读就答复"已创建"、把 `--folder` 当成空间 ID 传入。

### SOP-3 覆盖/追加内容（write-content）

**触发**：覆盖写/追加内容/改文档正文。

1. **执行（必须）**：覆盖先执行 `dws doc update --node <nodeId> --mode overwrite --content-file <tmp.md> --dry-run --format json` 预览，用户确认后改用 `--yes` 实际覆盖；追加执行 `dws doc update --node <nodeId> --mode append --content-file <tmp.md> --format json`。
2. **验证（必须）**：写后 `dws doc read --node <nodeId> --format json` 抽取受影响段落核对。

**禁止**：不加 `--yes` 反复重试覆盖、跳过 `--dry-run` 直接覆盖未确认的长文档。

### SOP-4 导出 / 下载（export-doc）

**触发**：导出文档/下载文档/转 PDF·Markdown。

1. **判类型（必须）**：先 `dws drive info --node <nodeId> --format json`；`extension=adoc` → `dws doc export --node <nodeId> --export-format <pdf|markdown|docx> --output <path> --format json`；普通文件 → 切 `dingtalk-drive` 用 `dws drive download --node <nodeId> --output <path> --format json`。

**禁止**：不分类型一律走 `doc export`（普通文件会失败）、跳过 `drive info` 判断。

### SOP-5 块级编辑（block-edit）

**触发**：插引用块/代码块/表格/分栏/图片/附件，或删除某块。

1. **先列块（必须）**：`dws doc block list --node <nodeId> --format json`，当前响应的可操作块 ID 位于 `blocks[].element.id`（部分版本可能回显为 `blockId`）；必须从目标内容对应项读取，不得编造。空文档的占位空段落可能不能作为 `--ref-block`。
2. **按动作执行（必须）**：
   - 插入：默认追加用 `dws doc block insert --node <nodeId> --text "<内容>" --format json`；只有明确要求相对位置时才加 `--ref-block <非空参照块ID> --where before|after`，容器内插入使用 `--parent-block <父块ID> --index <位置>`。插入命令**不接受** `--block-id`。
   - 更新：`dws doc block update --node <nodeId> --block-id <目标blockId> --text "<新内容>" --format json`。
   - 删除：用户确认后执行 `dws doc block delete --node <nodeId> --block-id <目标blockId> --yes --format json`。
3. **验证（必须）**：再次执行 `dws doc block list --node <nodeId> --format json` 核对插入、更新或删除结果。
4. **复杂块（必须）**：插入引用/代码/表格/分栏/附件/图片前，**必须**先读 [doc.md](references/doc.md) 对应小节，**禁止**只停在"准备查看 help"——说"我将插入..."后必须立即执行命令。

**禁止**：编造 blockId、未确认就删除、把完整 `--help` 输出当成最终结果答复用户。
### SOP-6 导入本地文件为在线文档（import-file）

**触发**：导入 Word / Excel / Markdown / 本地文件为在线文档。

1. **判类型（必须）**：确认用户意图是“导入为在线文档”，不是“上传到钉盘”。仅上传存储时切 `dingtalk-drive`。
2. **执行（必须）**：`dws doc import --file <path> --folder <FOLDER_NODE_ID> --name "<标题>" --format json`；复杂参数和限制见 [doc-import.md](references/doc/doc-import.md)。
3. **验证（必须）**：拿到返回 `nodeId` 后执行 `dws doc info --node <nodeId> --format json`，必要时 `dws doc read --node <nodeId> --format json` 抽样核对内容。

**禁止**：把上传文件到钉盘误当成 doc import；不知道目标文件夹 nodeId 时先切 `dingtalk-drive`/`dingtalk-wiki` 查询。

## 多步文档短路径

- 在目标文件夹创建文字文档：`dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --content-file <tmp.md> --format json`。拿到 `nodeId` 后立即回读。
- 块级编辑固定顺序：`doc block list --node <nodeId>` → 插入用 `--ref-block`/`--parent-block`，更新或删除用 `--block-id` → `doc block list` 验证。删除块必须已有用户明确删除意图或二次确认。
- 插入引用块、代码块、表格、分栏、附件、图片时，优先读 [doc.md](references/doc.md) 对应小节，不要只停在"准备查看 help"。说出"我将插入..."后必须立即执行对应 terminal 调用。
- 用户要求多个子文档/附件/块操作时，按 checklist 串行完成；最后一条 assistant 消息不能停在"接下来我要..."，必须有实际工具调用或明确失败原因。
- 用户说“读取并下载/导出”时，先 `drive info --node ... --format json` 按
  `extension` 判断类型：`adoc` 用 `doc export`，普通文件切到
  `dingtalk-drive` 用 `drive download`。
- 所有结构化 dws 命令带 `--format json`。仅参数不确定时查 `--help`，不要把完整 help 当成最终结果。

## 危险操作

`block delete` 不可逆，必须确认再加 `--yes`。

## 跨产品协作

- 文件存储 / 上传下载 → 切到 `dingtalk-drive`
- 知识库空间管理 → 切到 `dingtalk-wiki`
- 数据表 → 切到 `dingtalk-aitable`
- 原生 `.md` 文件读取、创建、全量覆盖或局部替换 → 切到 `dingtalk-markdown`
- 长篇报告生成（多源采集 + 写文档）→ 此 skill 提供 `doc_create_and_write.py` 脚本
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
