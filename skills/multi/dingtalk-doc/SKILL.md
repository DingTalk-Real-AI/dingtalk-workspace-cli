---
name: dingtalk-doc
description: 钉钉文档（云文档）。Use when 用户说 写文档/读文档/创建文档/编辑文档/搜文档/文档块/分块编辑/Markdown 写入/上传文件到文档。Distinct from dingtalk-drive(钉盘文件存储)、dingtalk-aitable(数据表格)、dingtalk-wiki(知识库空间)。命令前缀：dws doc。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 渐进式命令参考入口：[doc.md](references/doc.md)；剧本：[04-document.md](references/04-document.md)；URL 识别与类型探测：[url-patterns.md](references/url-patterns.md)。详细参数、示例和踩坑说明按任务读取 `references/doc/` 下对应子文档。

## URL 预检（含 alidocs URL 时必读）

输入含 `alidocs.dingtalk.com` URL 时，该域名下存在多种路径格式：`/i/p/...`（分享短链）、`/i/nodes/...`（节点链接，类型需探测）、`/spreadsheetv2/...`（电子表格直链）、`/document/edit|preview?dentryKey=...`（文档链接）等，每种处理流程不同。**必须先读 [url-patterns.md](references/url-patterns.md) 中的「alidocs URL 分流决策」**，按规则识别 URL 类型后再选择对应命令；其中 `/document/edit|preview?dentryKey=...` 直接路由到 `doc`，将完整 URL 原样传给 `--node`，**不要**提取 `dentryKey` 当裸 nodeId。

## 渐进式文档读取规则

执行任何 `dws doc` 操作前，先读 [references/doc.md](references/doc.md) 作为路由层，再按其中「命令索引表 / 场景索引」读取对应子文档。不要只凭本文件或记忆补参数。

- URL 解析、定位 nodeId、判断 contentType：读 [doc-info.md](references/doc/doc-info.md)，必要时加读 [doc-search.md](references/doc/doc-search.md) / [doc-list.md](references/doc/doc-list.md)。
- 新建文档：读 [doc-create.md](references/doc/doc-create.md)、[doc-update.md](references/doc/doc-update.md)、[doc-create-workflow.md](references/doc/style/doc-create-workflow.md)、[doc-style-guideline.md](references/doc/style/doc-style-guideline.md)。
- 改写已有文档或块级编辑：读 [doc-read.md](references/doc/doc-read.md)、[doc-update.md](references/doc/doc-update.md)、[doc-block.md](references/doc/doc-block.md)、[doc-update-workflow.md](references/doc/style/doc-update-workflow.md)。
- 涉及 callout、分栏、颜色、表格、@人、附件、图片或保真改写：加读 [doc-jsonml-cookbook.md](references/doc/format/doc-jsonml-cookbook.md) 和 [doc-jsonml-schema.md](references/doc/format/doc-jsonml-schema.md)。
- 评论、权限、附件、导出、上传下载、复制移动重命名删除：按 [doc.md](references/doc.md) 的命令索引读取对应 `doc-*.md` 子文档。

## JSONML / Markdown 形态选择

创建和改写文档时按 [doc-update-workflow.md](references/doc/style/doc-update-workflow.md) 的编辑形态优先级执行：**JSONML 首选，element JSON 次选，Markdown 兜底**。

- 已有文档的局部改写、富结构保真、属性调整、callout / 分栏 / 表格 / @人 / 附件 / 颜色 / 嵌套结构：优先 `--content-format jsonml` 或 `doc block ... --content-format jsonml`。
- 决策型、含对比的数据沉淀型、用户要求美观/醒目/重点突出的新文档：直接 JSONML 起稿，使用 `.json` + `--content-format jsonml`。
- 执行型、说明型、纯文本追加等简单内容：可以 Markdown 起稿，使用 `.md` + `--content-format markdown`；若后续需要富结构，再按 JSONML 工作流精修。
- Markdown overwrite 会丢失部分富结构。整篇 overwrite 前必须提示风险并等待用户确认，写入后必须回读校验。

