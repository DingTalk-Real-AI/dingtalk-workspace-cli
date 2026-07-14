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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[doc.md](references/doc.md)；剧本：[04-document.md](references/04-document.md)；URL 识别与类型探测：[url-patterns.md](references/url-patterns.md)。

## URL 预检（含 alidocs URL 时必读）

输入含 `alidocs.dingtalk.com` URL 时，该域名下存在多种路径格式：`/i/p/...`（分享短链）、`/i/nodes/...`（节点链接，类型需探测）、`/spreadsheetv2/...`（电子表格直链）、`/document/edit|preview?dentryKey=...`（文档链接）等，每种处理流程不同。**必须先读 [url-patterns.md](references/url-patterns.md) 中的「alidocs URL 分流决策」**，按规则识别 URL 类型后再选择对应命令；其中 `/document/edit|preview?dentryKey=...` 直接路由到 `doc`，将完整 URL 原样传给 `--node`，**不要**提取 `dentryKey` 当裸 nodeId。

## 参数硬约束

- 创建文档只用 `--name`，不要写 `--title`。
- 目标文件夹只用 `--folder <文档文件夹nodeId或URL>`，不要写 `--parent` / `--parent-node` / `--parent-id`。
- 目标知识库只用 `--workspace <workspaceId或URL>`，不要写 `--space-id` / `--spaceId`。
- 文档内容：`create` / `update` 都只接 `--content` / `--content-file`，不要写 `--markdown`。
- 复杂内容（换行、表格、代码块、长 Markdown）先写临时 `.md`，再用 `--content-file`，不要把大段 Markdown 塞进命令行。
- 每次 `create` / `update` / `block insert` / `media insert` 后必须 `dws doc read` 或 `dws doc block list` 回读关键内容。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建文档（短内容）" | `dws doc create --name "<标题>" --content "<内容>"` |
| "创建+写入（长内容自动分块）" | `python scripts/doc_create_and_write.py --name "<标题>" --content "<内容>" [--mode append\|overwrite]` |
| "搜文档"（全局） | `dws drive search --query "<关键词>"`（`doc search` 已弃用；切到 `dingtalk-drive`） |
| "在某知识库里搜文档" | `dws wiki node search --workspace <WS_ID> --query "<关键词>"` |
| "读文档内容" | `dws doc read --node <nodeId>` |
| "更新文档内容 / 分块追加" | `dws doc update --node <nodeId> --content "<分块>" --mode append` |
| "删除块" | `dws doc block delete`（需用户确认） |
| "更新文档评论" | `dws doc comment update --node <nodeId> --comment-key <key> --content "<内容>"` |
| "删除文档评论" | `dws doc comment delete --node <nodeId> --comment-key <key> --yes`（需用户确认） |

## 评测/多步文档短路径

- 知识库「评测记录」下按日期文件夹执行：`dws wiki space search --keyword "评测记录" --format json` → `dws wiki node list --workspace <WS_ID> --format json`（或 `dws drive list --workspace <WS_ID>`；`doc list` 已弃用）→ 找 `评测-doc-YYYYMMDD`；不存在则 `dws wiki node create --workspace <WS_ID> --name "评测-doc-YYYYMMDD" --type folder --format json`（`doc folder create` 已弃用）。
- 在目标文件夹创建文字文档：`dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --content-file <tmp.md> --format json`。拿到 `nodeId` 后立即回读。
- 块级编辑固定顺序：`doc block list --node <nodeId>` → 选 `blockId` → `doc block insert/update/delete` → `doc block list` 验证。删除块必须已有用户明确删除意图或二次确认。
- 插入引用块、代码块、表格、分栏、附件、图片时，优先读 [doc.md](references/doc.md) 对应小节，不要只停在"准备查看 help"。说出"我将插入..."后必须立即执行对应 terminal 调用。
- 用户要求多个子文档/附件/块操作时，按 checklist 串行完成；最后一条 assistant 消息不能停在"接下来我要..."，必须有实际工具调用或明确失败原因。
- 用户说"下载文件"（已有文件）时用 `dws drive download --node ... --output <path>`（`doc download` 已弃用，切到 `dingtalk-drive`）；用户说"导出在线文档为 docx"时用 `doc export --node ... --output <path>`（内容级命令，未迁移）。
- 所有 dws 命令带 `--format json`；仅参数不确定时查 `--help`，不要把完整 help 当成最终结果。

## 危险操作

`block delete` 和 `comment delete` 不可逆，必须确认再加 `--yes`。

## 跨产品协作

- 文件存储 / 上传下载 → 切到 `dingtalk-drive`
- 知识库空间管理 → 切到 `dingtalk-wiki`
- 数据表 → 切到 `dingtalk-aitable`
- 长篇报告生成（多源采集 + 写文档）→ 此 skill 提供 `doc_create_and_write.py` 脚本