## 参数硬约束

- 创建文档只用 `--name`，不要写 `--title`。
- 目标文件夹只用 `--folder <文档文件夹nodeId或URL>`，不要写 `--parent` / `--parent-node` / `--parent-id`。
- 目标知识库只用 `--workspace <workspaceId或URL>`，不要写 `--space-id` / `--spaceId`。
- 文档内容：`create` / `update` 都只接 `--content` / `--content-file`，不要写 `--markdown`。
- 内容格式显式传 `--content-format jsonml` 或 `--content-format markdown`；不确定时先读对应子文档或 `dws doc <cmd> --help`。
- 复杂内容（换行、表格、代码块、长 Markdown / JSONML）先写临时 `.md` 或 `.json`，再用 `--content-file`，不要把大段内容塞进命令行。
- 每次 `create` / `update` / `block insert` / `block update` / `media insert` 后必须 `dws doc read` 或 `dws doc block list` 回读关键内容。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建文档" | 先读 `doc-create` + 创建工作流；按类型选择 JSONML 或 Markdown；`dws doc create --name "<标题>" --content-file <tmp> --content-format <jsonml|markdown>` |
| "搜文档" | 读 `doc-search.md`；`dws doc search --query "<关键词>"` |
| "读文档内容" | 读 `doc-read.md`；`dws doc read --node <nodeId>`，保真改写前用 `--content-format jsonml` |
| "更新文档内容 / 分块追加" | 读 `doc-update.md` + 改写工作流；按形态选择 `doc update` 或 `doc block update` |
| "插入 / 修改富 block" | 读 `doc-block.md` + JSONML cookbook/schema；优先 `dws doc block ... --content-format jsonml` |
| "删除文档/文件" | `dws doc delete`（需用户确认） |
| "删除块" | `dws doc block delete`（需用户确认） |

## 评测/多步文档短路径

- 知识库「评测记录」下按日期文件夹执行：`dws wiki space search --keyword "评测记录" --format json` → `dws doc list --workspace <WS_ID> --format json` → 找 `评测-doc-YYYYMMDD`；不存在则 `dws doc folder create --name "评测-doc-YYYYMMDD" --workspace <WS_ID> --format json`。
- 在目标文件夹创建文字文档：按创建工作流选择 `<tmp.json>` + `--content-format jsonml` 或 `<tmp.md>` + `--content-format markdown`，执行 `dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --content-file <tmp> --content-format <jsonml|markdown> --format json`。拿到 `nodeId` 后立即回读。
- 块级编辑固定顺序：`doc block list --node <nodeId> --content-format jsonml` → 选 `uuid` → `doc block insert/update --content-format jsonml` → `doc block list` 或 `doc read` 验证。删除块必须已有用户明确删除意图或二次确认。
- 插入引用块、代码块、callout 高亮块、列表、分栏、分隔线、表格等富结构时，先读对应 JSONML 文档并按 cookbook 构造 `--element`；不要默认走旧的 `--type` 快捷路径。
- 用户要求多个子文档/附件/块操作时，按 checklist 串行完成；最后一条 assistant 消息不能停在"接下来我要..."，必须有实际工具调用或明确失败原因。
- 用户说"下载文件"时用 `doc download --node ... --output <path>`；用户说"导出在线文档为 docx"时用 `doc export --node ... --output <path>`。
- 所有 dws 命令带 `--format json`；仅参数不确定时查 `--help`，不要把完整 help 当成最终结果。

## 危险操作

`doc delete` 会删除整篇文档/文件到回收站，`block delete` 会删除文档内部块；两者都必须确认再加 `--yes`。

## 跨产品协作

- 文件存储 / 上传下载 → 切到 `dingtalk-drive`
- 知识库空间管理 → 切到 `dingtalk-wiki`
- 数据表 → 切到 `dingtalk-aitable`
- 长篇报告生成（多源采集 + 写文档）→ 仍按本文 JSONML / Markdown 形态选择和回读校验规则执行
